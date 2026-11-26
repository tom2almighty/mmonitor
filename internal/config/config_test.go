package config

import (
	"runtime"
	"testing"
)

func TestDefaultListenAddr(t *testing.T) {
	origIsContainer := IsContainerRuntime
	t.Cleanup(func() { IsContainerRuntime = origIsContainer })

	IsContainerRuntime = func() bool { return false }
	want := ":8080"
	if runtime.GOOS == "windows" {
		want = "127.0.0.1:38080"
	}
	if got := DefaultListenAddr(); got != want {
		t.Fatalf("DefaultListenAddr() non-container = %q, want %q", got, want)
	}

	IsContainerRuntime = func() bool { return true }
	if got := DefaultListenAddr(); got != ":8080" {
		t.Fatalf("DefaultListenAddr() container = %q, want %q", got, ":8080")
	}
}

func TestLoadListenAddrOverride(t *testing.T) {
	t.Setenv("MMONITOR_LISTEN", "127.0.0.1:19090")
	cfg := Load()
	if cfg.ListenAddr != "127.0.0.1:19090" {
		t.Fatalf("Load().ListenAddr = %q, want %q", cfg.ListenAddr, "127.0.0.1:19090")
	}
}
