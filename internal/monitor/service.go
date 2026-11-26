package monitor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"mmonitor/internal/config"
	"mmonitor/internal/mihomo"
	"mmonitor/internal/storage"
)

type aggregateKey struct {
	BackendID     int64
	BucketStart   int64
	SourceIP      string
	Host          string
	DestinationIP string
	Process       string
	Outbound      string
	Chains        string
	RuleGroup     string
	DNSMode       string
	Network       string
}

// LiveStatus captures real-time Mihomo connectivity, DNS mode, and throughput.
type LiveStatus struct {
	AppVersion                  string `json:"appVersion"`
	BackendID                   int64  `json:"backendId"`
	BackendName                 string `json:"backendName"`
	Configured                  bool   `json:"configured"`
	Connected                   bool   `json:"connected"`
	BackendCount                int    `json:"backendCount"`
	ConnectedCount              int    `json:"connectedCount"`
	Transport                   string `json:"transport"`
	MihomoVersion               string `json:"mihomoVersion"`
	MihomoMode                  string `json:"mihomoMode"`
	DNSEnhancedMode             string `json:"dnsEnhancedMode"`
	FakeIPRange                 string `json:"fakeIPRange"`
	TunEnabled                  bool   `json:"tunEnabled"`
	UploadSpeed                 int64  `json:"uploadSpeed"`
	DownloadSpeed               int64  `json:"downloadSpeed"`
	UploadTotal                 int64  `json:"uploadTotal"`
	DownloadTotal               int64  `json:"downloadTotal"`
	Memory                      int64  `json:"memory"`
	ActiveConnections           int    `json:"activeConnections"`
	ActiveFakeIPConnections     int    `json:"activeFakeIPConnections"`
	UnresolvedFakeIPConnections int    `json:"unresolvedFakeIPConnections"`
	LastUpdated                 int64  `json:"lastUpdated"`
	LastError                   string `json:"lastError"`
}

// BackendWithStatus enriches a stored Backend with its current live telemetry.
type BackendWithStatus struct {
	storage.Backend
	Status LiveStatus `json:"status"`
}

// LiveConnection represents an enriched active connection tagged with its backend.
type LiveConnection struct {
	BackendID       int64    `json:"backendId"`
	BackendName     string   `json:"backendName"`
	ID              string   `json:"id"`
	SourceIP        string   `json:"sourceIP"`
	Host            string   `json:"host"`
	SniffHost       string   `json:"sniffHost,omitempty"`
	DestinationIP   string   `json:"destinationIP"`
	DestinationPort string   `json:"destinationPort,omitempty"`
	Process         string   `json:"process"`
	ProcessPath     string   `json:"processPath,omitempty"`
	Outbound        string   `json:"outbound"`
	Chains          []string `json:"chains"`
	RuleGroup       string   `json:"ruleGroup"`
	DNSMode         string   `json:"dnsMode"`
	Network         string   `json:"network"`
	InboundType     string   `json:"inboundType,omitempty"`
	Start           string   `json:"start"`
	Upload          int64    `json:"upload"`
	Download        int64    `json:"download"`
	UploadSpeed     int64    `json:"uploadSpeed"`
	DownloadSpeed   int64    `json:"downloadSpeed"`
}

// TestConnectionResult reports the outcome of testing Mihomo controller credentials.
type TestConnectionResult struct {
	OK              bool   `json:"ok"`
	Version         string `json:"version"`
	Mode            string `json:"mode"`
	DNSEnhancedMode string `json:"dnsEnhancedMode"`
	FakeIPRange     string `json:"fakeIPRange"`
	TunEnabled      bool   `json:"tunEnabled"`
	LatencyMS       int64  `json:"latencyMs"`
	Error           string `json:"error,omitempty"`
}

// OverviewStats aggregates key dashboard KPIs for a selected time window.
type OverviewStats struct {
	TotalUpload           int64   `json:"totalUpload"`
	TotalDownload         int64   `json:"totalDownload"`
	TotalBytes            int64   `json:"totalBytes"`
	FakeIPUpload          int64   `json:"fakeIPUpload"`
	FakeIPDownload        int64   `json:"fakeIPDownload"`
	FakeIPBytes           int64   `json:"fakeIPBytes"`
	FakeIPRatio           float64 `json:"fakeIPRatio"`
	UnresolvedFakeIPBytes int64   `json:"unresolvedFakeIPBytes"`
	NormalBytes           int64   `json:"normalBytes"`
	ProcessCount          int     `json:"processCount"`
	HostCount             int     `json:"hostCount"`
	OutboundCount         int     `json:"outboundCount"`
	SourceIPCount         int     `json:"sourceIPCount"`
	RuleCount             int     `json:"ruleCount"`
}

type backendRuntime struct {
	backend           storage.Backend
	detector          *mihomo.FakeIPDetector
	lastConnections   map[string]mihomo.Connection
	lastUploadTotal   int64
	lastDownloadTotal int64
	lastSnapshotAt    time.Time
	status            LiveStatus
	liveConnections   []LiveConnection
	cancel            context.CancelFunc
	done              chan struct{}
}

// Service coordinates multi-backend Mihomo collection, SQLite persistence, and queries.
type Service struct {
	db     *sql.DB
	dbPath string
	client *mihomo.Client
	cfg    config.Config
	now    func() time.Time

	mu               sync.RWMutex
	prefs            storage.Preferences
	runtimes         map[int64]*backendRuntime
	collectorCtx     context.Context
	aggregateBuffer  map[aggregateKey]*storage.AggregatedEntry
	lastCleanup      time.Time
	lastVacuum       time.Time
	lastCheckpoint   time.Time
	lastSummaryBuild time.Time
}

// NewService initializes a multi-backend monitoring Service backed by db and cfg.
func NewService(db *sql.DB, dbPath string, cfg config.Config) (*Service, error) {
	if err := storage.EnsureEnvBackend(db, cfg.MihomoURL, cfg.MihomoSecret); err != nil {
		return nil, fmt.Errorf("ensure env backend: %w", err)
	}
	backends, err := storage.ListBackends(db)
	if err != nil {
		return nil, fmt.Errorf("list backends: %w", err)
	}
	prefs := storage.LoadPreferences(db, cfg.RetentionDays)

	if err := storage.BackfillSummary(db, time.Now().UnixMilli()); err != nil {
		return nil, fmt.Errorf("initial summary backfill: %w", err)
	}

	svc := &Service{
		db:              db,
		dbPath:          dbPath,
		client:          mihomo.NewClient(),
		cfg:             cfg,
		now:             time.Now,
		prefs:           prefs,
		runtimes:        make(map[int64]*backendRuntime),
		aggregateBuffer: make(map[aggregateKey]*storage.AggregatedEntry),
	}
	for _, b := range backends {
		svc.runtimes[b.ID] = newBackendRuntime(b)
	}
	return svc, nil
}

func newBackendRuntime(b storage.Backend) *backendRuntime {
	return &backendRuntime{
		backend:         b,
		detector:        mihomo.NewFakeIPDetector(),
		lastConnections: make(map[string]mihomo.Connection),
		liveConnections: make([]LiveConnection, 0),
		status: LiveStatus{
			AppVersion:  config.AppVersion,
			BackendID:   b.ID,
			BackendName: b.Name,
			Configured:  b.URL != "",
			Transport:   "disconnected",
		},
	}
}

// SetClock overrides the service clock for deterministic unit tests.
func (s *Service) SetClock(fn func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = fn
}

// SetClient overrides the Mihomo client (useful for tests).
func (s *Service) SetClient(client *mihomo.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.client = client
}

// DB exposes the underlying database handle.
func (s *Service) DB() *sql.DB {
	return s.db
}

// Config returns the startup configuration.
func (s *Service) Config() config.Config {
	return s.cfg
}

func (s *Service) currentTime() time.Time {
	s.mu.RLock()
	fn := s.now
	s.mu.RUnlock()
	if fn != nil {
		return fn()
	}
	return time.Now()
}

func (s *Service) currentClient() *mihomo.Client {
	s.mu.RLock()
	c := s.client
	s.mu.RUnlock()
	if c == nil {
		return mihomo.NewClient()
	}
	return c
}

// ListBackends returns all backends along with their live status.
func (s *Service) ListBackends() ([]BackendWithStatus, error) {
	backends, err := storage.ListBackends(s.db)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]BackendWithStatus, 0, len(backends))
	for _, b := range backends {
		var st LiveStatus
		if rt, ok := s.runtimes[b.ID]; ok {
			st = rt.status
		} else {
			st = LiveStatus{
				AppVersion:  config.AppVersion,
				BackendID:   b.ID,
				BackendName: b.Name,
				Configured:  b.URL != "",
				Transport:   "disconnected",
			}
		}
		st.AppVersion = config.AppVersion
		st.BackendID = b.ID
		st.BackendName = b.Name
		out = append(out, BackendWithStatus{Backend: b, Status: st})
	}
	return out, nil
}

// CreateBackend adds a new Mihomo backend and immediately starts collecting if enabled.
func (s *Service) CreateBackend(name, rawURL, secret string, enabled bool) (BackendWithStatus, error) {
	b, err := storage.CreateBackend(s.db, name, rawURL, secret, enabled)
	if err != nil {
		return BackendWithStatus{}, err
	}
	rt := newBackendRuntime(b)

	s.mu.Lock()
	s.runtimes[b.ID] = rt
	parentCtx := s.collectorCtx
	if parentCtx != nil && b.Enabled {
		s.startBackendLocked(parentCtx, rt)
	}
	st := rt.status
	s.mu.Unlock()

	return BackendWithStatus{Backend: b, Status: st}, nil
}

// UpdateBackend modifies an existing backend and restarts/stops its collector as needed.
func (s *Service) UpdateBackend(id int64, name, rawURL, secret string, clearSecret, enabled bool) (BackendWithStatus, error) {
	b, err := storage.UpdateBackend(s.db, id, name, rawURL, secret, clearSecret, enabled)
	if err != nil {
		return BackendWithStatus{}, err
	}

	s.mu.Lock()
	oldRT := s.runtimes[id]
	var oldCancel context.CancelFunc
	if oldRT != nil && oldRT.cancel != nil {
		oldCancel = oldRT.cancel
	}
	newRT := newBackendRuntime(b)
	s.runtimes[id] = newRT
	parentCtx := s.collectorCtx
	if parentCtx != nil && b.Enabled {
		s.startBackendLocked(parentCtx, newRT)
	}
	st := newRT.status
	s.mu.Unlock()

	if oldCancel != nil {
		oldCancel()
	}
	return BackendWithStatus{Backend: b, Status: st}, nil
}

// DeleteBackend stops the backend collector, removes the backend, and optionally purges its data.
func (s *Service) DeleteBackend(id int64, deleteData bool) error {
	s.mu.Lock()
	oldRT := s.runtimes[id]
	delete(s.runtimes, id)
	if deleteData {
		for k := range s.aggregateBuffer {
			if k.BackendID == id {
				delete(s.aggregateBuffer, k)
			}
		}
	}
	s.mu.Unlock()

	if oldRT != nil && oldRT.cancel != nil {
		oldRT.cancel()
	}
	return storage.DeleteBackend(s.db, id, deleteData)
}

// CurrentPreferences returns the active collector and display preferences.
func (s *Service) CurrentPreferences() storage.Preferences {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.prefs
}

// UpdatePreferences persists and applies user preferences.
func (s *Service) UpdatePreferences(prefs storage.Preferences) error {
	if prefs.RetentionDays < 1 || prefs.RetentionDays > 365 {
		return fmt.Errorf("retentionDays must be between 1 and 365")
	}
	if err := storage.SavePreferences(s.db, prefs); err != nil {
		return err
	}
	s.mu.Lock()
	s.prefs = prefs
	s.mu.Unlock()
	return nil
}

// LiveStatus returns telemetry for a specific backend (backendID > 0) or an
// aggregated summary across all configured backends (backendID == 0).
func (s *Service) LiveStatus(backendID int64) LiveStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if backendID > 0 {
		rt, ok := s.runtimes[backendID]
		if !ok {
			return LiveStatus{
				AppVersion:   config.AppVersion,
				BackendID:    backendID,
				BackendCount: len(s.runtimes),
				Transport:    "disconnected",
			}
		}
		st := rt.status
		st.AppVersion = config.AppVersion
		st.BackendID = rt.backend.ID
		st.BackendName = rt.backend.Name
		st.BackendCount = len(s.runtimes)
		if st.Connected {
			st.ConnectedCount = 1
		}
		return st
	}

	agg := LiveStatus{
		AppVersion:   config.AppVersion,
		Configured:   len(s.runtimes) > 0,
		BackendCount: len(s.runtimes),
		Transport:    "disconnected",
	}

	// Sort backend IDs for deterministic metadata selection when 1 backend exists.
	ids := make([]int64, 0, len(s.runtimes))
	for id := range s.runtimes {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	for _, id := range ids {
		rt := s.runtimes[id]
		st := rt.status
		if st.Connected {
			agg.Connected = true
			agg.ConnectedCount++
			if agg.Transport == "disconnected" || st.Transport == "ws" {
				agg.Transport = st.Transport
			}
		} else if agg.LastError == "" && st.LastError != "" && rt.backend.Enabled {
			agg.LastError = st.LastError
		}
		agg.UploadSpeed += st.UploadSpeed
		agg.DownloadSpeed += st.DownloadSpeed
		agg.UploadTotal += st.UploadTotal
		agg.DownloadTotal += st.DownloadTotal
		agg.Memory += st.Memory
		agg.ActiveConnections += st.ActiveConnections
		agg.ActiveFakeIPConnections += st.ActiveFakeIPConnections
		agg.UnresolvedFakeIPConnections += st.UnresolvedFakeIPConnections
		if st.LastUpdated > agg.LastUpdated {
			agg.LastUpdated = st.LastUpdated
		}
		if len(ids) == 1 {
			agg.BackendID = rt.backend.ID
			agg.BackendName = rt.backend.Name
			agg.MihomoVersion = st.MihomoVersion
			agg.MihomoMode = st.MihomoMode
			agg.DNSEnhancedMode = st.DNSEnhancedMode
			agg.FakeIPRange = st.FakeIPRange
			agg.TunEnabled = st.TunEnabled
		} else if st.DNSEnhancedMode == mihomo.DNSModeFakeIP {
			agg.DNSEnhancedMode = mihomo.DNSModeFakeIP
			if agg.FakeIPRange == "" {
				agg.FakeIPRange = st.FakeIPRange
			}
		}
	}
	return agg
}

// LiveConnections returns active connections for one backend or across all backends.
func (s *Service) LiveConnections(backendID int64) []LiveConnection {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if backendID > 0 {
		rt, ok := s.runtimes[backendID]
		if !ok {
			return []LiveConnection{}
		}
		out := make([]LiveConnection, len(rt.liveConnections))
		copy(out, rt.liveConnections)
		return out
	}

	out := make([]LiveConnection, 0)
	for _, rt := range s.runtimes {
		out = append(out, rt.liveConnections...)
	}
	sort.Slice(out, func(i, j int) bool {
		ti := out[i].Upload + out[i].Download
		tj := out[j].Upload + out[j].Download
		if ti == tj {
			return out[i].Host < out[j].Host
		}
		return ti > tj
	})
	return out
}

// TestConnection checks reachability of a Mihomo controller and reads its version and DNS mode.
func (s *Service) TestConnection(ctx context.Context, candidate mihomo.Settings, backendID int64, clearSecret bool) TestConnectionResult {
	candidate = candidate.Normalize()
	if backendID > 0 {
		if existing, found, err := storage.GetBackend(s.db, backendID); err == nil && found {
			if candidate.URL == "" {
				candidate.URL = existing.URL
			}
			if !clearSecret && candidate.Secret == "" {
				candidate.Secret = existing.Secret
			}
		}
	}
	if candidate.URL == "" {
		return TestConnectionResult{OK: false, Error: "Mihomo URL is required"}
	}

	start := time.Now()
	client := s.currentClient()
	ver, err := client.GetVersion(ctx, candidate)
	if err != nil {
		return TestConnectionResult{
			OK:        false,
			LatencyMS: time.Since(start).Milliseconds(),
			Error:     err.Error(),
		}
	}

	res := TestConnectionResult{
		OK:        true,
		Version:   ver.Version,
		LatencyMS: time.Since(start).Milliseconds(),
	}
	if cfg, cfgErr := client.GetConfigs(ctx, candidate); cfgErr == nil {
		res.Mode = cfg.Mode
		res.DNSEnhancedMode = cfg.DNS.EnhancedMode
		res.FakeIPRange = cfg.DNS.FakeIPRange
		res.TunEnabled = cfg.Tun.Enable
	}
	return res
}

// CloseLiveConnection terminates one active connection on the specified backend.
func (s *Service) CloseLiveConnection(ctx context.Context, backendID int64, connID string) error {
	b, found, err := storage.GetBackend(s.db, backendID)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("backend not found")
	}
	return s.currentClient().CloseConnection(ctx, mihomo.Settings{URL: b.URL, Secret: b.Secret}, connID)
}

// CloseAllLiveConnections terminates active connections on one backend or all enabled backends.
func (s *Service) CloseAllLiveConnections(ctx context.Context, backendID int64) error {
	if backendID > 0 {
		b, found, err := storage.GetBackend(s.db, backendID)
		if err != nil {
			return err
		}
		if !found {
			return errors.New("backend not found")
		}
		return s.currentClient().CloseAllConnections(ctx, mihomo.Settings{URL: b.URL, Secret: b.Secret})
	}
	backends, err := storage.ListBackends(s.db)
	if err != nil {
		return err
	}
	var errs []error
	for _, b := range backends {
		if !b.Enabled {
			continue
		}
		if err := s.currentClient().CloseAllConnections(ctx, mihomo.Settings{URL: b.URL, Secret: b.Secret}); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", b.Name, err))
		}
	}
	return errors.Join(errs...)
}

// ClearTraffic deletes persisted traffic and clears matching entries in the memory buffer.
func (s *Service) ClearTraffic(backendID int64) error {
	if err := storage.ClearTraffic(s.db, backendID); err != nil {
		return err
	}
	s.mu.Lock()
	if backendID > 0 {
		for k := range s.aggregateBuffer {
			if k.BackendID == backendID {
				delete(s.aggregateBuffer, k)
			}
		}
	} else {
		s.aggregateBuffer = make(map[aggregateKey]*storage.AggregatedEntry)
	}
	s.mu.Unlock()
	return nil
}

// StorageStats returns SQLite disk usage and record statistics.
func (s *Service) StorageStats() (storage.StorageStats, error) {
	return storage.GetStorageStats(s.db, s.dbPath)
}

// VacuumStorage flushes in-memory buckets, checkpoints WAL, and runs VACUUM.
func (s *Service) VacuumStorage() (storage.VacuumResult, error) {
	if err := s.FlushAllBuckets(); err != nil {
		return storage.VacuumResult{}, err
	}
	return storage.VacuumDatabase(s.db, s.dbPath)
}

// PruneTraffic removes records older than beforeMS (and removes matching old buffer entries).
func (s *Service) PruneTraffic(beforeMS int64, scope string, backendID int64) (storage.PruneResult, error) {
	if err := s.FlushAllBuckets(); err != nil {
		return storage.PruneResult{}, err
	}
	return storage.PruneTrafficBefore(s.db, beforeMS, scope, backendID)
}
