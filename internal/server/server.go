package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"mmonitor/internal/mihomo"
	"mmonitor/internal/monitor"
	"mmonitor/internal/storage"
)

// Server wires HTTP API handlers and embedded SPA static assets.
type Server struct {
	svc        *monitor.Service
	staticFS   fs.FS
	licenseRaw []byte
	logoSVG    []byte
}

// New constructs an HTTP handler for mmonitor.
func New(svc *monitor.Service, staticFS fs.FS, licenseRaw, logoSVG []byte) http.Handler {
	s := &Server{
		svc:        svc,
		staticFS:   staticFS,
		licenseRaw: licenseRaw,
		logoSVG:    logoSVG,
	}
	return s.routes()
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /LICENSE", s.handleLicense)
	mux.HandleFunc("HEAD /LICENSE", s.handleLicense)
	mux.HandleFunc("GET /favicon.svg", s.handleFavicon)
	mux.HandleFunc("HEAD /favicon.svg", s.handleFavicon)

	// Multi-backend management
	mux.HandleFunc("GET /api/backends", s.handleListBackends)
	mux.HandleFunc("POST /api/backends", s.handleCreateBackend)
	mux.HandleFunc("PUT /api/backends/{id}", s.handleUpdateBackend)
	mux.HandleFunc("DELETE /api/backends/{id}", s.handleDeleteBackend)
	mux.HandleFunc("POST /api/backends/test", s.handleTestBackend)

	// Runtime status & live connections
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/connections/live", s.handleLiveConnections)
	mux.HandleFunc("DELETE /api/connections/live/{backendId}/{id}", s.handleCloseLiveConnection)
	mux.HandleFunc("DELETE /api/connections/live", s.handleCloseAllLiveConnections)

	// Settings & storage management
	mux.HandleFunc("GET /api/settings/preferences", s.handleGetPreferences)
	mux.HandleFunc("PUT /api/settings/preferences", s.handlePutPreferences)
	mux.HandleFunc("GET /api/storage/stats", s.handleStorageStats)
	mux.HandleFunc("POST /api/storage/vacuum", s.handleVacuumStorage)
	mux.HandleFunc("POST /api/storage/prune", s.handlePruneStorage)

	// Traffic analytics
	mux.HandleFunc("GET /api/traffic/overview", s.handleOverview)
	mux.HandleFunc("GET /api/traffic/aggregate", s.handleAggregate)
	mux.HandleFunc("GET /api/traffic/substats", s.handleSubstats)
	mux.HandleFunc("GET /api/traffic/details", s.handleConnectionDetails)
	mux.HandleFunc("GET /api/traffic/trend", s.handleTrend)
	mux.HandleFunc("DELETE /api/traffic/logs", s.handleClearLogs)

	// SPA static file server
	mux.Handle("/", s.spaHandler())

	return s.withCORS(mux)
}

func (s *Server) spaHandler() http.Handler {
	if s.staticFS == nil {
		return http.NotFoundHandler()
	}
	fileServer := http.FileServer(http.FS(s.staticFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusNotFound, errors.New("api endpoint not found"))
			return
		}
		cleanPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if cleanPath == "" || cleanPath == "." {
			cleanPath = "index.html"
		}
		if _, err := fs.Stat(s.staticFS, cleanPath); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		indexBytes, err := fs.ReadFile(s.staticFS, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(indexBytes))
	})
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	allowedOrigin := s.svc.Config().AllowedOrigin
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allowedOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleLicense(w http.ResponseWriter, r *http.Request) {
	if len(s.licenseRaw) == 0 {
		http.Error(w, "license unavailable", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	http.ServeContent(w, r, "LICENSE", time.Time{}, bytes.NewReader(s.licenseRaw))
}

func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	if len(s.logoSVG) == 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	http.ServeContent(w, r, "favicon.svg", time.Time{}, bytes.NewReader(s.logoSVG))
}

type backendRequest struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Secret      string `json:"secret"`
	ClearSecret bool   `json:"clearSecret"`
	Enabled     *bool  `json:"enabled"`
}

type testBackendRequest struct {
	BackendID   int64  `json:"backendId"`
	URL         string `json:"url"`
	Secret      string `json:"secret"`
	ClearSecret bool   `json:"clearSecret"`
}

func (s *Server) handleListBackends(w http.ResponseWriter, _ *http.Request) {
	list, err := s.svc.ListBackends()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreateBackend(w http.ResponseWriter, r *http.Request) {
	var req backendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid json body"))
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	created, err := s.svc.CreateBackend(req.Name, req.URL, req.Secret, enabled)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleUpdateBackend(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("invalid backend id"))
		return
	}
	var req backendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid json body"))
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	updated, err := s.svc.UpdateBackend(id, req.Name, req.URL, req.Secret, req.ClearSecret, enabled)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteBackend(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("invalid backend id"))
		return
	}
	deleteData := r.URL.Query().Get("deleteData") == "1"
	if err := s.svc.DeleteBackend(id, deleteData); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleTestBackend(w http.ResponseWriter, r *http.Request) {
	var req testBackendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid json body"))
		return
	}
	candidate := mihomo.Settings{URL: req.URL, Secret: req.Secret}.Normalize()
	res := s.svc.TestConnection(r.Context(), candidate, req.BackendID, req.ClearSecret)
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	backendID := parseInt64(r.URL.Query().Get("backendId"), 0)
	writeJSON(w, http.StatusOK, s.svc.LiveStatus(backendID))
}

func (s *Server) handleLiveConnections(w http.ResponseWriter, r *http.Request) {
	backendID := parseInt64(r.URL.Query().Get("backendId"), 0)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      s.svc.LiveStatus(backendID),
		"connections": s.svc.LiveConnections(backendID),
	})
}

func (s *Server) handleCloseLiveConnection(w http.ResponseWriter, r *http.Request) {
	backendID, err := strconv.ParseInt(r.PathValue("backendId"), 10, 64)
	if err != nil || backendID <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("invalid backend id"))
		return
	}
	id := r.PathValue("id")
	if err := s.svc.CloseLiveConnection(r.Context(), backendID, id); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleCloseAllLiveConnections(w http.ResponseWriter, r *http.Request) {
	backendID := parseInt64(r.URL.Query().Get("backendId"), 0)
	if err := s.svc.CloseAllLiveConnections(r.Context(), backendID); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleGetPreferences(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.svc.CurrentPreferences())
}

func (s *Server) handlePutPreferences(w http.ResponseWriter, r *http.Request) {
	var prefs storage.Preferences
	if err := json.NewDecoder(r.Body).Decode(&prefs); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid json body"))
		return
	}
	if prefs.RetentionDays < 1 || prefs.RetentionDays > 365 {
		writeError(w, http.StatusBadRequest, errors.New("retentionDays must be between 1 and 365"))
		return
	}
	if err := s.svc.UpdatePreferences(prefs); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, s.svc.CurrentPreferences())
}

func (s *Server) handleStorageStats(w http.ResponseWriter, _ *http.Request) {
	stats, err := s.svc.StorageStats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleVacuumStorage(w http.ResponseWriter, _ *http.Request) {
	res, err := s.svc.VacuumStorage()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type pruneRequest struct {
	BeforeTS  int64  `json:"beforeTs"`
	Scope     string `json:"scope"`
	BackendID int64  `json:"backendId"`
}

func (s *Server) handlePruneStorage(w http.ResponseWriter, r *http.Request) {
	var req pruneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid json body"))
		return
	}
	if req.BeforeTS <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("beforeTs must be positive"))
		return
	}
	if req.Scope != "detail" && req.Scope != "all" {
		req.Scope = "detail"
	}
	res, err := s.svc.PruneTraffic(req.BeforeTS, req.Scope, req.BackendID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	start, end, err := parseTimeRange(r, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	q := r.URL.Query()
	backendID := parseInt64(q.Get("backendId"), 0)
	summary := q.Get("summary") == "1"
	stats, err := s.svc.QueryOverview(backendID, start, end, summary)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleAggregate(w http.ResponseWriter, r *http.Request) {
	start, end, err := parseTimeRange(r, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	q := r.URL.Query()
	backendID := parseInt64(q.Get("backendId"), 0)
	dimension := q.Get("dimension")
	raw := q.Get("raw") == "1"
	summary := q.Get("summary") == "1"
	dnsMode := q.Get("dnsMode")

	data, err := s.svc.QueryAggregate(backendID, dimension, start, end, raw, summary, dnsMode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func (s *Server) handleSubstats(w http.ResponseWriter, r *http.Request) {
	start, end, err := parseTimeRange(r, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	q := r.URL.Query()
	backendID := parseInt64(q.Get("backendId"), 0)
	dimension := q.Get("dimension")
	label := q.Get("label")
	summary := q.Get("summary") == "1"
	dnsMode := q.Get("dnsMode")

	data, err := s.svc.QuerySubstats(backendID, dimension, label, start, end, summary, dnsMode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func (s *Server) handleConnectionDetails(w http.ResponseWriter, r *http.Request) {
	start, end, err := parseTimeRange(r, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	q := r.URL.Query()
	backendID := parseInt64(q.Get("backendId"), 0)
	dimension := q.Get("dimension")
	primary := q.Get("primary")
	secondary := q.Get("secondary")
	dnsMode := q.Get("dnsMode")
	if primary == "" || secondary == "" {
		writeError(w, http.StatusBadRequest, errors.New("primary and secondary are required"))
		return
	}
	if q.Get("summary") == "1" {
		writeJSON(w, http.StatusOK, []storage.ConnectionDetail{})
		return
	}

	data, err := s.svc.QueryConnectionDetails(backendID, dimension, primary, secondary, start, end, dnsMode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func (s *Server) handleTrend(w http.ResponseWriter, r *http.Request) {
	start, end, err := parseTimeRange(r, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	q := r.URL.Query()
	backendID := parseInt64(q.Get("backendId"), 0)
	bucket := parseInt64(q.Get("bucket"), 60000)
	if bucket <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("bucket must be positive"))
		return
	}
	summary := q.Get("summary") == "1"
	dnsMode := q.Get("dnsMode")

	data, err := s.svc.QueryTrend(backendID, start, end, bucket, summary, dnsMode)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func (s *Server) handleClearLogs(w http.ResponseWriter, r *http.Request) {
	backendID := parseInt64(r.URL.Query().Get("backendId"), 0)
	if err := s.svc.ClearTraffic(backendID); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func parseTimeRange(r *http.Request, allowZeroStart bool) (int64, int64, error) {
	start := parseInt64(r.URL.Query().Get("start"), 0)
	end := parseInt64(r.URL.Query().Get("end"), 0)
	if end <= 0 {
		return 0, 0, errors.New("end is required")
	}
	if !allowZeroStart && start <= 0 {
		return 0, 0, errors.New("start is required")
	}
	if start < 0 || end < start {
		return 0, 0, errors.New("invalid time range")
	}
	return start, end, nil
}

func parseInt64(value string, fallback int64) int64 {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
