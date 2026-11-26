//go:build windows

package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

func TestDashboardURLFromAddr(t *testing.T) {
	cases := []struct {
		addr string
		want string
	}{
		{"0.0.0.0:38080", "http://127.0.0.1:38080/"},
		{"[::]:9090", "http://127.0.0.1:9090/"},
		{"127.0.0.1:38080", "http://127.0.0.1:38080/"},
	}
	for _, tc := range cases {
		tcpAddr, err := net.ResolveTCPAddr("tcp", tc.addr)
		if err != nil {
			t.Fatalf("resolve %s: %v", tc.addr, err)
		}
		got, err := dashboardURLFromAddr(tcpAddr)
		if err != nil {
			t.Fatalf("dashboardURLFromAddr(%s): %v", tc.addr, err)
		}
		if got != tc.want {
			t.Errorf("dashboardURLFromAddr(%s) = %q, want %q", tc.addr, got, tc.want)
		}
	}
}

func TestValidateMetadataURL(t *testing.T) {
	if err := validateMetadataURL("http://127.0.0.1:38080/"); err != nil {
		t.Fatalf("expected valid url, got %v", err)
	}
	for _, bad := range []string{
		"file:///C:/Windows/calc.exe",
		"javascript:alert(1)",
		"http://127.0.0.1:38080/evil",
		"http://127.0.0.1/",
	} {
		if err := validateMetadataURL(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestRegistryAutoStartStoreRoundTrip(t *testing.T) {
	testKey := fmt.Sprintf(`Software\MMonitorTest\%d`, time.Now().UnixNano())
	defer func() { _ = registry.DeleteKey(registry.CURRENT_USER, testKey) }()

	store := registryAutoStartStore{runKeyPath: testKey}
	exePath := `C:\Tools\MMonitor\mmonitor.exe`

	enabled, err := store.Enabled(exePath)
	if err != nil || enabled {
		t.Fatalf("expected initially disabled, got enabled=%v err=%v", enabled, err)
	}
	if err := store.Enable(exePath); err != nil {
		t.Fatalf("enable: %v", err)
	}
	enabled, err = store.Enabled(exePath)
	if err != nil || !enabled {
		t.Fatalf("expected enabled after Enable, got enabled=%v err=%v", enabled, err)
	}
	if err := store.Disable(); err != nil {
		t.Fatalf("disable: %v", err)
	}
	enabled, err = store.Enabled(exePath)
	if err != nil || enabled {
		t.Fatalf("expected disabled after Disable, got enabled=%v err=%v", enabled, err)
	}
}

func TestRotatingLogWriterRotatesOnOverflow(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "mmonitor.log")

	w, err := newRotatingLogWriter(logPath)
	if err != nil {
		t.Fatalf("newRotatingLogWriter: %v", err)
	}
	defer w.Close()

	w.size = rotatingLogMaxSize - 4
	if _, err := w.Write([]byte("0123456789\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := os.Stat(logPath + rotatingLogBackupSuffix); err != nil {
		t.Fatalf("expected backup log file to exist: %v", err)
	}
}
