package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// Open initializes a pure-Go SQLite database at path with WAL mode and schema.
func Open(path string) (*sql.DB, error) {
	dsn := buildDSN(path)
	if path != ":memory:" && !strings.HasPrefix(path, "file:") {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create db directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := initSchema(db); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func buildDSN(path string) string {
	if path == ":memory:" {
		return ":memory:?_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)"
	}
	if strings.HasPrefix(path, "file:") {
		return path
	}
	return "file:" + filepath.ToSlash(path) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)"
}

func initSchema(db *sql.DB) error {
	const schema = `
	CREATE TABLE IF NOT EXISTS backends (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		url TEXT NOT NULL,
		secret TEXT NOT NULL DEFAULT '',
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS traffic_aggregated (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		backend_id INTEGER NOT NULL,
		bucket_start INTEGER NOT NULL,
		bucket_end INTEGER NOT NULL,
		source_ip TEXT NOT NULL,
		host TEXT NOT NULL,
		destination_ip TEXT NOT NULL DEFAULT '',
		process TEXT NOT NULL,
		outbound TEXT NOT NULL,
		chains TEXT NOT NULL DEFAULT '[]',
		rule_group TEXT NOT NULL DEFAULT 'unknown',
		dns_mode TEXT NOT NULL DEFAULT 'normal',
		network TEXT NOT NULL DEFAULT 'tcp',
		upload INTEGER NOT NULL,
		download INTEGER NOT NULL,
		count INTEGER NOT NULL,
		UNIQUE(backend_id, bucket_start, bucket_end, source_ip, host, destination_ip, process, outbound, chains, rule_group, dns_mode, network)
	);

	CREATE INDEX IF NOT EXISTS idx_traffic_agg_bucket ON traffic_aggregated(bucket_start, bucket_end);
	CREATE INDEX IF NOT EXISTS idx_traffic_agg_backend_bucket ON traffic_aggregated(backend_id, bucket_start, bucket_end);
	CREATE INDEX IF NOT EXISTS idx_traffic_agg_process ON traffic_aggregated(process, bucket_start);
	CREATE INDEX IF NOT EXISTS idx_traffic_agg_host ON traffic_aggregated(host, bucket_start);
	CREATE INDEX IF NOT EXISTS idx_traffic_agg_source_ip ON traffic_aggregated(source_ip, bucket_start);
	CREATE INDEX IF NOT EXISTS idx_traffic_agg_outbound ON traffic_aggregated(outbound, bucket_start);
	CREATE INDEX IF NOT EXISTS idx_traffic_agg_rule_group ON traffic_aggregated(rule_group, bucket_start);
	CREATE INDEX IF NOT EXISTS idx_traffic_agg_dns_mode ON traffic_aggregated(dns_mode, bucket_start);

	CREATE TABLE IF NOT EXISTS traffic_summary (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		backend_id INTEGER NOT NULL,
		bucket_start INTEGER NOT NULL,
		bucket_end INTEGER NOT NULL,
		dimension TEXT NOT NULL,
		label TEXT NOT NULL,
		secondary_label TEXT NOT NULL DEFAULT '',
		dns_mode TEXT NOT NULL DEFAULT 'normal',
		upload INTEGER NOT NULL,
		download INTEGER NOT NULL,
		count INTEGER NOT NULL,
		UNIQUE(backend_id, bucket_start, bucket_end, dimension, label, secondary_label, dns_mode)
	);

	CREATE INDEX IF NOT EXISTS idx_traffic_summary_bucket ON traffic_summary(bucket_start, bucket_end);
	CREATE INDEX IF NOT EXISTS idx_traffic_summary_backend_bucket ON traffic_summary(backend_id, bucket_start, bucket_end);
	CREATE INDEX IF NOT EXISTS idx_traffic_summary_dim_label ON traffic_summary(dimension, label, bucket_start, bucket_end);
	CREATE INDEX IF NOT EXISTS idx_traffic_summary_dim_secondary ON traffic_summary(dimension, secondary_label, bucket_start, bucket_end);

	CREATE TABLE IF NOT EXISTS app_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT ''
	);
	`
	_, err := db.Exec(schema)
	return err
}
