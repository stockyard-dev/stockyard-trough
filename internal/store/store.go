package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct{ conn *sql.DB }

func Open(dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	conn, err := sql.Open("sqlite", filepath.Join(dataDir, "trough.db"))
	if err != nil {
		return nil, err
	}
	conn.Exec("PRAGMA journal_mode=WAL")
	conn.Exec("PRAGMA busy_timeout=5000")
	conn.SetMaxOpenConns(1)
	db := &DB{conn: conn}
	return db, db.migrate()
}

func (db *DB) Close() error { return db.conn.Close() }

func (db *DB) migrate() error {
	_, err := db.conn.Exec(`
CREATE TABLE IF NOT EXISTS upstreams (
    id          TEXT PRIMARY KEY,
    name        TEXT UNIQUE NOT NULL,
    base_url    TEXT NOT NULL,
    enabled     INTEGER DEFAULT 1,
    created_at  TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS cost_rules (
    id           TEXT PRIMARY KEY,
    upstream_id  TEXT NOT NULL REFERENCES upstreams(id),
    method       TEXT DEFAULT '*',
    path_pattern TEXT NOT NULL,
    cost_cents   INTEGER NOT NULL DEFAULT 0,
    description  TEXT DEFAULT '',
    enabled      INTEGER DEFAULT 1,
    created_at   TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_rules_upstream ON cost_rules(upstream_id);

CREATE TABLE IF NOT EXISTS requests (
    id           TEXT PRIMARY KEY,
    upstream_id  TEXT NOT NULL,
    upstream_name TEXT NOT NULL,
    method       TEXT NOT NULL,
    path         TEXT NOT NULL,
    status       INTEGER DEFAULT 0,
    latency_ms   INTEGER DEFAULT 0,
    req_bytes    INTEGER DEFAULT 0,
    resp_bytes   INTEGER DEFAULT 0,
    cost_cents   INTEGER DEFAULT 0,
    rule_id      TEXT DEFAULT '',
    source_ip    TEXT DEFAULT '',
    created_at   TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_req_upstream ON requests(upstream_id);
CREATE INDEX IF NOT EXISTS idx_req_time     ON requests(created_at);
CREATE INDEX IF NOT EXISTS idx_req_path     ON requests(path);

CREATE TABLE IF NOT EXISTS alerts (
    id             TEXT PRIMARY KEY,
    upstream_id    TEXT DEFAULT '',
    period         TEXT DEFAULT 'daily',
    threshold_cents INTEGER NOT NULL,
    webhook_url    TEXT NOT NULL,
    enabled        INTEGER DEFAULT 1,
    last_fired_at  TEXT DEFAULT '',
    created_at     TEXT DEFAULT (datetime('now'))
);
`)
	return err
}

// ── Upstreams ─────────────────────────────────────────────────────────

type Upstream struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	BaseURL   string `json:"base_url"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
}

func (db *DB) CreateUpstream(name, baseURL string) (*Upstream, error) {
	id := "up_" + genID(8)
	_, err := db.conn.Exec("INSERT INTO upstreams (id,name,base_url) VALUES (?,?,?)", id, name, baseURL)
	if err != nil {
		return nil, err
	}
	return &Upstream{ID: id, Name: name, BaseURL: baseURL, Enabled: true,
		CreatedAt: time.Now().UTC().Format(time.RFC3339)}, nil
}

func (db *DB) ListUpstreams() ([]Upstream, error) {
	rows, err := db.conn.Query("SELECT id,name,base_url,enabled,created_at FROM upstreams ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Upstream
	for rows.Next() {
		var u Upstream
		var en int
		rows.Scan(&u.ID, &u.Name, &u.BaseURL, &en, &u.CreatedAt)
		u.Enabled = en == 1
		out = append(out, u)
	}
	return out, rows.Err()
}

func (db *DB) GetUpstream(id string) (*Upstream, error) {
	var u Upstream
	var en int
	err := db.conn.QueryRow("SELECT id,name,base_url,enabled,created_at FROM upstreams WHERE id=?", id).
		Scan(&u.ID, &u.Name, &u.BaseURL, &en, &u.CreatedAt)
	u.Enabled = en == 1
	return &u, err
}

func (db *DB) DeleteUpstream(id string) error {
	db.conn.Exec("DELETE FROM cost_rules WHERE upstream_id=?", id)
	_, err := db.conn.Exec("DELETE FROM upstreams WHERE id=?", id)
	return err
}

// ── Cost Rules ────────────────────────────────────────────────────────

type CostRule struct {
	ID          string `json:"id"`
	UpstreamID  string `json:"upstream_id"`
	Method      string `json:"method"`
	PathPattern string `json:"path_pattern"`
	CostCents   int    `json:"cost_cents"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   string `json:"created_at"`
}

func (db *DB) CreateRule(upstreamID, method, pathPattern, description string, costCents int) (*CostRule, error) {
	id := "rule_" + genID(8)
	if method == "" {
		method = "*"
	}
	_, err := db.conn.Exec(
		"INSERT INTO cost_rules (id,upstream_id,method,path_pattern,cost_cents,description) VALUES (?,?,?,?,?,?)",
		id, upstreamID, method, pathPattern, costCents, description)
	if err != nil {
		return nil, err
	}
	return &CostRule{ID: id, UpstreamID: upstreamID, Method: method, PathPattern: pathPattern,
		CostCents: costCents, Description: description, Enabled: true,
		CreatedAt: time.Now().UTC().Format(time.RFC3339)}, nil
}

func (db *DB) ListRules(upstreamID string) ([]CostRule, error) {
	rows, err := db.conn.Query(
		"SELECT id,upstream_id,method,path_pattern,cost_cents,description,enabled,created_at FROM cost_rules WHERE upstream_id=? AND enabled=1",
		upstreamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CostRule
	for rows.Next() {
		var r CostRule
		var en int
		rows.Scan(&r.ID, &r.UpstreamID, &r.Method, &r.PathPattern, &r.CostCents, &r.Description, &en, &r.CreatedAt)
		r.Enabled = en == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

func (db *DB) DeleteRule(id string) error {
	_, err := db.conn.Exec("DELETE FROM cost_rules WHERE id=?", id)
	return err
}

// MatchCost returns the cost in cents for a given method+path against upstream rules.
func (db *DB) MatchCost(upstreamID, method, path string) (costCents int, ruleID string) {
	rules, err := db.ListRules(upstreamID)
	if err != nil {
		return 0, ""
	}
	for _, r := range rules {
		if (r.Method == "*" || r.Method == method) && matchPath(r.PathPattern, path) {
			return r.CostCents, r.ID
		}
	}
	return 0, ""
}

// matchPath supports exact match and prefix wildcard (e.g. "/messages/*")
func matchPath(pattern, path string) bool {
	if pattern == "*" || pattern == "" {
		return true
	}
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		return len(path) >= len(pattern)-1 && path[:len(pattern)-1] == pattern[:len(pattern)-1]
	}
	return pattern == path
}

// ── Request Logging ───────────────────────────────────────────────────

type Request struct {
	ID           string `json:"id"`
	UpstreamID   string `json:"upstream_id"`
	UpstreamName string `json:"upstream_name"`
	Method       string `json:"method"`
	Path         string `json:"path"`
	Status       int    `json:"status"`
	LatencyMs    int    `json:"latency_ms"`
	ReqBytes     int    `json:"req_bytes"`
	RespBytes    int    `json:"resp_bytes"`
	CostCents    int    `json:"cost_cents"`
	RuleID       string `json:"rule_id,omitempty"`
	SourceIP     string `json:"source_ip"`
	CreatedAt    string `json:"created_at"`
}

func (db *DB) LogRequest(r Request) {
	if r.ID == "" {
		r.ID = "req_" + genID(10)
	}
	db.conn.Exec(
		`INSERT INTO requests (id,upstream_id,upstream_name,method,path,status,latency_ms,req_bytes,resp_bytes,cost_cents,rule_id,source_ip)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.UpstreamID, r.UpstreamName, r.Method, r.Path, r.Status,
		r.LatencyMs, r.ReqBytes, r.RespBytes, r.CostCents, r.RuleID, r.SourceIP)
}

type RequestFilter struct {
	UpstreamID string
	Method     string
	From       string
	To         string
	Limit      int
	Offset     int
}

func (db *DB) ListRequests(f RequestFilter) ([]Request, int, error) {
	if f.Limit <= 0 || f.Limit > 1000 {
		f.Limit = 100
	}
	where := "1=1"
	args := []any{}
	if f.UpstreamID != "" {
		where += " AND upstream_id=?"
		args = append(args, f.UpstreamID)
	}
	if f.Method != "" {
		where += " AND method=?"
		args = append(args, f.Method)
	}
	if f.From != "" {
		where += " AND created_at >= ?"
		args = append(args, f.From)
	}
	if f.To != "" {
		where += " AND created_at <= ?"
		args = append(args, f.To)
	}

	var total int
	db.conn.QueryRow("SELECT COUNT(*) FROM requests WHERE "+where, args...).Scan(&total)

	args = append(args, f.Limit, f.Offset)
	rows, err := db.conn.Query(
		`SELECT id,upstream_id,upstream_name,method,path,status,latency_ms,req_bytes,resp_bytes,cost_cents,rule_id,source_ip,created_at
		 FROM requests WHERE `+where+` ORDER BY created_at DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Request
	for rows.Next() {
		var r Request
		rows.Scan(&r.ID, &r.UpstreamID, &r.UpstreamName, &r.Method, &r.Path, &r.Status,
			&r.LatencyMs, &r.ReqBytes, &r.RespBytes, &r.CostCents, &r.RuleID, &r.SourceIP, &r.CreatedAt)
		out = append(out, r)
	}
	return out, total, rows.Err()
}

// ── Spend Summary ─────────────────────────────────────────────────────

type DaySpend struct {
	Date      string `json:"date"`
	Requests  int    `json:"requests"`
	CostCents int    `json:"cost_cents"`
}

type EndpointSpend struct {
	Path      string `json:"path"`
	Method    string `json:"method"`
	Requests  int    `json:"requests"`
	CostCents int    `json:"cost_cents"`
}

type SpendSummary struct {
	UpstreamID   string          `json:"upstream_id,omitempty"`
	TotalCents   int             `json:"total_cents"`
	TotalCentsMo int             `json:"total_cents_month"`
	TotalReqs    int             `json:"total_requests"`
	ByDay        []DaySpend      `json:"by_day"`
	ByEndpoint   []EndpointSpend `json:"by_endpoint"`
}

func (db *DB) SpendSummary(upstreamID string, days int) SpendSummary {
	if days <= 0 {
		days = 30
	}
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	monthStart := time.Now().AddDate(0, 0, -30).Format("2006-01-02")

	where := "created_at >= ?"
	args := []any{cutoff}
	if upstreamID != "" {
		where += " AND upstream_id=?"
		args = append(args, upstreamID)
	}

	var s SpendSummary
	s.UpstreamID = upstreamID
	db.conn.QueryRow("SELECT COUNT(*), COALESCE(SUM(cost_cents),0) FROM requests WHERE "+where, args...).
		Scan(&s.TotalReqs, &s.TotalCents)

	moArgs := []any{monthStart}
	moWhere := "created_at >= ?"
	if upstreamID != "" {
		moWhere += " AND upstream_id=?"
		moArgs = append(moArgs, upstreamID)
	}
	db.conn.QueryRow("SELECT COALESCE(SUM(cost_cents),0) FROM requests WHERE "+moWhere, moArgs...).
		Scan(&s.TotalCentsMo)

	// By day
	rows, _ := db.conn.Query(
		"SELECT date(created_at), COUNT(*), COALESCE(SUM(cost_cents),0) FROM requests WHERE "+where+" GROUP BY date(created_at) ORDER BY date(created_at) DESC LIMIT 30",
		args...)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var d DaySpend
			rows.Scan(&d.Date, &d.Requests, &d.CostCents)
			s.ByDay = append(s.ByDay, d)
		}
	}

	// By endpoint
	rows2, _ := db.conn.Query(
		"SELECT path, method, COUNT(*), COALESCE(SUM(cost_cents),0) FROM requests WHERE "+where+" GROUP BY path, method ORDER BY SUM(cost_cents) DESC LIMIT 20",
		args...)
	if rows2 != nil {
		defer rows2.Close()
		for rows2.Next() {
			var e EndpointSpend
			rows2.Scan(&e.Path, &e.Method, &e.Requests, &e.CostCents)
			s.ByEndpoint = append(s.ByEndpoint, e)
		}
	}

	return s
}

// ── Alerts ────────────────────────────────────────────────────────────

type Alert struct {
	ID             string `json:"id"`
	UpstreamID     string `json:"upstream_id"`
	Period         string `json:"period"`
	ThresholdCents int    `json:"threshold_cents"`
	WebhookURL     string `json:"webhook_url"`
	Enabled        bool   `json:"enabled"`
	LastFiredAt    string `json:"last_fired_at,omitempty"`
	CreatedAt      string `json:"created_at"`
}

func (db *DB) CreateAlert(upstreamID, period, webhookURL string, thresholdCents int) (*Alert, error) {
	id := "alrt_" + genID(8)
	if period == "" {
		period = "daily"
	}
	_, err := db.conn.Exec(
		"INSERT INTO alerts (id,upstream_id,period,threshold_cents,webhook_url) VALUES (?,?,?,?,?)",
		id, upstreamID, period, thresholdCents, webhookURL)
	if err != nil {
		return nil, err
	}
	return &Alert{ID: id, UpstreamID: upstreamID, Period: period,
		ThresholdCents: thresholdCents, WebhookURL: webhookURL, Enabled: true,
		CreatedAt: time.Now().UTC().Format(time.RFC3339)}, nil
}

func (db *DB) ListAlerts() ([]Alert, error) {
	rows, err := db.conn.Query(
		"SELECT id,upstream_id,period,threshold_cents,webhook_url,enabled,last_fired_at,created_at FROM alerts WHERE enabled=1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Alert
	for rows.Next() {
		var a Alert
		var en int
		rows.Scan(&a.ID, &a.UpstreamID, &a.Period, &a.ThresholdCents, &a.WebhookURL, &en, &a.LastFiredAt, &a.CreatedAt)
		a.Enabled = en == 1
		out = append(out, a)
	}
	return out, rows.Err()
}

func (db *DB) DeleteAlert(id string) error {
	_, err := db.conn.Exec("DELETE FROM alerts WHERE id=?", id)
	return err
}

func (db *DB) MarkAlertFired(id string) {
	db.conn.Exec("UPDATE alerts SET last_fired_at=datetime('now') WHERE id=?", id)
}

func (db *DB) SpendSince(upstreamID, since string) int {
	where := "created_at >= ?"
	args := []any{since}
	if upstreamID != "" {
		where += " AND upstream_id=?"
		args = append(args, upstreamID)
	}
	var total int
	db.conn.QueryRow("SELECT COALESCE(SUM(cost_cents),0) FROM requests WHERE "+where, args...).Scan(&total)
	return total
}

// ── Stats ─────────────────────────────────────────────────────────────

func (db *DB) Stats() map[string]any {
	var totalReqs, todayReqs, totalCents, todayCents int
	db.conn.QueryRow("SELECT COUNT(*), COALESCE(SUM(cost_cents),0) FROM requests").Scan(&totalReqs, &totalCents)
	db.conn.QueryRow("SELECT COUNT(*), COALESCE(SUM(cost_cents),0) FROM requests WHERE created_at >= date('now')").Scan(&todayReqs, &todayCents)
	return map[string]any{
		"total_requests":    totalReqs,
		"requests_today":    todayReqs,
		"total_cost_cents":  totalCents,
		"cost_today_cents":  todayCents,
	}
}

// ── CSV Export ────────────────────────────────────────────────────────

func (db *DB) ExportCSV(upstreamID, from, to string) ([][]string, error) {
	where := "1=1"
	args := []any{}
	if upstreamID != "" {
		where += " AND upstream_id=?"
		args = append(args, upstreamID)
	}
	if from != "" {
		where += " AND created_at >= ?"
		args = append(args, from)
	}
	if to != "" {
		where += " AND created_at <= ?"
		args = append(args, to)
	}

	rows, err := db.conn.Query(
		"SELECT created_at,upstream_name,method,path,status,latency_ms,req_bytes,resp_bytes,cost_cents FROM requests WHERE "+where+" ORDER BY created_at DESC LIMIT 50000",
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := [][]string{{"timestamp", "upstream", "method", "path", "status", "latency_ms", "req_bytes", "resp_bytes", "cost_cents"}}
	for rows.Next() {
		var ts, upstream, method, path string
		var status, latency, reqB, respB, cost int
		rows.Scan(&ts, &upstream, &method, &path, &status, &latency, &reqB, &respB, &cost)
		out = append(out, []string{ts, upstream, method, path,
			fmt.Sprintf("%d", status), fmt.Sprintf("%d", latency),
			fmt.Sprintf("%d", reqB), fmt.Sprintf("%d", respB), fmt.Sprintf("%d", cost)})
	}
	return out, rows.Err()
}

// ── Helpers ───────────────────────────────────────────────────────────

func genID(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// MonthlyRequestCount returns the number of proxied requests this calendar month for an upstream.
// If upstreamID is empty, counts across all upstreams.
func (db *DB) MonthlyRequestCount(upstreamID string) int {
	var n int
	if upstreamID != "" {
		db.conn.QueryRow(
			"SELECT COUNT(*) FROM requests WHERE upstream_id=? AND created_at >= date('now','start of month')",
			upstreamID).Scan(&n)
	} else {
		db.conn.QueryRow(
			"SELECT COUNT(*) FROM requests WHERE created_at >= date('now','start of month')").Scan(&n)
	}
	return n
}

// Anomaly represents a detected spend or volume spike.
type Anomaly struct {
	UpstreamID   string  `json:"upstream_id"`
	UpstreamName string  `json:"upstream_name"`
	Path         string  `json:"path"`
	Type         string  `json:"type"` // "spend" or "volume"
	TodayCents   int     `json:"today_cents,omitempty"`
	AvgCents     float64 `json:"avg_daily_cents,omitempty"`
	TodayReqs    int     `json:"today_requests,omitempty"`
	AvgReqs      float64 `json:"avg_daily_requests,omitempty"`
	Ratio        float64 `json:"ratio"` // today / avg
}

// DetectAnomalies finds routes where today's spend or volume is >2x the 7-day daily average.
func (db *DB) DetectAnomalies() []Anomaly {
	query := `
SELECT upstream_id, upstream_name, path,
       SUM(CASE WHEN date(created_at)=date('now') THEN cost_cents ELSE 0 END) as today_cost,
       AVG(CASE WHEN date(created_at)>=date('now','-7 days') AND date(created_at)<date('now') THEN daily_cost ELSE NULL END) as avg_cost,
       SUM(CASE WHEN date(created_at)=date('now') THEN 1 ELSE 0 END) as today_reqs,
       AVG(CASE WHEN date(created_at)>=date('now','-7 days') AND date(created_at)<date('now') THEN daily_reqs ELSE NULL END) as avg_reqs
FROM (
  SELECT upstream_id, upstream_name, path, date(created_at) as d,
         SUM(cost_cents) as daily_cost, COUNT(*) as daily_reqs, created_at
  FROM requests
  WHERE created_at >= date('now','-8 days')
  GROUP BY upstream_id, path, date(created_at)
)
GROUP BY upstream_id, path
HAVING today_cost > avg_cost * 2 OR today_reqs > avg_reqs * 2`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []Anomaly
	for rows.Next() {
		var a Anomaly
		var todayCost, todayReqs int
		var avgCost, avgReqs float64
		if rows.Scan(&a.UpstreamID, &a.UpstreamName, &a.Path, &todayCost, &avgCost, &todayReqs, &avgReqs) != nil {
			continue
		}
		if avgCost > 0 && float64(todayCost) > avgCost*2 {
			a.Type = "spend"
			a.TodayCents = todayCost
			a.AvgCents = avgCost
			a.Ratio = float64(todayCost) / avgCost
		} else {
			a.Type = "volume"
			a.TodayReqs = todayReqs
			a.AvgReqs = avgReqs
			a.Ratio = float64(todayReqs) / avgReqs
		}
		out = append(out, a)
	}
	return out
}
