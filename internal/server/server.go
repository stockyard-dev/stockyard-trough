package server

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/stockyard-dev/stockyard-trough/internal/store"
)

type Server struct {
	db       *store.DB
	mux      *http.ServeMux
	port     int
	adminKey string
	client   *http.Client

	mu      sync.RWMutex
	proxies map[string]*httputil.ReverseProxy // upstream_id → proxy
	upMap   map[string]*store.Upstream        // upstream_id → upstream
	limits  Limits
}

func New(db *store.DB, port int, adminKey string, limits Limits) *Server {
	s := &Server{
		db:       db,
		mux:      http.NewServeMux(),
		port:     port,
		adminKey: adminKey,
		client:   &http.Client{Timeout: 120 * time.Second},
		proxies:  make(map[string]*httputil.ReverseProxy),
		upMap:    make(map[string]*store.Upstream),
		limits:   limits,
	}
	s.loadUpstreams()
	s.routes()
	go s.alertLoop()
	return s
}

func (s *Server) loadUpstreams() {
	ups, err := s.db.ListUpstreams()
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range ups {
		s.addProxyLocked(&ups[i])
	}
}

func (s *Server) addProxyLocked(u *store.Upstream) {
	target, err := url.Parse(u.BaseURL)
	if err != nil {
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("[trough] upstream %s error: %v", u.Name, err)
		w.WriteHeader(502)
		w.Write([]byte(`{"error":"upstream unavailable"}`))
	}
	s.proxies[u.ID] = proxy
	cp := *u
	s.upMap[u.ID] = &cp
}

func (s *Server) routes() {
	// Admin API
	s.mux.HandleFunc("GET /api/upstreams", s.admin(s.handleListUpstreams))
	s.mux.HandleFunc("POST /api/upstreams", s.admin(s.handleCreateUpstream))
	s.mux.HandleFunc("DELETE /api/upstreams/{id}", s.admin(s.handleDeleteUpstream))

	s.mux.HandleFunc("GET /api/upstreams/{id}/rules", s.admin(s.handleListRules))
	s.mux.HandleFunc("POST /api/upstreams/{id}/rules", s.admin(s.handleCreateRule))
	s.mux.HandleFunc("DELETE /api/upstreams/{id}/rules/{rid}", s.admin(s.handleDeleteRule))

	s.mux.HandleFunc("GET /api/requests", s.admin(s.handleListRequests))
	s.mux.HandleFunc("GET /api/spend", s.admin(s.handleSpend))
	s.mux.HandleFunc("GET /api/export.csv", s.admin(s.handleExportCSV))

	s.mux.HandleFunc("GET /api/alerts", s.admin(s.handleListAlerts))
	s.mux.HandleFunc("POST /api/alerts", s.admin(s.handleCreateAlert))
	s.mux.HandleFunc("DELETE /api/alerts/{id}", s.admin(s.handleDeleteAlert))

	s.mux.HandleFunc("GET /api/stats", s.admin(s.handleStats))
	s.mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok"})
	})

	// Proxy routes: /{upstream_id}/{path...}
	s.mux.HandleFunc("/proxy/", s.handleProxy)
}

func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	log.Printf("[trough] listening on %s", addr)
	srv := &http.Server{
		Addr:         addr,
		Handler:      s.mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
	}
	return srv.ListenAndServe()
}

// ── Auth ──────────────────────────────────────────────────────────────

func (s *Server) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.adminKey == "" {
			next(w, r)
			return
		}
		key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if key == "" {
			key = r.URL.Query().Get("key")
		}
		if key != s.adminKey {
			writeJSON(w, 401, map[string]string{"error": "admin key required"})
			return
		}
		next(w, r)
	}
}

// ── Proxy Handler ─────────────────────────────────────────────────────
// Route format: /proxy/{upstream_id}/{path...}

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	// Strip /proxy/ prefix and extract upstream_id
	stripped := strings.TrimPrefix(r.URL.Path, "/proxy/")
	parts := strings.SplitN(stripped, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, 400, map[string]string{"error": "upstream_id required in path: /proxy/{upstream_id}/..."})
		return
	}
	upstreamID := parts[0]
	subPath := "/"
	if len(parts) > 1 {
		subPath = "/" + parts[1]
	}

	s.mu.RLock()
	proxy, ok := s.proxies[upstreamID]
	upstream := s.upMap[upstreamID]
	s.mu.RUnlock()

	if !ok || upstream == nil || !upstream.Enabled {
		writeJSON(w, 404, map[string]string{"error": "upstream not found or disabled"})
		return
	}

	// Read request body size
	var reqBytes int
	if r.Body != nil {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 10<<20))
		reqBytes = len(body)
		r.Body = io.NopCloser(bytes.NewReader(body))
	}

	// Match cost rule
	costCents, ruleID := s.db.MatchCost(upstreamID, r.Method, subPath)

	// Rewrite path for upstream
	r.URL.Path = subPath
	r.URL.RawPath = subPath

	sourceIP := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		sourceIP = strings.Split(fwd, ",")[0]
	}

	start := time.Now()
	rec := &responseRecorder{ResponseWriter: w, status: 200}
	proxy.ServeHTTP(rec, r)
	latency := int(time.Since(start).Milliseconds())

	s.db.LogRequest(store.Request{
		UpstreamID:   upstreamID,
		UpstreamName: upstream.Name,
		Method:       r.Method,
		Path:         subPath,
		Status:       rec.status,
		LatencyMs:    latency,
		ReqBytes:     reqBytes,
		RespBytes:    rec.size,
		CostCents:    costCents,
		RuleID:       ruleID,
		SourceIP:     sourceIP,
	})

	if costCents > 0 {
		log.Printf("[trough] %s %s/%s → %d (%dms) cost=$%0.4f",
			r.Method, upstream.Name, subPath, rec.status, latency, float64(costCents)/100.0)
	}
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	size   int
}

func (r *responseRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
func (r *responseRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.size += n
	return n, err
}

// ── Upstream Handlers ─────────────────────────────────────────────────

func (s *Server) handleListUpstreams(w http.ResponseWriter, r *http.Request) {
	ups, err := s.db.ListUpstreams()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if ups == nil {
		ups = []store.Upstream{}
	}
	writeJSON(w, 200, map[string]any{"upstreams": ups, "count": len(ups)})
}

func (s *Server) handleCreateUpstream(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		BaseURL string `json:"base_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.BaseURL == "" {
		writeJSON(w, 400, map[string]string{"error": "name and base_url required"})
		return
	}
	if s.limits.MaxUpstreams > 0 {
		ups, _ := s.db.ListUpstreams()
		if LimitReached(s.limits.MaxUpstreams, len(ups)) {
			writeJSON(w, 402, map[string]string{"error": "free tier limit: " + strconv.Itoa(s.limits.MaxUpstreams) + " service max — upgrade to Pro", "upgrade": "https://stockyard.dev/trough/"})
			return
		}
	}
	u, err := s.db.CreateUpstream(req.Name, req.BaseURL)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "upstream name already exists"})
		return
	}
	s.mu.Lock()
	s.addProxyLocked(u)
	s.mu.Unlock()

	writeJSON(w, 201, map[string]any{
		"upstream": u,
		"proxy_url": fmt.Sprintf("http://localhost:%d/proxy/%s/", s.port, u.ID),
	})
}

func (s *Server) handleDeleteUpstream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.db.DeleteUpstream(id)
	s.mu.Lock()
	delete(s.proxies, id)
	delete(s.upMap, id)
	s.mu.Unlock()
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// ── Rule Handlers ─────────────────────────────────────────────────────

func (s *Server) handleListRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.db.ListRules(r.PathValue("id"))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if rules == nil {
		rules = []store.CostRule{}
	}
	writeJSON(w, 200, map[string]any{"rules": rules})
}

func (s *Server) handleCreateRule(w http.ResponseWriter, r *http.Request) {
	upstreamID := r.PathValue("id")
	var req struct {
		Method      string `json:"method"`
		PathPattern string `json:"path_pattern"`
		CostCents   int    `json:"cost_cents"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PathPattern == "" {
		writeJSON(w, 400, map[string]string{"error": "path_pattern and cost_cents required"})
		return
	}
	rule, err := s.db.CreateRule(upstreamID, req.Method, req.PathPattern, req.Description, req.CostCents)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]any{"rule": rule})
}

func (s *Server) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	s.db.DeleteRule(r.PathValue("rid"))
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// ── Request + Spend Handlers ──────────────────────────────────────────

func (s *Server) handleListRequests(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 100
	fmt.Sscanf(q.Get("limit"), "%d", &limit)
	reqs, total, err := s.db.ListRequests(store.RequestFilter{
		UpstreamID: q.Get("upstream_id"),
		Method:     q.Get("method"),
		From:       q.Get("from"),
		To:         q.Get("to"),
		Limit:      limit,
	})
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if reqs == nil {
		reqs = []store.Request{}
	}
	writeJSON(w, 200, map[string]any{"requests": reqs, "total": total, "count": len(reqs)})
}

func (s *Server) handleSpend(w http.ResponseWriter, r *http.Request) {
	days := 30
	fmt.Sscanf(r.URL.Query().Get("days"), "%d", &days)
	summary := s.db.SpendSummary(r.URL.Query().Get("upstream_id"), days)
	writeJSON(w, 200, summary)
}

func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rows, err := s.db.ExportCSV(q.Get("upstream_id"), q.Get("from"), q.Get("to"))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="trough-%s.csv"`, time.Now().Format("20060102")))
	csv.NewWriter(w).WriteAll(rows)
}

// ── Alert Handlers ────────────────────────────────────────────────────

func (s *Server) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	alerts, err := s.db.ListAlerts()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if alerts == nil {
		alerts = []store.Alert{}
	}
	writeJSON(w, 200, map[string]any{"alerts": alerts})
}

func (s *Server) handleCreateAlert(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UpstreamID     string `json:"upstream_id"`
		Period         string `json:"period"`
		ThresholdCents int    `json:"threshold_cents"`
		WebhookURL     string `json:"webhook_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.WebhookURL == "" || req.ThresholdCents <= 0 {
		writeJSON(w, 400, map[string]string{"error": "webhook_url and threshold_cents required"})
		return
	}
	a, err := s.db.CreateAlert(req.UpstreamID, req.Period, req.WebhookURL, req.ThresholdCents)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]any{"alert": a})
}

func (s *Server) handleDeleteAlert(w http.ResponseWriter, r *http.Request) {
	s.db.DeleteAlert(r.PathValue("id"))
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.db.Stats())
}

// ── Alert Loop ────────────────────────────────────────────────────────

func (s *Server) alertLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.checkAlerts()
	}
}

func (s *Server) checkAlerts() {
	alerts, err := s.db.ListAlerts()
	if err != nil {
		return
	}
	for _, a := range alerts {
		var since string
		now := time.Now()
		switch a.Period {
		case "daily":
			since = now.Format("2006-01-02")
		case "monthly":
			since = now.Format("2006-01") + "-01"
		default:
			since = now.Format("2006-01-02")
		}

		spent := s.db.SpendSince(a.UpstreamID, since)
		if spent < a.ThresholdCents {
			continue
		}

		// Throttle: don't fire same alert more than once per period window
		if a.LastFiredAt != "" {
			lastFired, err := time.Parse("2006-01-02 15:04:05", a.LastFiredAt)
			if err == nil {
				var window time.Duration
				if a.Period == "monthly" {
					window = 24 * time.Hour
				} else {
					window = 1 * time.Hour
				}
				if time.Since(lastFired) < window {
					continue
				}
			}
		}

		s.fireAlert(a, spent)
		s.db.MarkAlertFired(a.ID)
	}
}

func (s *Server) fireAlert(a store.Alert, spentCents int) {
	payload, _ := json.Marshal(map[string]any{
		"event":           "trough.spend_alert",
		"upstream_id":     a.UpstreamID,
		"period":          a.Period,
		"spent_cents":     spentCents,
		"threshold_cents": a.ThresholdCents,
		"fired_at":        time.Now().UTC().Format(time.RFC3339),
	})
	req, err := http.NewRequest("POST", a.WebhookURL, bytes.NewReader(payload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		log.Printf("[trough] alert webhook %s failed: %v", a.WebhookURL, err)
		return
	}
	resp.Body.Close()
	log.Printf("[trough] alert fired: upstream=%s spent=$%.2f threshold=$%.2f",
		a.UpstreamID, float64(spentCents)/100, float64(a.ThresholdCents)/100)
}

// ── Helpers ───────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
