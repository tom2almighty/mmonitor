package monitor

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"

	"mmonitor/internal/mihomo"
	"mmonitor/internal/storage"
)

func dimensionColumn(dimension string) (string, error) {
	switch dimension {
	case "process":
		return "process", nil
	case "host":
		return "host", nil
	case "outbound":
		return "outbound", nil
	case "rule":
		return "rule_group", nil
	case "sourceIP":
		return "source_ip", nil
	case "dnsMode":
		return "dns_mode", nil
	default:
		return "", fmt.Errorf("unsupported dimension %q", dimension)
	}
}

func summaryDimensionParts(dimension string) (string, string, error) {
	switch dimension {
	case "process":
		return "process", "label", nil
	case "sourceIP":
		return "source_ip", "label", nil
	case "host":
		return "source_ip", "secondary_label", nil
	case "outbound":
		return "outbound", "label", nil
	case "rule":
		return "rule", "label", nil
	case "dnsMode":
		return "dns_mode", "label", nil
	default:
		return "", "", fmt.Errorf("unsupported dimension %q", dimension)
	}
}

func dnsModeSQLClause(filter string) (string, []any) {
	switch strings.ToLower(strings.TrimSpace(filter)) {
	case "", "all":
		return "", nil
	case mihomo.DNSModeFakeIP:
		return "dns_mode IN (?, ?)", []any{mihomo.DNSModeFakeIP, mihomo.DNSModeFakeIPUnresolved}
	case "fake-ip-resolved":
		return "dns_mode = ?", []any{mihomo.DNSModeFakeIP}
	case mihomo.DNSModeFakeIPUnresolved:
		return "dns_mode = ?", []any{mihomo.DNSModeFakeIPUnresolved}
	case mihomo.DNSModeNormal:
		return "dns_mode = ?", []any{mihomo.DNSModeNormal}
	case mihomo.DNSModeRedirHost:
		return "dns_mode = ?", []any{mihomo.DNSModeRedirHost}
	default:
		return "dns_mode = ?", []any{filter}
	}
}

func backendSQLClause(backendID int64) (string, []any) {
	if backendID > 0 {
		return "backend_id = ?", []any{backendID}
	}
	return "", nil
}

func matchesBackend(entryBackendID, filterBackendID int64) bool {
	return filterBackendID <= 0 || entryBackendID == filterBackendID
}

func (s *Service) summaryCoveredEnd() (int64, error) {
	var lastBucketEnd sql.NullInt64
	if err := s.db.QueryRow(`SELECT MAX(bucket_end) FROM traffic_summary`).Scan(&lastBucketEnd); err != nil {
		return 0, err
	}
	if !lastBucketEnd.Valid {
		return 0, nil
	}
	return lastBucketEnd.Int64, nil
}

// earliestTimestamp returns the earliest recorded bucket_start across summary,
// detail, and in-memory buffer for "All Time" queries (start <= 0).
func (s *Service) earliestTimestamp(backendID int64) int64 {
	var minTS int64
	updateMin := func(v int64) {
		if v > 0 && (minTS == 0 || v < minTS) {
			minTS = v
		}
	}

	var sumMin sql.NullInt64
	if backendID > 0 {
		_ = s.db.QueryRow(`SELECT MIN(bucket_start) FROM traffic_summary WHERE backend_id = ?`, backendID).Scan(&sumMin)
	} else {
		_ = s.db.QueryRow(`SELECT MIN(bucket_start) FROM traffic_summary`).Scan(&sumMin)
	}
	if sumMin.Valid {
		updateMin(sumMin.Int64)
	}

	var aggMin sql.NullInt64
	if backendID > 0 {
		_ = s.db.QueryRow(`SELECT MIN(bucket_start) FROM traffic_aggregated WHERE backend_id = ?`, backendID).Scan(&aggMin)
	} else {
		_ = s.db.QueryRow(`SELECT MIN(bucket_start) FROM traffic_aggregated`).Scan(&aggMin)
	}
	if aggMin.Valid {
		updateMin(aggMin.Int64)
	}

	for _, e := range s.snapshotBuffer(func(e storage.AggregatedEntry) bool { return matchesBackend(e.BackendID, backendID) }) {
		updateMin(e.BucketStart)
	}
	return minTS
}

// QueryOverview computes high-level traffic KPIs including Fake-IP breakdown.
func (s *Service) QueryOverview(backendID, start, end int64, summary bool) (OverviewStats, error) {
	dnsRows, err := s.QueryAggregate(backendID, "dnsMode", start, end, true, summary, "")
	if err != nil {
		return OverviewStats{}, err
	}

	var stats OverviewStats
	for _, r := range dnsRows {
		stats.TotalUpload += r.Upload
		stats.TotalDownload += r.Download
		stats.TotalBytes += r.Total
		switch r.Label {
		case mihomo.DNSModeFakeIP:
			stats.FakeIPUpload += r.Upload
			stats.FakeIPDownload += r.Download
			stats.FakeIPBytes += r.Total
		case mihomo.DNSModeFakeIPUnresolved:
			stats.FakeIPUpload += r.Upload
			stats.FakeIPDownload += r.Download
			stats.FakeIPBytes += r.Total
			stats.UnresolvedFakeIPBytes += r.Total
		default:
			stats.NormalBytes += r.Total
		}
	}
	if stats.TotalBytes > 0 {
		stats.FakeIPRatio = float64(stats.FakeIPBytes) / float64(stats.TotalBytes)
	}

	if rows, err := s.QueryAggregate(backendID, "process", start, end, false, summary, ""); err == nil {
		stats.ProcessCount = len(rows)
	}
	if rows, err := s.QueryAggregate(backendID, "host", start, end, false, summary, ""); err == nil {
		stats.HostCount = len(rows)
	}
	if rows, err := s.QueryAggregate(backendID, "outbound", start, end, false, summary, ""); err == nil {
		stats.OutboundCount = len(rows)
	}
	if rows, err := s.QueryAggregate(backendID, "sourceIP", start, end, false, summary, ""); err == nil {
		stats.SourceIPCount = len(rows)
	}
	if rows, err := s.QueryAggregate(backendID, "rule", start, end, false, summary, ""); err == nil {
		stats.RuleCount = len(rows)
	}

	return stats, nil
}

// QueryAggregate aggregates traffic by dimension over [start, end].
// start == 0 queries all history up to end.
func (s *Service) QueryAggregate(backendID int64, dimension string, start, end int64, raw, summary bool, dnsModeFilter string) ([]storage.AggregatedRow, error) {
	column, err := dimensionColumn(dimension)
	if err != nil {
		return nil, err
	}

	var rows []storage.AggregatedRow
	if summary {
		rows, err = s.querySummaryAggregate(backendID, dimension, start, end, dnsModeFilter)
	} else {
		rows, err = s.queryDetailAggregate(backendID, column, "", nil, start, end, dnsModeFilter)
	}
	if err != nil {
		return nil, err
	}

	if column == "host" && !raw && s.CurrentPreferences().DomainGroupingEnabled {
		rows = groupHostRows(rows)
	}
	if column == "rule_group" {
		rows = hideUnknownRuleGroup(rows)
	}
	return rows, nil
}

// QuerySubstats returns secondary breakdown rows for a selected primary item.
func (s *Service) QuerySubstats(backendID int64, dimension, label string, start, end int64, summary bool, dnsModeFilter string) ([]storage.AggregatedRow, error) {
	column, err := dimensionColumn(dimension)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(label) == "" {
		return nil, errors.New("label is required")
	}

	if summary {
		return s.querySummarySubstats(backendID, dimension, label, start, end, dnsModeFilter)
	}

	if column == "host" {
		if s.CurrentPreferences().DomainGroupingEnabled && net.ParseIP(label) == nil {
			filter, args := s.hostFilterArgs(label)
			return s.queryDetailAggregate(backendID, "host", filter, args, start, end, dnsModeFilter)
		}
		return s.queryDetailAggregate(backendID, "process", "host = ?", []any{label}, start, end, dnsModeFilter)
	}

	return s.queryDetailAggregate(backendID, "host", column+" = ?", []any{label}, start, end, dnsModeFilter)
}

// QueryConnectionDetails returns grouped connection chains for a primary + secondary drilldown pair.
func (s *Service) QueryConnectionDetails(backendID int64, dimension, primary, secondary string, start, end int64, dnsModeFilter string) ([]storage.ConnectionDetail, error) {
	filter, args, err := detailFilter(dimension, primary, secondary)
	if err != nil {
		return nil, err
	}

	if dimension == "host" && s.CurrentPreferences().DomainGroupingEnabled {
		if net.ParseIP(primary) != nil {
			filter = "host = ? AND process = ?"
			args = []any{primary, secondary}
		} else {
			filter = "host = ?"
			args = []any{secondary}
		}
	}

	whereParts := []string{"bucket_end > ?", "bucket_start <= ?", filter}
	queryArgs := append([]any{start, end}, args...)
	if bClause, bArgs := backendSQLClause(backendID); bClause != "" {
		whereParts = append(whereParts, bClause)
		queryArgs = append(queryArgs, bArgs...)
	}
	if dnsClause, dnsArgs := dnsModeSQLClause(dnsModeFilter); dnsClause != "" {
		whereParts = append(whereParts, dnsClause)
		queryArgs = append(queryArgs, dnsArgs...)
	}

	rows, err := s.db.Query(`
		SELECT destination_ip,
		       source_ip,
		       process,
		       outbound,
		       chains,
		       rule_group,
		       dns_mode,
		       network,
		       COALESCE(SUM(upload), 0),
		       COALESCE(SUM(download), 0),
		       COALESCE(SUM(upload + download), 0),
		       COALESCE(SUM(count), 0)
		FROM traffic_aggregated
		WHERE `+strings.Join(whereParts, " AND ")+`
		GROUP BY destination_ip, source_ip, process, outbound, chains, rule_group, dns_mode, network
	`, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	merged := make(map[string]*storage.ConnectionDetail)
	for rows.Next() {
		var item storage.ConnectionDetail
		var chainsRaw string
		if err := rows.Scan(
			&item.DestinationIP,
			&item.SourceIP,
			&item.Process,
			&item.Outbound,
			&chainsRaw,
			&item.RuleGroup,
			&item.DNSMode,
			&item.Network,
			&item.Upload,
			&item.Download,
			&item.Total,
			&item.Count,
		); err != nil {
			return nil, err
		}
		item.Chains = storage.ParseChains(chainsRaw)
		mergeConnectionDetail(merged, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	bufEntries := s.snapshotBuffer(func(e storage.AggregatedEntry) bool {
		return matchesBackend(e.BackendID, backendID) &&
			e.BucketEnd > start && e.BucketStart <= end &&
			matchesDNSModeFilter(e.DNSMode, dnsModeFilter) &&
			matchesEntryFilter(e, filter, args)
	})
	for _, e := range bufEntries {
		mergeConnectionDetail(merged, storage.ConnectionDetail{
			DestinationIP: e.DestinationIP,
			SourceIP:      e.SourceIP,
			Process:       e.Process,
			Outbound:      e.Outbound,
			Chains:        storage.ParseChains(e.Chains),
			RuleGroup:     e.RuleGroup,
			DNSMode:       e.DNSMode,
			Network:       e.Network,
			Upload:        e.Upload,
			Download:      e.Download,
			Total:         e.Upload + e.Download,
			Count:         e.Count,
		})
	}

	return sortedConnectionDetails(merged), nil
}

// QueryTrend returns time-bucketed upload/download series over [start, end].
// When start <= 0 (all-time summary), start is automatically clamped to the earliest record.
func (s *Service) QueryTrend(backendID, start, end, bucket int64, summary bool, dnsModeFilter string) ([]storage.TrendPoint, error) {
	if bucket <= 0 {
		return nil, errors.New("bucket must be positive")
	}

	effectiveStart := start
	if effectiveStart <= 0 {
		earliest := s.earliestTimestamp(backendID)
		if earliest == 0 {
			return []storage.TrendPoint{}, nil
		}
		effectiveStart = earliest
		if end-effectiveStart > 365*86400000 && bucket < 7*86400000 {
			bucket = 7 * 86400000
		} else if bucket < 86400000 {
			bucket = 86400000
		}
	}

	buckets := make(map[int64]storage.TrendPoint)

	if summary {
		detailStart, err := s.summaryCoveredEnd()
		if err != nil {
			return nil, err
		}
		fallbackStart := detailStart
		if fallbackStart < effectiveStart {
			fallbackStart = effectiveStart
		}

		whereParts := []string{"dimension = 'total'", "bucket_end > ?", "bucket_start <= ?"}
		args := []any{bucket, bucket, effectiveStart, end}
		if bClause, bArgs := backendSQLClause(backendID); bClause != "" {
			whereParts = append(whereParts, bClause)
			args = append(args, bArgs...)
		}
		if dnsClause, dnsArgs := dnsModeSQLClause(dnsModeFilter); dnsClause != "" {
			whereParts = append(whereParts, dnsClause)
			args = append(args, dnsArgs...)
		}

		rows, err := s.db.Query(`
			SELECT ((bucket_start / ?) * ?) AS b_start,
			       COALESCE(SUM(upload), 0),
			       COALESCE(SUM(download), 0)
			FROM traffic_summary
			WHERE `+strings.Join(whereParts, " AND ")+`
			GROUP BY b_start
			ORDER BY b_start ASC
		`, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var p storage.TrendPoint
			if err := rows.Scan(&p.Timestamp, &p.Upload, &p.Download); err != nil {
				return nil, err
			}
			mergeTrendPoint(buckets, p)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}

		if fallbackStart <= end {
			detailPts, err := s.queryTrendFromAggregates(backendID, fallbackStart, end, bucket, dnsModeFilter)
			if err != nil {
				return nil, err
			}
			for _, p := range detailPts {
				mergeTrendPoint(buckets, p)
			}
		}
	} else {
		detailPts, err := s.queryTrendFromAggregates(backendID, effectiveStart, end, bucket, dnsModeFilter)
		if err != nil {
			return nil, err
		}
		for _, p := range detailPts {
			mergeTrendPoint(buckets, p)
		}
	}

	bufEntries := s.snapshotBuffer(func(e storage.AggregatedEntry) bool {
		return matchesBackend(e.BackendID, backendID) &&
			e.BucketEnd > effectiveStart && e.BucketStart <= end &&
			matchesDNSModeFilter(e.DNSMode, dnsModeFilter)
	})
	for _, e := range bufEntries {
		ts := (e.BucketStart / bucket) * bucket
		mergeTrendPoint(buckets, storage.TrendPoint{
			Timestamp: ts,
			Upload:    e.Upload,
			Download:  e.Download,
		})
	}

	alignedStart := (effectiveStart / bucket) * bucket
	points := make([]storage.TrendPoint, 0, (end-alignedStart)/bucket+2)
	for t := alignedStart; t <= end; t += bucket {
		if pt, ok := buckets[t]; ok {
			points = append(points, pt)
		} else {
			points = append(points, storage.TrendPoint{Timestamp: t})
		}
	}
	return points, nil
}

func (s *Service) queryTrendFromAggregates(backendID, start, end, bucket int64, dnsModeFilter string) ([]storage.TrendPoint, error) {
	whereParts := []string{"bucket_end > ?", "bucket_start <= ?"}
	args := []any{bucket, bucket, start, end}
	if bClause, bArgs := backendSQLClause(backendID); bClause != "" {
		whereParts = append(whereParts, bClause)
		args = append(args, bArgs...)
	}
	if dnsClause, dnsArgs := dnsModeSQLClause(dnsModeFilter); dnsClause != "" {
		whereParts = append(whereParts, dnsClause)
		args = append(args, dnsArgs...)
	}

	rows, err := s.db.Query(`
		SELECT ((bucket_start / ?) * ?) AS b_start,
		       COALESCE(SUM(upload), 0),
		       COALESCE(SUM(download), 0)
		FROM traffic_aggregated
		WHERE `+strings.Join(whereParts, " AND ")+`
		GROUP BY b_start
		ORDER BY b_start ASC
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	pts := make([]storage.TrendPoint, 0)
	for rows.Next() {
		var p storage.TrendPoint
		if err := rows.Scan(&p.Timestamp, &p.Upload, &p.Download); err != nil {
			return nil, err
		}
		pts = append(pts, p)
	}
	return pts, rows.Err()
}

func (s *Service) queryDetailAggregate(backendID int64, groupColumn, extraFilter string, extraArgs []any, start, end int64, dnsModeFilter string) ([]storage.AggregatedRow, error) {
	merged := make(map[string]*storage.AggregatedRow)

	dbRows, err := s.queryAggregatesTable(backendID, groupColumn, extraFilter, extraArgs, start, end, dnsModeFilter)
	if err != nil {
		return nil, err
	}
	mergeAggregatedRows(merged, dbRows)

	bufEntries := s.snapshotBuffer(func(e storage.AggregatedEntry) bool {
		return matchesBackend(e.BackendID, backendID) &&
			e.BucketEnd > start && e.BucketStart <= end &&
			matchesDNSModeFilter(e.DNSMode, dnsModeFilter) &&
			matchesEntryFilter(e, extraFilter, extraArgs)
	})
	for _, e := range bufEntries {
		label := entryColumnValue(e, groupColumn)
		if label == "" {
			continue
		}
		mergeAggregatedRows(merged, []storage.AggregatedRow{{
			Label:    label,
			Upload:   e.Upload,
			Download: e.Download,
			Total:    e.Upload + e.Download,
			Count:    e.Count,
		}})
	}

	return sortedAggregatedRows(merged), nil
}

func (s *Service) queryAggregatesTable(backendID int64, groupColumn, extraFilter string, extraArgs []any, start, end int64, dnsModeFilter string) ([]storage.AggregatedRow, error) {
	fromClause := "traffic_aggregated"
	if extraFilter == "" && dnsModeFilter == "" {
		if backendID > 0 {
			fromClause += " INDEXED BY idx_traffic_agg_backend_bucket"
		} else {
			fromClause += " INDEXED BY idx_traffic_agg_bucket"
		}
	}

	whereParts := []string{"bucket_end > ?", "bucket_start <= ?"}
	args := []any{start, end}
	if bClause, bArgs := backendSQLClause(backendID); bClause != "" {
		whereParts = append(whereParts, bClause)
		args = append(args, bArgs...)
	}
	if extraFilter != "" {
		whereParts = append(whereParts, extraFilter)
		args = append(args, extraArgs...)
	}
	if dnsClause, dnsArgs := dnsModeSQLClause(dnsModeFilter); dnsClause != "" {
		whereParts = append(whereParts, dnsClause)
		args = append(args, dnsArgs...)
	}

	query := `
		SELECT ` + groupColumn + ` AS label,
		       COALESCE(SUM(upload), 0),
		       COALESCE(SUM(download), 0),
		       COALESCE(SUM(upload + download), 0),
		       COALESCE(SUM(count), 0)
		FROM ` + fromClause + `
		WHERE ` + strings.Join(whereParts, " AND ") + `
		GROUP BY ` + groupColumn + `
		ORDER BY 4 DESC, 1 ASC
	`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]storage.AggregatedRow, 0)
	for rows.Next() {
		var r storage.AggregatedRow
		if err := rows.Scan(&r.Label, &r.Upload, &r.Download, &r.Total, &r.Count); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *Service) querySummaryAggregate(backendID int64, dimension string, start, end int64, dnsModeFilter string) ([]storage.AggregatedRow, error) {
	sumDim, labelCol, err := summaryDimensionParts(dimension)
	if err != nil {
		return nil, err
	}
	col, err := dimensionColumn(dimension)
	if err != nil {
		return nil, err
	}

	merged := make(map[string]*storage.AggregatedRow)
	sumRows, err := s.querySummaryTable(backendID, sumDim, labelCol, "", nil, start, end, dnsModeFilter)
	if err != nil {
		return nil, err
	}
	mergeAggregatedRows(merged, sumRows)

	detailStart, err := s.summaryCoveredEnd()
	if err != nil {
		return nil, err
	}
	fallbackStart := detailStart
	if fallbackStart < start {
		fallbackStart = start
	}
	if fallbackStart <= end {
		tailRows, err := s.queryAggregatesTable(backendID, col, "", nil, fallbackStart, end, dnsModeFilter)
		if err != nil {
			return nil, err
		}
		mergeAggregatedRows(merged, tailRows)
	}

	bufEntries := s.snapshotBuffer(func(e storage.AggregatedEntry) bool {
		return matchesBackend(e.BackendID, backendID) &&
			e.BucketEnd > start && e.BucketStart <= end &&
			matchesDNSModeFilter(e.DNSMode, dnsModeFilter)
	})
	for _, e := range bufEntries {
		label := entryColumnValue(e, col)
		if label == "" {
			continue
		}
		mergeAggregatedRows(merged, []storage.AggregatedRow{{
			Label:    label,
			Upload:   e.Upload,
			Download: e.Download,
			Total:    e.Upload + e.Download,
			Count:    e.Count,
		}})
	}

	return sortedAggregatedRows(merged), nil
}

func (s *Service) querySummarySubstats(backendID int64, dimension, label string, start, end int64, dnsModeFilter string) ([]storage.AggregatedRow, error) {
	var sumDim, labelCol, sumFilter string
	var sumArgs []any
	var aggCol, aggFilter string
	var aggArgs []any

	switch dimension {
	case "process":
		sumDim, labelCol, sumFilter, sumArgs = "process", "secondary_label", "label = ?", []any{label}
		aggCol, aggFilter, aggArgs = "host", "process = ?", []any{label}
	case "sourceIP":
		sumDim, labelCol, sumFilter, sumArgs = "source_ip", "secondary_label", "label = ?", []any{label}
		aggCol, aggFilter, aggArgs = "host", "source_ip = ?", []any{label}
	case "outbound":
		sumDim, labelCol, sumFilter, sumArgs = "outbound", "secondary_label", "label = ?", []any{label}
		aggCol, aggFilter, aggArgs = "host", "outbound = ?", []any{label}
	case "rule":
		sumDim, labelCol, sumFilter, sumArgs = "rule", "secondary_label", "label = ?", []any{label}
		aggCol, aggFilter, aggArgs = "host", "rule_group = ?", []any{label}
	case "dnsMode":
		sumDim, labelCol, sumFilter, sumArgs = "dns_mode", "secondary_label", "label = ?", []any{label}
		aggCol, aggFilter, aggArgs = "host", "dns_mode = ?", []any{label}
	case "host":
		if s.CurrentPreferences().DomainGroupingEnabled && net.ParseIP(label) == nil {
			sumDim, labelCol = "source_ip", "secondary_label"
			sumFilter = "(secondary_label = ? OR secondary_label LIKE ?)"
			sumArgs = []any{label, "%." + label}
			aggCol = "host"
			aggFilter, aggArgs = s.hostFilterArgs(label)
		} else {
			sumDim, labelCol, sumFilter, sumArgs = "process", "label", "secondary_label = ?", []any{label}
			aggCol, aggFilter, aggArgs = "process", "host = ?", []any{label}
		}
	default:
		return nil, fmt.Errorf("unsupported dimension %q", dimension)
	}

	merged := make(map[string]*storage.AggregatedRow)
	sumRows, err := s.querySummaryTable(backendID, sumDim, labelCol, sumFilter, sumArgs, start, end, dnsModeFilter)
	if err != nil {
		return nil, err
	}
	mergeAggregatedRows(merged, sumRows)

	detailStart, err := s.summaryCoveredEnd()
	if err != nil {
		return nil, err
	}
	fallbackStart := detailStart
	if fallbackStart < start {
		fallbackStart = start
	}
	if fallbackStart <= end {
		tailRows, err := s.queryAggregatesTable(backendID, aggCol, aggFilter, aggArgs, fallbackStart, end, dnsModeFilter)
		if err != nil {
			return nil, err
		}
		mergeAggregatedRows(merged, tailRows)
	}

	bufEntries := s.snapshotBuffer(func(e storage.AggregatedEntry) bool {
		return matchesBackend(e.BackendID, backendID) &&
			e.BucketEnd > start && e.BucketStart <= end &&
			matchesDNSModeFilter(e.DNSMode, dnsModeFilter) &&
			matchesEntryFilter(e, aggFilter, aggArgs)
	})
	for _, e := range bufEntries {
		val := entryColumnValue(e, aggCol)
		if val == "" {
			continue
		}
		mergeAggregatedRows(merged, []storage.AggregatedRow{{
			Label:    val,
			Upload:   e.Upload,
			Download: e.Download,
			Total:    e.Upload + e.Download,
			Count:    e.Count,
		}})
	}

	return sortedAggregatedRows(merged), nil
}

func (s *Service) querySummaryTable(backendID int64, dimension, labelCol, extraFilter string, extraArgs []any, start, end int64, dnsModeFilter string) ([]storage.AggregatedRow, error) {
	whereParts := []string{"dimension = ?", "bucket_end > ?", "bucket_start <= ?"}
	args := []any{dimension, start, end}
	if bClause, bArgs := backendSQLClause(backendID); bClause != "" {
		whereParts = append(whereParts, bClause)
		args = append(args, bArgs...)
	}
	if extraFilter != "" {
		whereParts = append(whereParts, extraFilter)
		args = append(args, extraArgs...)
	}
	if dnsClause, dnsArgs := dnsModeSQLClause(dnsModeFilter); dnsClause != "" {
		whereParts = append(whereParts, dnsClause)
		args = append(args, dnsArgs...)
	}

	query := `
		SELECT ` + labelCol + ` AS label,
		       COALESCE(SUM(upload), 0),
		       COALESCE(SUM(download), 0),
		       COALESCE(SUM(upload + download), 0),
		       COALESCE(SUM(count), 0)
		FROM traffic_summary
		WHERE ` + strings.Join(whereParts, " AND ") + `
		GROUP BY ` + labelCol + `
		ORDER BY 4 DESC, 1 ASC
	`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]storage.AggregatedRow, 0)
	for rows.Next() {
		var r storage.AggregatedRow
		if err := rows.Scan(&r.Label, &r.Upload, &r.Download, &r.Total, &r.Count); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *Service) hostFilterArgs(host string) (string, []any) {
	if s.CurrentPreferences().DomainGroupingEnabled && net.ParseIP(host) == nil {
		return "(host = ? OR host LIKE ?)", []any{host, "%." + host}
	}
	return "host = ?", []any{host}
}

func detailFilter(dimension, primary, secondary string) (string, []any, error) {
	switch dimension {
	case "process":
		return "process = ? AND host = ?", []any{primary, secondary}, nil
	case "sourceIP":
		return "source_ip = ? AND host = ?", []any{primary, secondary}, nil
	case "host":
		return "host = ? AND process = ?", []any{primary, secondary}, nil
	case "outbound":
		return "outbound = ? AND host = ?", []any{primary, secondary}, nil
	case "rule":
		return "rule_group = ? AND host = ?", []any{primary, secondary}, nil
	case "dnsMode":
		return "dns_mode = ? AND host = ?", []any{primary, secondary}, nil
	default:
		return "", nil, fmt.Errorf("unsupported dimension %q", dimension)
	}
}

func groupHostRows(rows []storage.AggregatedRow) []storage.AggregatedRow {
	merged := make(map[string]*storage.AggregatedRow, len(rows))
	for _, r := range rows {
		key := storage.NormalizeHost(r.Label)
		existing, ok := merged[key]
		if !ok {
			copyRow := r
			copyRow.Label = key
			merged[key] = &copyRow
			continue
		}
		existing.Upload += r.Upload
		existing.Download += r.Download
		existing.Total += r.Total
		existing.Count += r.Count
	}
	return sortedAggregatedRows(merged)
}

func hideUnknownRuleGroup(rows []storage.AggregatedRow) []storage.AggregatedRow {
	out := make([]storage.AggregatedRow, 0, len(rows))
	for _, r := range rows {
		if r.Label != mihomo.UnknownRuleGroup {
			out = append(out, r)
		}
	}
	return out
}

func matchesEntryFilter(entry storage.AggregatedEntry, filter string, args []any) bool {
	if filter == "" {
		return true
	}
	clauses := strings.Split(filter, " AND ")
	argIdx := 0
	for _, clause := range clauses {
		clause = strings.TrimSpace(clause)
		if argIdx >= len(args) {
			return false
		}
		if strings.HasPrefix(clause, "(") && strings.HasSuffix(clause, ")") {
			if argIdx+1 >= len(args) {
				return false
			}
			inner := clause[1 : len(clause)-1]
			parts := strings.SplitN(inner, " OR ", 2)
			if len(parts) != 2 {
				return false
			}
			col := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(parts[0]), "= ?"))
			val := entryColumnValue(entry, col)
			exact := fmt.Sprint(args[argIdx])
			pattern := fmt.Sprint(args[argIdx+1])
			if val != exact && !matchesLikePattern(val, pattern) {
				return false
			}
			argIdx += 2
			continue
		}
		col := strings.TrimSpace(strings.TrimSuffix(clause, "= ?"))
		if col == "" || entryColumnValue(entry, col) != fmt.Sprint(args[argIdx]) {
			return false
		}
		argIdx++
	}
	return argIdx == len(args)
}

func matchesLikePattern(value, pattern string) bool {
	if strings.HasPrefix(pattern, "%.") {
		return strings.HasSuffix(value, pattern[1:])
	}
	return value == pattern
}

func entryColumnValue(e storage.AggregatedEntry, col string) string {
	switch col {
	case "process":
		return e.Process
	case "source_ip":
		return e.SourceIP
	case "host":
		return e.Host
	case "destination_ip":
		return e.DestinationIP
	case "outbound":
		return e.Outbound
	case "chains":
		return e.Chains
	case "rule_group":
		return e.RuleGroup
	case "dns_mode":
		return e.DNSMode
	case "network":
		return e.Network
	default:
		return ""
	}
}

func mergeAggregatedRows(target map[string]*storage.AggregatedRow, items []storage.AggregatedRow) {
	for _, item := range items {
		existing, ok := target[item.Label]
		if !ok {
			cp := item
			target[item.Label] = &cp
			continue
		}
		existing.Upload += item.Upload
		existing.Download += item.Download
		existing.Total += item.Total
		existing.Count += item.Count
	}
}

func sortedAggregatedRows(items map[string]*storage.AggregatedRow) []storage.AggregatedRow {
	out := make([]storage.AggregatedRow, 0, len(items))
	for _, item := range items {
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Total == out[j].Total {
			return out[i].Label < out[j].Label
		}
		return out[i].Total > out[j].Total
	})
	return out
}

func mergeConnectionDetail(target map[string]*storage.ConnectionDetail, item storage.ConnectionDetail) {
	key := strings.Join([]string{
		item.DestinationIP,
		item.SourceIP,
		item.Process,
		item.Outbound,
		item.RuleGroup,
		item.DNSMode,
		item.Network,
		strings.Join(item.Chains, "\x1f"),
	}, "\x00")
	existing, ok := target[key]
	if !ok {
		cp := item
		target[key] = &cp
		return
	}
	existing.Upload += item.Upload
	existing.Download += item.Download
	existing.Total += item.Total
	existing.Count += item.Count
}

func sortedConnectionDetails(items map[string]*storage.ConnectionDetail) []storage.ConnectionDetail {
	out := make([]storage.ConnectionDetail, 0, len(items))
	for _, item := range items {
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Total == out[j].Total {
			if out[i].DestinationIP == out[j].DestinationIP {
				return out[i].Process < out[j].Process
			}
			return out[i].DestinationIP < out[j].DestinationIP
		}
		return out[i].Total > out[j].Total
	})
	return out
}

func mergeTrendPoint(target map[int64]storage.TrendPoint, item storage.TrendPoint) {
	existing := target[item.Timestamp]
	existing.Timestamp = item.Timestamp
	existing.Upload += item.Upload
	existing.Download += item.Download
	target[item.Timestamp] = existing
}
