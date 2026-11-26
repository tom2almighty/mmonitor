package monitor

import (
	"context"
	"encoding/json"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"mmonitor/internal/config"
	"mmonitor/internal/mihomo"
	"mmonitor/internal/storage"
)

type trafficDelta struct {
	BackendID     int64
	Timestamp     int64
	SourceIP      string
	Host          string
	DestinationIP string
	Process       string
	Outbound      string
	Chains        []string
	RuleGroup     string
	DNSMode       string
	Network       string
	Upload        int64
	Download      int64
}

// RunCollector starts independent collector loops for all enabled backends and
// runs periodic buffer flushing & database maintenance until ctx is cancelled.
func (s *Service) RunCollector(ctx context.Context) {
	s.mu.Lock()
	s.collectorCtx = ctx
	for _, rt := range s.runtimes {
		if rt.backend.Enabled {
			s.startBackendLocked(ctx, rt)
		}
	}
	s.mu.Unlock()

	maintTicker := time.NewTicker(30 * time.Second)
	defer maintTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.stopAllBackends()
			return
		case <-maintTicker.C:
			now := s.currentTime()
			nowMS := now.UnixMilli()
			if err := s.FlushCompletedBuckets(nowMS); err != nil {
				log.Printf("flush aggregate buffer: %v", err)
			}
			s.runPeriodicMaintenance(now, nowMS)
		}
	}
}

func (s *Service) startBackendLocked(parentCtx context.Context, rt *backendRuntime) {
	if rt.cancel != nil {
		rt.cancel()
	}
	bCtx, cancel := context.WithCancel(parentCtx)
	rt.cancel = cancel
	rt.done = make(chan struct{})
	go func() {
		defer close(rt.done)
		s.runBackendLoop(bCtx, rt)
	}()
}

func (s *Service) stopAllBackends() {
	s.mu.Lock()
	var dones []chan struct{}
	for _, rt := range s.runtimes {
		if rt.cancel != nil {
			rt.cancel()
			if rt.done != nil {
				dones = append(dones, rt.done)
			}
		}
	}
	s.collectorCtx = nil
	s.mu.Unlock()

	var wg sync.WaitGroup
	for _, ch := range dones {
		wg.Add(1)
		go func(c chan struct{}) {
			defer wg.Done()
			select {
			case <-c:
			case <-time.After(2 * time.Second):
			}
		}(ch)
	}
	wg.Wait()
}

func (s *Service) runBackendLoop(ctx context.Context, rt *backendRuntime) {
	backoff := 2 * time.Second
	const maxBackoff = 15 * time.Second
	settings := mihomo.Settings{URL: rt.backend.URL, Secret: rt.backend.Secret}.Normalize()

	for {
		if ctx.Err() != nil {
			return
		}
		if settings.URL == "" {
			s.setBackendDisconnected(rt, "Mihomo URL is not configured")
			return
		}

		s.refreshKernelMetadata(ctx, rt, settings)

		wsConnected := false
		err := s.currentClient().StreamConnections(ctx, settings, func(snapshot *mihomo.ConnectionsSnapshot) error {
			if !wsConnected {
				wsConnected = true
				backoff = 2 * time.Second
				s.setBackendConnected(rt, "ws")
			}
			return s.ProcessConnections(rt.backend.ID, snapshot)
		})

		if ctx.Err() != nil {
			return
		}

		// Fallback to a single HTTP poll if WebSocket stream failed.
		if snap, pollErr := s.currentClient().GetConnections(ctx, settings); pollErr == nil {
			s.setBackendConnected(rt, "http")
			if procErr := s.ProcessConnections(rt.backend.ID, snap); procErr != nil {
				log.Printf("process connections (%s): %v", rt.backend.Name, procErr)
			}
		} else {
			errMsg := pollErr.Error()
			if err != nil {
				errMsg = err.Error()
			}
			log.Printf("mihomo collector error (%s): %v", rt.backend.Name, errMsg)
			s.setBackendDisconnected(rt, errMsg)
		}

		s.mu.RLock()
		transport := rt.status.Transport
		s.mu.RUnlock()

		wait := backoff
		if transport == "http" {
			wait = s.cfg.PollInterval
			if wait <= 0 {
				wait = config.DefaultPollInterval
			}
		} else {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

func (s *Service) refreshKernelMetadata(ctx context.Context, rt *backendRuntime, settings mihomo.Settings) {
	metaCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	client := s.currentClient()
	var versionStr string
	if ver, err := client.GetVersion(metaCtx, settings); err == nil {
		versionStr = ver.Version
	}

	var cfg mihomo.KernelConfig
	hasCfg := false
	if c, err := client.GetConfigs(metaCtx, settings); err == nil {
		cfg = c
		hasCfg = true
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if versionStr != "" {
		rt.status.MihomoVersion = versionStr
	}
	if hasCfg {
		rt.status.MihomoMode = cfg.Mode
		rt.status.DNSEnhancedMode = cfg.DNS.EnhancedMode
		rt.status.FakeIPRange = cfg.DNS.FakeIPRange
		rt.status.TunEnabled = cfg.Tun.Enable
		rt.detector = mihomo.NewFakeIPDetector(cfg.DNS.FakeIPRange, cfg.DNS.FakeIPRange6)
	}
}

func (s *Service) setBackendConnected(rt *backendRuntime, transport string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rt.status.Connected = true
	rt.status.Transport = transport
	rt.status.LastError = ""
}

func (s *Service) setBackendDisconnected(rt *backendRuntime, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rt.status.Connected = false
	rt.status.Transport = "disconnected"
	rt.status.UploadSpeed = 0
	rt.status.DownloadSpeed = 0
	rt.status.LastError = errMsg
}

// ProcessConnections diffs a Mihomo snapshot for backendID against its previous
// baseline, updates live telemetry, and buffers minute-level aggregates.
func (s *Service) ProcessConnections(backendID int64, payload *mihomo.ConnectionsSnapshot) error {
	if payload == nil {
		return nil
	}
	now := s.currentTime()
	nowMS := now.UnixMilli()

	s.mu.Lock()
	rt, ok := s.runtimes[backendID]
	if !ok {
		s.mu.Unlock()
		return nil
	}

	detector := rt.detector
	if detector == nil {
		detector = mihomo.NewFakeIPDetector()
	}
	filterDirect := s.prefs.FilterDirectTraffic

	counterReset := payload.UploadTotal < rt.lastUploadTotal || payload.DownloadTotal < rt.lastDownloadTotal
	if counterReset {
		log.Printf("detected Mihomo counter reset (%s), clearing baselines", rt.backend.Name)
		rt.lastConnections = make(map[string]mihomo.Connection)
	}

	var uploadSpeed, downloadSpeed int64
	elapsedSec := 0.0
	if !rt.lastSnapshotAt.IsZero() && !counterReset {
		elapsedSec = now.Sub(rt.lastSnapshotAt).Seconds()
		if elapsedSec >= 0.2 {
			if deltaUp := payload.UploadTotal - rt.lastUploadTotal; deltaUp > 0 {
				uploadSpeed = int64(float64(deltaUp) / elapsedSec)
			}
			if deltaDown := payload.DownloadTotal - rt.lastDownloadTotal; deltaDown > 0 {
				downloadSpeed = int64(float64(deltaDown) / elapsedSec)
			}
		}
	}

	rt.lastUploadTotal = payload.UploadTotal
	rt.lastDownloadTotal = payload.DownloadTotal
	rt.lastSnapshotAt = now

	activeIDs := make(map[string]struct{}, len(payload.Connections))
	deltas := make([]trafficDelta, 0, len(payload.Connections))
	liveConns := make([]LiveConnection, 0, len(payload.Connections))

	activeFakeIP := 0
	unresolvedFakeIP := 0

	for _, conn := range payload.Connections {
		activeIDs[conn.ID] = struct{}{}

		prev, hasPrev := rt.lastConnections[conn.ID]
		uploadDelta := conn.Upload
		downloadDelta := conn.Download
		if hasPrev {
			uploadDelta = conn.Upload - prev.Upload
			downloadDelta = conn.Download - prev.Download
		}
		if uploadDelta < 0 {
			uploadDelta = conn.Upload
		}
		if downloadDelta < 0 {
			downloadDelta = conn.Download
		}

		resolved := detector.ResolveConnection(conn)
		if mihomo.IsFakeIPMode(resolved.DNSMode) {
			activeFakeIP++
			if resolved.DNSMode == mihomo.DNSModeFakeIPUnresolved {
				unresolvedFakeIP++
			}
		}

		var connUpSpeed, connDownSpeed int64
		if hasPrev && elapsedSec >= 0.2 {
			if uploadDelta > 0 {
				connUpSpeed = int64(float64(uploadDelta) / elapsedSec)
			}
			if downloadDelta > 0 {
				connDownSpeed = int64(float64(downloadDelta) / elapsedSec)
			}
		}

		liveConns = append(liveConns, LiveConnection{
			BackendID:       rt.backend.ID,
			BackendName:     rt.backend.Name,
			ID:              resolved.ID,
			SourceIP:        resolved.SourceIP,
			Host:            resolved.Host,
			SniffHost:       resolved.SniffHost,
			DestinationIP:   resolved.DestinationIP,
			DestinationPort: resolved.DestinationPort,
			Process:         resolved.Process,
			ProcessPath:     resolved.ProcessPath,
			Outbound:        resolved.Outbound,
			Chains:          resolved.Chains,
			RuleGroup:       resolved.RuleGroup,
			DNSMode:         resolved.DNSMode,
			Network:         resolved.Network,
			InboundType:     resolved.InboundType,
			Start:           resolved.Start,
			Upload:          resolved.Upload,
			Download:        resolved.Download,
			UploadSpeed:     connUpSpeed,
			DownloadSpeed:   connDownSpeed,
		})

		rt.lastConnections[conn.ID] = conn

		if uploadDelta == 0 && downloadDelta == 0 {
			continue
		}
		if filterDirect && mihomo.IsDirectConnection(resolved.Chains) {
			continue
		}

		deltas = append(deltas, trafficDelta{
			BackendID:     rt.backend.ID,
			Timestamp:     nowMS,
			SourceIP:      resolved.SourceIP,
			Host:          resolved.Host,
			DestinationIP: resolved.DestinationIP,
			Process:       resolved.Process,
			Outbound:      resolved.Outbound,
			Chains:        resolved.Chains,
			RuleGroup:     resolved.RuleGroup,
			DNSMode:       resolved.DNSMode,
			Network:       resolved.Network,
			Upload:        uploadDelta,
			Download:      downloadDelta,
		})
	}

	for id := range rt.lastConnections {
		if _, ok := activeIDs[id]; !ok {
			delete(rt.lastConnections, id)
		}
	}

	sort.Slice(liveConns, func(i, j int) bool {
		ti := liveConns[i].Upload + liveConns[i].Download
		tj := liveConns[j].Upload + liveConns[j].Download
		if ti == tj {
			return liveConns[i].Host < liveConns[j].Host
		}
		return ti > tj
	})

	rt.liveConnections = liveConns
	rt.status.UploadSpeed = uploadSpeed
	rt.status.DownloadSpeed = downloadSpeed
	rt.status.UploadTotal = payload.UploadTotal
	rt.status.DownloadTotal = payload.DownloadTotal
	rt.status.Memory = payload.Memory
	rt.status.ActiveConnections = len(liveConns)
	rt.status.ActiveFakeIPConnections = activeFakeIP
	rt.status.UnresolvedFakeIPConnections = unresolvedFakeIP
	rt.status.LastUpdated = nowMS
	s.mu.Unlock()

	if len(deltas) > 0 {
		if err := s.addToAggregateBuffer(deltas, nowMS); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) runPeriodicMaintenance(now time.Time, nowMS int64) {
	s.mu.RLock()
	lastSummary := s.lastSummaryBuild
	lastCleanup := s.lastCleanup
	lastVacuum := s.lastVacuum
	lastCheckpoint := s.lastCheckpoint
	retentionDays := s.prefs.RetentionDays
	s.mu.RUnlock()

	if now.Sub(lastSummary) >= config.SummaryBuildInterval {
		if err := storage.BackfillSummary(s.db, nowMS); err != nil {
			log.Printf("build traffic summary: %v", err)
		} else {
			s.mu.Lock()
			s.lastSummaryBuild = now
			s.mu.Unlock()
		}
	}

	if now.Sub(lastCleanup) >= time.Hour {
		runVacuum := now.Sub(lastVacuum) >= 7*24*time.Hour
		if err := storage.CleanupExpiredDetails(s.db, nowMS, retentionDays, runVacuum); err != nil {
			log.Printf("cleanup expired detail logs: %v", err)
		} else {
			s.mu.Lock()
			s.lastCleanup = now
			if runVacuum {
				s.lastVacuum = now
			}
			s.mu.Unlock()
		}
	}

	if now.Sub(lastCheckpoint) >= config.WALCheckpointInterval {
		if _, err := s.db.Exec(`PRAGMA wal_checkpoint(PASSIVE)`); err != nil {
			log.Printf("wal checkpoint: %v", err)
		} else {
			s.mu.Lock()
			s.lastCheckpoint = now
			s.mu.Unlock()
		}
	}
}

func (s *Service) addToAggregateBuffer(deltas []trafficDelta, nowMS int64) error {
	bucketStart := (nowMS / 60000) * 60000
	bucketEnd := bucketStart + 60000

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, d := range deltas {
		chainsJSON, err := json.Marshal(mihomo.SanitizeChains(d.Chains))
		if err != nil {
			return err
		}
		chainsStr := string(chainsJSON)
		key := aggregateKey{
			BackendID:     d.BackendID,
			BucketStart:   bucketStart,
			SourceIP:      d.SourceIP,
			Host:          d.Host,
			DestinationIP: d.DestinationIP,
			Process:       d.Process,
			Outbound:      d.Outbound,
			Chains:        chainsStr,
			RuleGroup:     d.RuleGroup,
			DNSMode:       d.DNSMode,
			Network:       d.Network,
		}
		if entry, exists := s.aggregateBuffer[key]; exists {
			entry.Upload += d.Upload
			entry.Download += d.Download
			entry.Count++
		} else {
			s.aggregateBuffer[key] = &storage.AggregatedEntry{
				BackendID:     d.BackendID,
				BucketStart:   bucketStart,
				BucketEnd:     bucketEnd,
				SourceIP:      d.SourceIP,
				Host:          d.Host,
				DestinationIP: d.DestinationIP,
				Process:       d.Process,
				Outbound:      d.Outbound,
				Chains:        chainsStr,
				RuleGroup:     d.RuleGroup,
				DNSMode:       d.DNSMode,
				Network:       d.Network,
				Upload:        d.Upload,
				Download:      d.Download,
				Count:         1,
			}
		}
	}
	return nil
}

// FlushCompletedBuckets persists buckets older than the current flush interval.
func (s *Service) FlushCompletedBuckets(nowMS int64) error {
	flushMS := int64(s.cfg.FlushInterval / time.Millisecond)
	if flushMS <= 0 {
		flushMS = int64(config.DefaultFlushInterval / time.Millisecond)
	}
	cutoff := (nowMS / flushMS) * flushMS
	return s.flushEntries(func(entry *storage.AggregatedEntry) bool {
		return entry.BucketEnd <= cutoff
	})
}

// FlushAllBuckets persists every entry currently in the in-memory aggregate buffer.
func (s *Service) FlushAllBuckets() error {
	return s.flushEntries(func(*storage.AggregatedEntry) bool {
		return true
	})
}

func (s *Service) flushEntries(shouldFlush func(*storage.AggregatedEntry) bool) error {
	s.mu.RLock()
	keys := make([]aggregateKey, 0)
	entries := make([]storage.AggregatedEntry, 0)
	for k, e := range s.aggregateBuffer {
		if shouldFlush(e) {
			keys = append(keys, k)
			entries = append(entries, *e)
		}
	}
	s.mu.RUnlock()

	if len(entries) == 0 {
		return nil
	}

	if err := storage.FlushAggregatedEntries(s.db, entries); err != nil {
		return err
	}

	s.mu.Lock()
	for i, k := range keys {
		current, ok := s.aggregateBuffer[k]
		if !ok {
			continue
		}
		flushed := entries[i]
		if current.Upload <= flushed.Upload && current.Download <= flushed.Download && current.Count <= flushed.Count {
			delete(s.aggregateBuffer, k)
		} else {
			current.Upload -= flushed.Upload
			current.Download -= flushed.Download
			current.Count -= flushed.Count
		}
	}
	s.mu.Unlock()
	return nil
}

func (s *Service) snapshotBuffer(filter func(storage.AggregatedEntry) bool) []storage.AggregatedEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]storage.AggregatedEntry, 0, len(s.aggregateBuffer))
	for _, e := range s.aggregateBuffer {
		entry := *e
		if filter == nil || filter(entry) {
			out = append(out, entry)
		}
	}
	return out
}

func matchesDNSModeFilter(entryMode, filter string) bool {
	switch strings.ToLower(strings.TrimSpace(filter)) {
	case "", "all":
		return true
	case mihomo.DNSModeFakeIP:
		return mihomo.IsFakeIPMode(entryMode)
	case "fake-ip-resolved":
		return entryMode == mihomo.DNSModeFakeIP
	case mihomo.DNSModeFakeIPUnresolved:
		return entryMode == mihomo.DNSModeFakeIPUnresolved
	case mihomo.DNSModeNormal:
		return entryMode == mihomo.DNSModeNormal
	case mihomo.DNSModeRedirHost:
		return entryMode == mihomo.DNSModeRedirHost
	default:
		return entryMode == filter
	}
}
