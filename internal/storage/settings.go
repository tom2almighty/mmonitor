package storage

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"mmonitor/internal/config"
	"mmonitor/internal/mihomo"
)

// Backend represents a configured Mihomo controller instance.
type Backend struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	Secret    string `json:"-"`
	HasSecret bool   `json:"hasSecret"`
	Enabled   bool   `json:"enabled"`
	CreatedAt int64  `json:"createdAt"`
}

// Preferences holds user-configurable collector and display preferences.
type Preferences struct {
	DomainGroupingEnabled bool `json:"domainGroupingEnabled"`
	FilterDirectTraffic   bool `json:"filterDirectTraffic"`
	RetentionDays         int  `json:"retentionDays"`
}

func upsertSetting(execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}, key, value string) error {
	_, err := execer.Exec(`
		INSERT INTO app_settings (key, value)
		VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}

// ListBackends returns all configured Mihomo backends ordered by creation id.
func ListBackends(db *sql.DB) ([]Backend, error) {
	rows, err := db.Query(`SELECT id, name, url, secret, enabled, created_at FROM backends ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]Backend, 0)
	for rows.Next() {
		var b Backend
		var enabled int
		if err := rows.Scan(&b.ID, &b.Name, &b.URL, &b.Secret, &enabled, &b.CreatedAt); err != nil {
			return nil, err
		}
		b.Enabled = enabled == 1
		b.HasSecret = b.Secret != ""
		list = append(list, b)
	}
	return list, rows.Err()
}

// GetBackend returns a single backend by ID.
func GetBackend(db *sql.DB, id int64) (Backend, bool, error) {
	var b Backend
	var enabled int
	err := db.QueryRow(`SELECT id, name, url, secret, enabled, created_at FROM backends WHERE id = ?`, id).
		Scan(&b.ID, &b.Name, &b.URL, &b.Secret, &enabled, &b.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Backend{}, false, nil
	}
	if err != nil {
		return Backend{}, false, err
	}
	b.Enabled = enabled == 1
	b.HasSecret = b.Secret != ""
	return b, true, nil
}

// CreateBackend inserts a new Mihomo backend and returns the created record.
func CreateBackend(db *sql.DB, name, rawURL, secret string, enabled bool) (Backend, error) {
	norm := mihomo.Settings{URL: rawURL, Secret: secret}.Normalize()
	name = strings.TrimSpace(name)
	if name == "" {
		name = norm.URL
	}
	if norm.URL == "" {
		return Backend{}, errors.New("backend url is required")
	}
	now := time.Now().UnixMilli()
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	res, err := db.Exec(`
		INSERT INTO backends (name, url, secret, enabled, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, name, norm.URL, norm.Secret, enabledInt, now)
	if err != nil {
		return Backend{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Backend{}, err
	}
	return Backend{
		ID:        id,
		Name:      name,
		URL:       norm.URL,
		Secret:    norm.Secret,
		HasSecret: norm.Secret != "",
		Enabled:   enabled,
		CreatedAt: now,
	}, nil
}

// UpdateBackend modifies an existing backend. When clearSecret is false and
// secret is empty, the existing secret is preserved.
func UpdateBackend(db *sql.DB, id int64, name, rawURL, secret string, clearSecret bool, enabled bool) (Backend, error) {
	existing, found, err := GetBackend(db, id)
	if err != nil {
		return Backend{}, err
	}
	if !found {
		return Backend{}, errors.New("backend not found")
	}

	norm := mihomo.Settings{URL: rawURL, Secret: secret}.Normalize()
	if norm.URL == "" {
		return Backend{}, errors.New("backend url is required")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = existing.Name
	}
	finalSecret := existing.Secret
	if clearSecret {
		finalSecret = ""
	} else if norm.Secret != "" {
		finalSecret = norm.Secret
	}

	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	if _, err := db.Exec(`
		UPDATE backends SET name = ?, url = ?, secret = ?, enabled = ? WHERE id = ?
	`, name, norm.URL, finalSecret, enabledInt, id); err != nil {
		return Backend{}, err
	}

	return Backend{
		ID:        id,
		Name:      name,
		URL:       norm.URL,
		Secret:    finalSecret,
		HasSecret: finalSecret != "",
		Enabled:   enabled,
		CreatedAt: existing.CreatedAt,
	}, nil
}

// DeleteBackend removes a backend and optionally deletes its associated traffic data.
func DeleteBackend(db *sql.DB, id int64, deleteData bool) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM backends WHERE id = ?`, id); err != nil {
		tx.Rollback()
		return err
	}
	if deleteData {
		if _, err := tx.Exec(`DELETE FROM traffic_aggregated WHERE backend_id = ?`, id); err != nil {
			tx.Rollback()
			return err
		}
		if _, err := tx.Exec(`DELETE FROM traffic_summary WHERE backend_id = ?`, id); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// EnsureEnvBackend seeds a backend from MIHOMO_URL / MIHOMO_SECRET if provided
// and not yet present in the database.
func EnsureEnvBackend(db *sql.DB, envURL, envSecret string) error {
	norm := mihomo.Settings{URL: envURL, Secret: envSecret}.Normalize()
	if norm.URL == "" {
		return nil
	}
	backends, err := ListBackends(db)
	if err != nil {
		return err
	}
	for _, b := range backends {
		if b.URL == norm.URL {
			if norm.Secret != "" && b.Secret != norm.Secret {
				_, err = UpdateBackend(db, b.ID, b.Name, b.URL, norm.Secret, false, b.Enabled)
				return err
			}
			return nil
		}
	}
	_, err = CreateBackend(db, "Default Mihomo", norm.URL, norm.Secret, true)
	return err
}

// LoadPreferences reads user preferences with sensible defaults.
func LoadPreferences(db *sql.DB, defaultRetentionDays int) Preferences {
	if defaultRetentionDays < 1 {
		defaultRetentionDays = config.DefaultRetentionDays
	}
	prefs := Preferences{
		DomainGroupingEnabled: true,
		FilterDirectTraffic:   false,
		RetentionDays:         defaultRetentionDays,
	}

	rows, err := db.Query(`
		SELECT key, value FROM app_settings
		WHERE key IN ('domain_grouping_enabled', 'filter_direct_traffic', 'aggregate_retention_days')
	`)
	if err != nil {
		return prefs
	}
	defer rows.Close()

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		switch key {
		case "domain_grouping_enabled":
			prefs.DomainGroupingEnabled = value == "true"
		case "filter_direct_traffic":
			prefs.FilterDirectTraffic = value == "true"
		case "aggregate_retention_days":
			if days, err := strconv.Atoi(value); err == nil && days >= 1 && days <= 365 {
				prefs.RetentionDays = days
			}
		}
	}
	if err := rows.Err(); err != nil {
		return prefs
	}
	return prefs
}

// SavePreferences persists all user preferences atomically.
func SavePreferences(db *sql.DB, prefs Preferences) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	vals := map[string]string{
		"domain_grouping_enabled":  strconv.FormatBool(prefs.DomainGroupingEnabled),
		"filter_direct_traffic":    strconv.FormatBool(prefs.FilterDirectTraffic),
		"aggregate_retention_days": strconv.Itoa(prefs.RetentionDays),
	}
	for k, v := range vals {
		if err := upsertSetting(tx, k, v); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}
