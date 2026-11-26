package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"mmonitor/internal/config"
	"mmonitor/internal/mihomo"
	"mmonitor/internal/monitor"
	"mmonitor/internal/server"
	"mmonitor/internal/storage"
)

func newTestService(t *testing.T) *monitor.Service {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "mmonitor.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	svc, err := monitor.NewService(db, dbPath, config.Config{
		ListenAddr:    "127.0.0.1:0",
		FlushInterval: 10 * time.Minute,
		RetentionDays: 30,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

func TestFakeIPResolutionAndMetadataAttribution(t *testing.T) {
	detector := mihomo.NewFakeIPDetector("198.18.0.1/16", "fc00::/18")

	c1 := detector.ResolveConnection(mihomo.Connection{
		ID:          "c1",
		Chains:      []string{"HK-01", "Proxy"},
		Rule:        "GeoSite",
		RulePayload: "youtube",
		Metadata: mihomo.Metadata{
			Network:           "tcp",
			SourceIP:          "127.0.0.1",
			Host:              "rr1.googlevideo.com",
			DestinationIP:     "198.18.0.42",
			RemoteDestination: "142.250.72.14",
			DNSMode:           "fake-ip",
			ProcessPath:       `C:\Program Files\Google\Chrome\Application\chrome.exe`,
		},
	})
	if c1.DNSMode != mihomo.DNSModeFakeIP || c1.Host != "rr1.googlevideo.com" || c1.DestinationIP != "142.250.72.14" || c1.Process != "chrome.exe" {
		t.Fatalf("unexpected resolved c1: %+v", c1)
	}

	c2 := detector.ResolveConnection(mihomo.Connection{
		ID:     "c2",
		Chains: []string{"HK-01"},
		Metadata: mihomo.Metadata{
			Network:       "udp",
			DestinationIP: "198.18.1.9",
			SniffHost:     "api.openai.com",
			ProcessPath:   "/Applications/Safari.app/Contents/MacOS/Safari",
		},
	})
	if c2.DNSMode != mihomo.DNSModeFakeIP || c2.Host != "api.openai.com" || c2.Process != "Safari" {
		t.Fatalf("unexpected resolved c2: %+v", c2)
	}

	c3 := detector.ResolveConnection(mihomo.Connection{
		ID:     "c3",
		Chains: []string{"DIRECT"},
		Metadata: mihomo.Metadata{
			Network:       "udp",
			DestinationIP: "198.18.2.55",
			DNSMode:       "fake-ip",
		},
	})
	if c3.DNSMode != mihomo.DNSModeFakeIPUnresolved || c3.Host != "198.18.2.55" {
		t.Fatalf("unexpected resolved c3: %+v", c3)
	}
}

func TestNormalizeHostPublicSuffix(t *testing.T) {
	cases := map[string]string{
		"www.google.com":            "google.com",
		"api.service.aliyun.com.cn": "aliyun.com.cn",
		"sub.bbc.co.uk":             "bbc.co.uk",
		"user.github.io":            "user.github.io",
		"198.18.0.1":                "198.18.0.1",
		"localhost":                 "localhost",
		"example.com:443":           "example.com",
	}
	for in, want := range cases {
		if got := storage.NormalizeHost(in); got != want {
			t.Errorf("NormalizeHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMultiBackendTrafficIsolationAndPermanentSummary(t *testing.T) {
	svc := newTestService(t)
	b1, err := svc.CreateBackend("Local PC", "http://127.0.0.1:9090", "", true)
	if err != nil {
		t.Fatalf("create b1: %v", err)
	}
	b2, err := svc.CreateBackend("OpenWrt Router", "http://192.168.1.1:9090", "secret", true)
	if err != nil {
		t.Fatalf("create b2: %v", err)
	}

	baseTime := time.Date(2026, 1, 10, 10, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return baseTime })

	if err := svc.ProcessConnections(b1.ID, &mihomo.ConnectionsSnapshot{
		UploadTotal:   1000,
		DownloadTotal: 4000,
		Connections: []mihomo.Connection{
			{
				ID:          "1",
				Upload:      1000,
				Download:    4000,
				Chains:      []string{"HK-Node"},
				Rule:        "GeoSite",
				RulePayload: "youtube",
				Metadata: mihomo.Metadata{
					SourceIP:      "127.0.0.1",
					Host:          "video.youtube.com",
					DestinationIP: "198.18.0.10",
					Process:       "chrome.exe",
					DNSMode:       "fake-ip",
					Network:       "tcp",
				},
			},
		},
	}); err != nil {
		t.Fatalf("process b1: %v", err)
	}

	if err := svc.ProcessConnections(b2.ID, &mihomo.ConnectionsSnapshot{
		UploadTotal:   500,
		DownloadTotal: 1500,
		Connections: []mihomo.Connection{
			{
				ID:       "2",
				Upload:   500,
				Download: 1500,
				Chains:   []string{"JP-Node"},
				Rule:     "MATCH",
				Metadata: mihomo.Metadata{
					SourceIP:      "192.168.1.50",
					DestinationIP: "198.18.0.99",
					Process:       "Unknown",
					DNSMode:       "fake-ip",
					Network:       "udp",
				},
			},
		},
	}); err != nil {
		t.Fatalf("process b2: %v", err)
	}

	if err := svc.FlushAllBuckets(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if err := storage.BackfillSummary(svc.DB(), baseTime.Add(24*time.Hour).UnixMilli()); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	// Simulate 200 days later: expired minute details are cleaned, but summary stays forever!
	later := baseTime.Add(200 * 24 * time.Hour)
	if err := storage.CleanupExpiredDetails(svc.DB(), later.UnixMilli(), 30, false); err != nil {
		t.Fatalf("cleanup expired details: %v", err)
	}

	// All-time summary query (start=0) across all backends should still return 7000 bytes!
	ovAll, err := svc.QueryOverview(0, 0, later.UnixMilli(), true)
	if err != nil {
		t.Fatalf("query all overview: %v", err)
	}
	if ovAll.TotalBytes != 7000 || ovAll.FakeIPBytes != 7000 || ovAll.UnresolvedFakeIPBytes != 2000 {
		t.Fatalf("unexpected all-time overview: %+v", ovAll)
	}

	// Per-backend isolation check
	ovB1, err := svc.QueryOverview(b1.ID, 0, later.UnixMilli(), true)
	if err != nil {
		t.Fatalf("query b1 overview: %v", err)
	}
	if ovB1.TotalBytes != 5000 {
		t.Fatalf("expected b1 total 5000, got %d", ovB1.TotalBytes)
	}

	// Custom prune & vacuum
	pruneRes, err := svc.PruneTraffic(baseTime.Add(48*time.Hour).UnixMilli(), "all", b2.ID)
	if err != nil {
		t.Fatalf("prune b2: %v", err)
	}
	if pruneRes.SummaryDeleted == 0 {
		t.Fatalf("expected b2 summary rows to be pruned, got %+v", pruneRes)
	}
	if _, err := svc.VacuumStorage(); err != nil {
		t.Fatalf("vacuum: %v", err)
	}
	stats, err := svc.StorageStats()
	if err != nil {
		t.Fatalf("storage stats: %v", err)
	}
	if stats.DBSizeBytes <= 0 {
		t.Fatalf("expected positive db size, got %+v", stats)
	}
}

func TestWebSocketStreamConnections(t *testing.T) {
	upgrader := websocket.Upgrader{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/connections" || r.Header.Get("Authorization") != "Bearer top-secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteJSON(mihomo.ConnectionsSnapshot{UploadTotal: 123, DownloadTotal: 456})
	}))
	defer ts.Close()

	client := mihomo.NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var got *mihomo.ConnectionsSnapshot
	err := client.StreamConnections(ctx, mihomo.Settings{URL: ts.URL, Secret: "top-secret"}, func(snap *mihomo.ConnectionsSnapshot) error {
		got = snap
		cancel()
		return nil
	})
	if err != nil && ctx.Err() == nil {
		t.Fatalf("stream error: %v", err)
	}
	if got == nil || got.DownloadTotal != 456 {
		t.Fatalf("unexpected ws snapshot: %+v", got)
	}
}

func TestHTTPBackendsAPIMasksSecretAndTestsConnection(t *testing.T) {
	mockMihomo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version":
			_ = json.NewEncoder(w).Encode(mihomo.VersionInfo{Version: "v1.19.2", Meta: true})
		case "/configs":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"mode": "rule",
				"dns": map[string]any{
					"enable":        true,
					"enhanced-mode": "fake-ip",
					"fake-ip-range": "198.18.0.1/16",
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockMihomo.Close()

	svc := newTestService(t)
	handler := server.New(svc, nil, []byte("MIT"), []byte("<svg/>"))

	// Create backend
	body := bytes.NewBufferString(`{"name":"Local","url":"` + mockMihomo.URL + `","secret":"my-secret","enabled":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/backends", body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create backend status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "my-secret") {
		t.Fatalf("create backend leaked secret: %s", rec.Body.String())
	}
	var created monitor.BackendWithStatus
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	// Test using stored backendId
	testReq := httptest.NewRequest(http.MethodPost, "/api/backends/test", bytes.NewBufferString(`{"backendId":1}`))
	testRec := httptest.NewRecorder()
	handler.ServeHTTP(testRec, testReq)
	var testRes monitor.TestConnectionResult
	if err := json.Unmarshal(testRec.Body.Bytes(), &testRes); err != nil {
		t.Fatalf("decode test response: %v", err)
	}
	if !testRes.OK || testRes.Version != "v1.19.2" || testRes.DNSEnhancedMode != "fake-ip" {
		t.Fatalf("unexpected test result: %+v", testRes)
	}
}
