package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"mmonitor/internal/config"
	"mmonitor/internal/mihomo"
)

// AggregatedEntry represents one 1-minute bucketed traffic record scoped to a backend.
type AggregatedEntry struct {
	BackendID     int64
	BucketStart   int64
	BucketEnd     int64
	SourceIP      string
	Host          string
	DestinationIP string
	Process       string
	Outbound      string
	Chains        string
	RuleGroup     string
	DNSMode       string
	Network       string
	Upload        int64
	Download      int64
	Count         int64
}

// AggregatedRow is returned by dimension aggregate and substat queries.
type AggregatedRow struct {
	Label    string `json:"label"`
	Upload   int64  `json:"upload"`
	Download int64  `json:"download"`
	Total    int64  `json:"total"`
	Count    int64  `json:"count"`
}

// TrendPoint represents one time-series point.
type TrendPoint struct {
	Timestamp int64 `json:"timestamp"`
	Upload    int64 `json:"upload"`
	Download  int64 `json:"download"`
}

// ConnectionDetail represents grouped connection chains for drilldown inspection.
type ConnectionDetail struct {
	DestinationIP string   `json:"destinationIP"`
	SourceIP      string   `json:"sourceIP"`
	Process       string   `json:"process"`
	Outbound      string   `json:"outbound"`
	Chains        []string `json:"chains"`
	RuleGroup     string   `json:"ruleGroup"`
	DNSMode       string   `json:"dnsMode"`
	Network       string   `json:"network"`
	Upload        int64    `json:"upload"`
	Download      int64    `json:"download"`
	Total         int64    `json:"total"`
	Count         int64    `json:"count"`
}

// StorageStats reports SQLite disk usage and record bounds for Data Management.
type StorageStats struct {
	DBSizeBytes     int64 `json:"dbSizeBytes"`
	WALSizeBytes    int64 `json:"walSizeBytes"`
	DetailRowCount  int64 `json:"detailRowCount"`
	SummaryRowCount int64 `json:"summaryRowCount"`
	OldestDetailTS  int64 `json:"oldestDetailTs"`
	OldestSummaryTS int64 `json:"oldestSummaryTs"`
	NewestTS        int64 `json:"newestTs"`
}

// VacuumResult summarizes space reclaimed by VACUUM + WAL checkpoint.
type VacuumResult struct {
	BeforeBytes int64 `json:"beforeBytes"`
	AfterBytes  int64 `json:"afterBytes"`
}

// PruneResult reports how many rows were removed by a custom cleanup operation.
type PruneResult struct {
	DetailDeleted  int64 `json:"detailDeleted"`
	SummaryDeleted int64 `json:"summaryDeleted"`
}

// FlushAggregatedEntries writes a batch of minute-bucketed records into traffic_aggregated.
func FlushAggregatedEntries(db *sql.DB, entries []AggregatedEntry) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`
		INSERT INTO traffic_aggregated
		(backend_id, bucket_start, bucket_end, source_ip, host, destination_ip, process, outbound, chains, rule_group, dns_mode, network, upload, download, count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(backend_id, bucket_start, bucket_end, source_ip, host, destination_ip, process, outbound, chains, rule_group, dns_mode, network)
		DO UPDATE SET
			upload = traffic_aggregated.upload + excluded.upload,
			download = traffic_aggregated.download + excluded.download,
			count = traffic_aggregated.count + excluded.count
	`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, e := range entries {
		if _, err := stmt.Exec(
			e.BackendID, e.BucketStart, e.BucketEnd, e.SourceIP, e.Host, e.DestinationIP,
			e.Process, e.Outbound, e.Chains, e.RuleGroup, e.DNSMode, e.Network,
			e.Upload, e.Download, e.Count,
		); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// BackfillSummary rolls completed daily buckets from traffic_aggregated into
// traffic_summary (kept permanently so full historical summary is always available).
func BackfillSummary(db *sql.DB, beforeMS int64) error {
	if beforeMS <= 0 {
		return nil
	}
	var lastBucketEnd sql.NullInt64
	if err := db.QueryRow(`SELECT MAX(bucket_end) FROM traffic_summary`).Scan(&lastBucketEnd); err != nil {
		return err
	}
	startMS := int64(0)
	if lastBucketEnd.Valid {
		startMS = lastBucketEnd.Int64
	}
	endExclusive := (beforeMS / config.SummaryBucketSizeMS) * config.SummaryBucketSizeMS
	if startMS >= endExclusive {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	dimQueries := []struct {
		dimension string
		labelExpr string
		secExpr   string
		groupBy   string
	}{
		{"process", "process", "host", "backend_id, process, host, dns_mode"},
		{"source_ip", "source_ip", "host", "backend_id, source_ip, host, dns_mode"},
		{"outbound", "outbound", "host", "backend_id, outbound, host, dns_mode"},
		{"rule", "rule_group", "host", "backend_id, rule_group, host, dns_mode"},
		{"dns_mode", "dns_mode", "host", "backend_id, dns_mode, host"},
		{"total", "'total'", "''", "backend_id, dns_mode"},
	}

	bSize := config.SummaryBucketSizeMS
	for _, dq := range dimQueries {
		query := fmt.Sprintf(`
			INSERT INTO traffic_summary
			(backend_id, bucket_start, bucket_end, dimension, label, secondary_label, dns_mode, upload, download, count)
			SELECT backend_id,
			       ((bucket_start / ?) * ?) AS b_start,
			       ((bucket_start / ?) * ?) + ? AS b_end,
			       ?,
			       %s,
			       %s,
			       dns_mode,
			       COALESCE(SUM(upload), 0),
			       COALESCE(SUM(download), 0),
			       COALESCE(SUM(count), 0)
			FROM traffic_aggregated
			WHERE bucket_start >= ? AND bucket_start < ?
			GROUP BY ((bucket_start / ?) * ?), %s
			ON CONFLICT(backend_id, bucket_start, bucket_end, dimension, label, secondary_label, dns_mode)
			DO UPDATE SET
				upload = excluded.upload,
				download = excluded.download,
				count = excluded.count
		`, dq.labelExpr, dq.secExpr, dq.groupBy)

		if _, err := tx.Exec(query, bSize, bSize, bSize, bSize, bSize, dq.dimension, startMS, endExclusive, bSize, bSize); err != nil {
			tx.Rollback()
			return fmt.Errorf("backfill summary dimension %s: %w", dq.dimension, err)
		}
	}

	return tx.Commit()
}

// ClearTraffic deletes traffic rows for one backend (backendID > 0) or all backends (backendID == 0).
func ClearTraffic(db *sql.DB, backendID int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if backendID > 0 {
		if _, err := tx.Exec(`DELETE FROM traffic_aggregated WHERE backend_id = ?`, backendID); err != nil {
			tx.Rollback()
			return err
		}
		if _, err := tx.Exec(`DELETE FROM traffic_summary WHERE backend_id = ?`, backendID); err != nil {
			tx.Rollback()
			return err
		}
	} else {
		if _, err := tx.Exec(`DELETE FROM traffic_aggregated`); err != nil {
			tx.Rollback()
			return err
		}
		if _, err := tx.Exec(`DELETE FROM traffic_summary`); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// PruneTrafficBefore deletes records older than beforeMS.
// scope: "detail" keeps permanent daily summaries; "all" removes both detail and summary rows.
func PruneTrafficBefore(db *sql.DB, beforeMS int64, scope string, backendID int64) (PruneResult, error) {
	tx, err := db.Begin()
	if err != nil {
		return PruneResult{}, err
	}

	var result PruneResult
	var res sql.Result
	if backendID > 0 {
		res, err = tx.Exec(`DELETE FROM traffic_aggregated WHERE backend_id = ? AND bucket_end <= ?`, backendID, beforeMS)
	} else {
		res, err = tx.Exec(`DELETE FROM traffic_aggregated WHERE bucket_end <= ?`, beforeMS)
	}
	if err != nil {
		tx.Rollback()
		return PruneResult{}, err
	}
	result.DetailDeleted, _ = res.RowsAffected()

	if scope == "all" {
		if backendID > 0 {
			res, err = tx.Exec(`DELETE FROM traffic_summary WHERE backend_id = ? AND bucket_end <= ?`, backendID, beforeMS)
		} else {
			res, err = tx.Exec(`DELETE FROM traffic_summary WHERE bucket_end <= ?`, beforeMS)
		}
		if err != nil {
			tx.Rollback()
			return PruneResult{}, err
		}
		result.SummaryDeleted, _ = res.RowsAffected()
	}

	if err := tx.Commit(); err != nil {
		return PruneResult{}, err
	}
	_, _ = db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	return result, nil
}

// CleanupExpiredDetails prunes only minute-level traffic_aggregated rows older
// than retentionDays while keeping daily traffic_summary permanently.
func CleanupExpiredDetails(db *sql.DB, nowMS int64, retentionDays int, runVacuum bool) error {
	if retentionDays < 1 {
		retentionDays = config.DefaultRetentionDays
	}
	aggCutoff := nowMS - int64(retentionDays)*86400000
	if _, err := db.Exec(`DELETE FROM traffic_aggregated WHERE bucket_end < ?`, aggCutoff); err != nil {
		return err
	}
	_, _ = db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	if runVacuum {
		_, _ = db.Exec(`VACUUM`)
	}
	return nil
}

// GetStorageStats returns SQLite file sizes and row counts.
func GetStorageStats(db *sql.DB, dbPath string) (StorageStats, error) {
	var stats StorageStats
	if dbPath != "" && dbPath != ":memory:" {
		if fi, err := os.Stat(dbPath); err == nil {
			stats.DBSizeBytes = fi.Size()
		}
		if fi, err := os.Stat(dbPath + "-wal"); err == nil {
			stats.WALSizeBytes = fi.Size()
		}
	}

	var oldestDetail, newestDetail sql.NullInt64
	if err := db.QueryRow(`SELECT COUNT(*), MIN(bucket_start), MAX(bucket_end) FROM traffic_aggregated`).
		Scan(&stats.DetailRowCount, &oldestDetail, &newestDetail); err != nil {
		return StorageStats{}, err
	}
	if oldestDetail.Valid {
		stats.OldestDetailTS = oldestDetail.Int64
	}
	if newestDetail.Valid {
		stats.NewestTS = newestDetail.Int64
	}

	var oldestSummary, newestSummary sql.NullInt64
	if err := db.QueryRow(`SELECT COUNT(*), MIN(bucket_start), MAX(bucket_end) FROM traffic_summary`).
		Scan(&stats.SummaryRowCount, &oldestSummary, &newestSummary); err != nil {
		return StorageStats{}, err
	}
	if oldestSummary.Valid {
		stats.OldestSummaryTS = oldestSummary.Int64
	}
	if newestSummary.Valid && newestSummary.Int64 > stats.NewestTS {
		stats.NewestTS = newestSummary.Int64
	}
	return stats, nil
}

// VacuumDatabase checkpoints WAL and runs SQLite VACUUM to reclaim unused disk space.
func VacuumDatabase(db *sql.DB, dbPath string) (VacuumResult, error) {
	before := totalDBFileBytes(dbPath)
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return VacuumResult{}, err
	}
	if _, err := db.Exec(`VACUUM`); err != nil {
		return VacuumResult{}, err
	}
	_, _ = db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	after := totalDBFileBytes(dbPath)
	return VacuumResult{BeforeBytes: before, AfterBytes: after}, nil
}

func totalDBFileBytes(dbPath string) int64 {
	if dbPath == "" || dbPath == ":memory:" {
		return 0
	}
	var total int64
	for _, p := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		if fi, err := os.Stat(p); err == nil {
			total += fi.Size()
		}
	}
	return total
}

// ParseChains unmarshals a JSON-encoded chains column cleanly.
func ParseChains(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{mihomo.DirectOutboundName}
	}
	var chains []string
	if err := json.Unmarshal([]byte(raw), &chains); err != nil {
		return []string{raw}
	}
	return mihomo.SanitizeChains(chains)
}
