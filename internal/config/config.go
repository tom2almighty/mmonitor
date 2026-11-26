package config

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// AppVersion is the semantic version of mmonitor. Release builds override it via
// -ldflags "-X mmonitor/internal/config.AppVersion=x.y.z".
var AppVersion = "3.0.0"

const (
	DefaultPollInterval   = 5 * time.Second
	DefaultFlushInterval  = 10 * time.Minute
	DefaultRetentionDays  = 30
	SummaryBucketSizeMS   = int64(24 * time.Hour / time.Millisecond)
	SummaryBuildInterval  = 10 * time.Minute
	WALCheckpointInterval = 5 * time.Minute
)

// IsContainerRuntime reports whether the process runs inside a Docker/OCI container.
var IsContainerRuntime = func() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

// Config holds startup configuration loaded from environment variables.
type Config struct {
	ListenAddr    string
	DBPath        string
	MihomoURL     string
	MihomoSecret  string
	PollInterval  time.Duration
	FlushInterval time.Duration
	RetentionDays int
	AllowedOrigin string
}

// DefaultListenAddr returns loopback on Windows desktop (safer by default) and
// wildcard on Linux/macOS/Docker so LAN clients can reach router deployments.
func DefaultListenAddr() string {
	if runtime.GOOS == "windows" && !IsContainerRuntime() {
		return "127.0.0.1:8080"
	}
	return ":8080"
}

// DefaultDatabasePath returns the platform default SQLite database path.
func DefaultDatabasePath() string {
	if IsContainerRuntime() {
		return "/data/mmonitor.db"
	}
	return "./data/mmonitor.db"
}

// Load reads startup configuration from environment variables.
func Load() Config {
	pollMS := getenvInt("MMONITOR_POLL_INTERVAL_MS", int(DefaultPollInterval/time.Millisecond))
	if pollMS < 500 {
		pollMS = 500
	}

	flushMS := getenvInt("MMONITOR_FLUSH_INTERVAL_MS", int(DefaultFlushInterval/time.Millisecond))
	if flushMS < 10000 {
		flushMS = 10000
	}

	retentionDays := getenvInt("MMONITOR_RETENTION_DAYS", DefaultRetentionDays)
	if retentionDays < 1 || retentionDays > 365 {
		retentionDays = DefaultRetentionDays
	}

	return Config{
		ListenAddr:    getenv("MMONITOR_LISTEN", DefaultListenAddr()),
		DBPath:        strings.TrimSpace(os.Getenv("MMONITOR_DB")),
		MihomoURL:     strings.TrimRight(strings.TrimSpace(os.Getenv("MIHOMO_URL")), "/"),
		MihomoSecret:  strings.TrimSpace(os.Getenv("MIHOMO_SECRET")),
		PollInterval:  time.Duration(pollMS) * time.Millisecond,
		FlushInterval: time.Duration(flushMS) * time.Millisecond,
		RetentionDays: retentionDays,
		AllowedOrigin: strings.TrimSpace(os.Getenv("MMONITOR_ALLOWED_ORIGIN")),
	}
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return parsed
}
