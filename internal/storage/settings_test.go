package storage

import (
	"database/sql"
	"path/filepath"
	"testing"

	"mmonitor/internal/config"
)

func openTestStorageDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestBackendCRUDAndSecrets(t *testing.T) {
	db := openTestStorageDB(t)

	// Empty initially
	list, err := ListBackends(db)
	if err != nil {
		t.Fatalf("list empty backends: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 backends, got %d", len(list))
	}

	// Empty URL is rejected
	if _, err := CreateBackend(db, "Invalid", "   ", "", true); err == nil {
		t.Fatal("expected error for empty backend URL")
	}

	// Create backend with blank name defaults to normalized URL
	created, err := CreateBackend(db, "", " http://127.0.0.1:9090/ ", " my-secret ", true)
	if err != nil {
		t.Fatalf("create backend: %v", err)
	}
	if created.Name != "http://127.0.0.1:9090" || created.URL != "http://127.0.0.1:9090" {
		t.Fatalf("unexpected normalized backend: %+v", created)
	}
	if !created.HasSecret || created.Secret != "my-secret" || !created.Enabled {
		t.Fatalf("unexpected backend flags: %+v", created)
	}

	// GetBackend returns stored record
	got, found, err := GetBackend(db, created.ID)
	if err != nil || !found {
		t.Fatalf("get backend: found=%v err=%v", found, err)
	}
	if got.ID != created.ID || got.Secret != "my-secret" {
		t.Fatalf("unexpected fetched backend: %+v", got)
	}

	// Update preserving secret when secret is empty and clearSecret is false
	updated, err := UpdateBackend(db, created.ID, "Local Mihomo", "http://127.0.0.1:9091/", "", false, false)
	if err != nil {
		t.Fatalf("update backend preserve secret: %v", err)
	}
	if updated.Name != "Local Mihomo" || updated.URL != "http://127.0.0.1:9091" || updated.Secret != "my-secret" || !updated.HasSecret || updated.Enabled {
		t.Fatalf("unexpected updated backend: %+v", updated)
	}

	// Update clearing secret
	cleared, err := UpdateBackend(db, created.ID, "", "http://127.0.0.1:9091", "", true, true)
	if err != nil {
		t.Fatalf("update backend clear secret: %v", err)
	}
	if cleared.Name != "Local Mihomo" || cleared.Secret != "" || cleared.HasSecret || !cleared.Enabled {
		t.Fatalf("expected cleared secret and preserved name: %+v", cleared)
	}

	// Update nonexistent backend fails
	if _, err := UpdateBackend(db, 9999, "Ghost", "http://127.0.0.1:9099", "", false, true); err == nil {
		t.Fatal("expected error updating nonexistent backend")
	}

	// Seed traffic rows and verify DeleteBackend with deleteData=true removes them
	if _, err := db.Exec(`
		INSERT INTO traffic_aggregated (backend_id, bucket_start, bucket_end, source_ip, host, destination_ip, process, outbound, chains, rule_group, dns_mode, network, upload, download, count)
		VALUES (?, 1000, 2000, '127.0.0.1', 'example.com', '1.1.1.1', 'chrome.exe', 'PROXY', '["PROXY"]', 'MATCH', 'fake-ip', 'tcp', 100, 200, 1)
	`, created.ID); err != nil {
		t.Fatalf("seed traffic_aggregated: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO traffic_summary (backend_id, bucket_start, bucket_end, dimension, label, secondary_label, dns_mode, upload, download, count)
		VALUES (?, 1000, 2000, 'host', 'example.com', '', 'fake-ip', 100, 200, 1)
	`, created.ID); err != nil {
		t.Fatalf("seed traffic_summary: %v", err)
	}

	if err := DeleteBackend(db, created.ID, true); err != nil {
		t.Fatalf("delete backend with data: %v", err)
	}
	if _, found, err := GetBackend(db, created.ID); err != nil || found {
		t.Fatalf("expected backend deleted, found=%v err=%v", found, err)
	}
	var aggCount, sumCount int
	_ = db.QueryRow(`SELECT COUNT(*) FROM traffic_aggregated WHERE backend_id = ?`, created.ID).Scan(&aggCount)
	_ = db.QueryRow(`SELECT COUNT(*) FROM traffic_summary WHERE backend_id = ?`, created.ID).Scan(&sumCount)
	if aggCount != 0 || sumCount != 0 {
		t.Fatalf("expected traffic deleted, got agg=%d sum=%d", aggCount, sumCount)
	}
}

func TestEnsureEnvBackend(t *testing.T) {
	db := openTestStorageDB(t)

	// Empty env URL is a no-op
	if err := EnsureEnvBackend(db, "", ""); err != nil {
		t.Fatalf("ensure empty env backend: %v", err)
	}

	// First call inserts Default Mihomo
	if err := EnsureEnvBackend(db, "http://127.0.0.1:9090", "secret-1"); err != nil {
		t.Fatalf("ensure env backend insert: %v", err)
	}
	list, err := ListBackends(db)
	if err != nil || len(list) != 1 {
		t.Fatalf("expected 1 backend, got len=%d err=%v", len(list), err)
	}
	if list[0].Name != "Default Mihomo" || list[0].Secret != "secret-1" {
		t.Fatalf("unexpected env backend: %+v", list[0])
	}

	// Second call with updated secret updates in place without duplicating
	if err := EnsureEnvBackend(db, "http://127.0.0.1:9090/", "secret-2"); err != nil {
		t.Fatalf("ensure env backend update secret: %v", err)
	}
	list, err = ListBackends(db)
	if err != nil || len(list) != 1 {
		t.Fatalf("expected still 1 backend, got len=%d err=%v", len(list), err)
	}
	if list[0].Secret != "secret-2" {
		t.Fatalf("expected updated secret-2, got %q", list[0].Secret)
	}
}

func TestPreferencesLoadAndSave(t *testing.T) {
	db := openTestStorageDB(t)

	// Defaults when empty and invalid defaultRetentionDays falls back to config default
	defaults := LoadPreferences(db, 0)
	if !defaults.DomainGroupingEnabled || defaults.FilterDirectTraffic || defaults.RetentionDays != config.DefaultRetentionDays {
		t.Fatalf("unexpected default preferences: %+v", defaults)
	}

	// Explicit default retention days is respected when not yet stored
	customDefault := LoadPreferences(db, 45)
	if customDefault.RetentionDays != 45 {
		t.Fatalf("expected retention 45, got %d", customDefault.RetentionDays)
	}

	// SavePreferences persists all fields
	want := Preferences{
		DomainGroupingEnabled: false,
		FilterDirectTraffic:   true,
		RetentionDays:         90,
	}
	if err := SavePreferences(db, want); err != nil {
		t.Fatalf("save preferences: %v", err)
	}
	got := LoadPreferences(db, 30)
	if got != want {
		t.Fatalf("loaded preferences mismatch: got %+v, want %+v", got, want)
	}

	// Out-of-range retention value in DB falls back to defaultRetentionDays
	if _, err := db.Exec(`UPDATE app_settings SET value = '9999' WHERE key = 'aggregate_retention_days'`); err != nil {
		t.Fatalf("corrupt retention setting: %v", err)
	}
	fallback := LoadPreferences(db, 14)
	if fallback.RetentionDays != 14 {
		t.Fatalf("expected fallback retention 14 for out-of-range DB value, got %d", fallback.RetentionDays)
	}
}
