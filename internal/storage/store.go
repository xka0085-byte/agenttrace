// Package storage provides the SQLite-backed persistence layer for traces and spans.
// Uses modernc.org/sqlite �?a pure Go SQLite implementation, zero CGO.
// All timestamps are stored as RFC 3339 Nano strings because the pure-Go driver
// does not auto-convert TEXT to time.Time.
package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/iFurySt/agenttrace/internal/tracer"
)

// Store is the primary data access layer for trace data.
type Store struct {
	db *sql.DB
}

// Open creates or opens a SQLite database at the given path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("storage: open: %w", err)
	}
	// Force single connection for :memory: to prevent pool isolation
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA cache_size=-8000",
		"PRAGMA busy_timeout=5000",
	} {
		db.Exec(pragma)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS traces (
		id TEXT PRIMARY KEY, name TEXT NOT NULL, user_id TEXT DEFAULT '',
		session_id TEXT DEFAULT '', tags TEXT DEFAULT '[]', metadata TEXT DEFAULT '{}',
		created_at TEXT NOT NULL, updated_at TEXT NOT NULL
	); CREATE TABLE IF NOT EXISTS spans (
		id TEXT PRIMARY KEY, trace_id TEXT NOT NULL, parent_span_id TEXT DEFAULT '',
		name TEXT NOT NULL, kind TEXT NOT NULL DEFAULT 'LLM', status TEXT NOT NULL DEFAULT 'ok',
		started_at TEXT NOT NULL, ended_at TEXT DEFAULT '',
		model TEXT DEFAULT '', provider TEXT DEFAULT '',
		input_json TEXT DEFAULT '', output_json TEXT DEFAULT '',
		prompt_tokens INTEGER DEFAULT 0, completion_tokens INTEGER DEFAULT 0,
		total_tokens INTEGER DEFAULT 0, cost REAL DEFAULT 0,
		error_message TEXT DEFAULT '', metadata TEXT DEFAULT '{}', created_at TEXT NOT NULL,
		FOREIGN KEY (trace_id) REFERENCES traces(id) ON DELETE CASCADE
	); CREATE INDEX IF NOT EXISTS idx_spans_trace ON spans(trace_id);
	CREATE INDEX IF NOT EXISTS idx_spans_parent ON spans(parent_span_id);
	CREATE INDEX IF NOT EXISTS idx_traces_created ON traces(created_at DESC);
	CREATE TABLE IF NOT EXISTS trace_stats (
		trace_id TEXT PRIMARY KEY, total_spans INTEGER DEFAULT 0, total_cost REAL DEFAULT 0,
		total_tokens INTEGER DEFAULT 0, error_count INTEGER DEFAULT 0, avg_latency_ms REAL DEFAULT 0,
		FOREIGN KEY (trace_id) REFERENCES traces(id) ON DELETE CASCADE
	);`); err != nil {
		db.Close()
		return nil, fmt.Errorf("storage: migrate: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	return s.db.Close()
}

// ---- Trace CRUD ----

func (s *Store) SaveTrace(t tracer.Trace) error {
	tags, _ := json.Marshal(t.Tags)
	meta, _ := json.Marshal(t.Metadata)
	// Use UPSERT instead of INSERT OR REPLACE: REPLACE does DELETE+INSERT,
	// which triggers ON DELETE CASCADE and would delete all associated spans.
	_, err := s.db.Exec(`INSERT INTO traces VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, user_id=excluded.user_id,
		session_id=excluded.session_id, tags=excluded.tags, metadata=excluded.metadata,
		updated_at=excluded.updated_at`,
		t.ID, t.Name, t.UserID, t.SessionID, string(tags), string(meta),
		t.CreatedAt.Format(time.RFC3339Nano), t.UpdatedAt.Format(time.RFC3339Nano))
	return err
}

func (s *Store) GetTrace(id string) (tracer.Trace, error) {
	var tr tracer.Trace
	var tagsJSON, metaJSON, ca, ua string
	err := s.db.QueryRow(`SELECT id,name,user_id,session_id,tags,metadata,created_at,updated_at FROM traces WHERE id=?`, id).
		Scan(&tr.ID, &tr.Name, &tr.UserID, &tr.SessionID, &tagsJSON, &metaJSON, &ca, &ua)
	if err != nil {
		return tr, err
	}
	tr.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
	tr.UpdatedAt, _ = time.Parse(time.RFC3339Nano, ua)
	json.Unmarshal([]byte(tagsJSON), &tr.Tags)
	json.Unmarshal([]byte(metaJSON), &tr.Metadata)
	return tr, nil
}

func (s *Store) ListTraces(limit, offset int) ([]tracer.TraceWithSummary, error) {
	rows, err := s.db.Query(`SELECT t.id,t.name,t.user_id,t.session_id,t.tags,t.metadata,t.created_at,t.updated_at,
		COALESCE(s.total_spans,0),COALESCE(s.total_cost,0),COALESCE(s.total_tokens,0),
		COALESCE(s.error_count,0),COALESCE(s.avg_latency_ms,0)
		FROM traces t LEFT JOIN trace_stats s ON s.trace_id=t.id
		ORDER BY t.created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []tracer.TraceWithSummary
	for rows.Next() {
		var tw tracer.TraceWithSummary
		var tagsJSON, metaJSON, ca, ua string
		if err := rows.Scan(&tw.Trace.ID, &tw.Trace.Name, &tw.Trace.UserID, &tw.Trace.SessionID,
			&tagsJSON, &metaJSON, &ca, &ua,
			&tw.Summary.TotalSpans, &tw.Summary.TotalCost, &tw.Summary.TotalTokens,
			&tw.Summary.ErrorCount, &tw.Summary.AvgLatencyMs); err != nil {
			return nil, err
		}
		tw.Trace.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
		tw.Trace.UpdatedAt, _ = time.Parse(time.RFC3339Nano, ua)
		json.Unmarshal([]byte(tagsJSON), &tw.Trace.Tags)
		json.Unmarshal([]byte(metaJSON), &tw.Trace.Metadata)
		out = append(out, tw)
	}
	return out, nil
}
func (s *Store) DeleteTrace(id string) error {
	// Manual cascade: foreign_keys may be OFF on some connections
	s.db.Exec("DELETE FROM spans WHERE trace_id=?", id)
	s.db.Exec("DELETE FROM trace_stats WHERE trace_id=?", id)
	_, err := s.db.Exec("DELETE FROM traces WHERE id=?", id)
	return err
}

// ---- Span CRUD ----

func (s *Store) SaveSpan(sp tracer.Span) error {
	metaJSON, _ := json.Marshal(sp.Metadata)
	endedAt := ""
	if sp.EndedAt != nil {
		endedAt = sp.EndedAt.Format(time.RFC3339Nano)
	}
	_, err := s.db.Exec(`INSERT OR REPLACE INTO spans VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		sp.ID, sp.TraceID, sp.ParentSpanID, sp.Name, string(sp.Kind), string(sp.Status),
		sp.StartedAt.Format(time.RFC3339Nano), endedAt,
		sp.Model, sp.Provider, sp.InputJSON, sp.OutputJSON,
		sp.PromptTokens, sp.CompletionTokens, sp.TotalTokens, sp.Cost,
		sp.ErrorMessage, string(metaJSON), sp.CreatedAt.Format(time.RFC3339Nano))
	return err
}

func (s *Store) GetSpansByTrace(traceID string) ([]tracer.Span, error) {
	rows, err := s.db.Query(`SELECT id,trace_id,parent_span_id,name,kind,status,
		started_at,ended_at,model,provider,input_json,output_json,
		prompt_tokens,completion_tokens,total_tokens,cost,error_message,metadata,created_at
		FROM spans WHERE trace_id=? ORDER BY started_at ASC`, traceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSpans(rows)
}

func (s *Store) GetSpan(id string) (tracer.Span, error) {
	row := s.db.QueryRow(`SELECT id,trace_id,parent_span_id,name,kind,status,
		started_at,ended_at,model,provider,input_json,output_json,
		prompt_tokens,completion_tokens,total_tokens,cost,error_message,metadata,created_at
		FROM spans WHERE id=?`, id)
	return scanSpan(row)
}

func (s *Store) RefreshStats(traceID string) error {
	// Compute average latency in Go to avoid julianday() SQLite edge cases
	rows, err := s.db.Query(`SELECT started_at, ended_at FROM spans WHERE trace_id=? AND ended_at!=''`, traceID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var totalMs float64
	var count int
	for rows.Next() {
		var sa, ea string
		if err := rows.Scan(&sa, &ea); err != nil {
			continue
		}
		st, err1 := time.Parse(time.RFC3339Nano, sa)
		et, err2 := time.Parse(time.RFC3339Nano, ea)
		if err1 == nil && err2 == nil {
			totalMs += et.Sub(st).Seconds() * 1000
			count++
		}
	}
	avgMs := 0.0
	if count > 0 {
		avgMs = totalMs / float64(count)
	}

	_, err = s.db.Exec(`
		INSERT INTO trace_stats (trace_id,total_spans,total_cost,total_tokens,error_count,avg_latency_ms)
		SELECT ?1, COUNT(*), COALESCE(SUM(cost),0), COALESCE(SUM(total_tokens),0),
			SUM(CASE WHEN status='error' THEN 1 ELSE 0 END), ?2
		FROM spans WHERE trace_id=?1 GROUP BY trace_id
		ON CONFLICT(trace_id) DO UPDATE SET total_spans=excluded.total_spans,
			total_cost=excluded.total_cost, total_tokens=excluded.total_tokens,
			error_count=excluded.error_count, avg_latency_ms=excluded.avg_latency_ms`, traceID, avgMs)
	return err
}
func (s *Store) Stats() (map[string]any, error) {
	var totalTraces, totalSpans, totalErrors, totalTokens int
	var totalCost float64
	if err := s.db.QueryRow("SELECT COUNT(*) FROM traces").Scan(&totalTraces); err != nil {
		return nil, err
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM spans").Scan(&totalSpans); err != nil {
		return nil, err
	}
	if err := s.db.QueryRow("SELECT COALESCE(SUM(cost),0) FROM spans").Scan(&totalCost); err != nil {
		return nil, err
	}
	if err := s.db.QueryRow("SELECT COALESCE(SUM(total_tokens),0) FROM spans").Scan(&totalTokens); err != nil {
		return nil, err
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM spans WHERE status='error'").Scan(&totalErrors); err != nil {
		return nil, err
	}
	return map[string]any{
		"total_traces": totalTraces, "total_spans": totalSpans,
		"total_cost": totalCost, "total_tokens": totalTokens, "error_count": totalErrors,
	}, nil
}

func (s *Store) SearchTraces(query string, limit int) ([]tracer.Trace, error) {
	like := "%" + strings.ReplaceAll(query, "%", "\\%") + "%"
	rows, err := s.db.Query(`
		SELECT DISTINCT t.id,t.name,t.user_id,t.session_id,t.tags,t.metadata,t.created_at,t.updated_at
		FROM traces t LEFT JOIN spans s ON s.trace_id=t.id
		WHERE t.name LIKE ?1 ESCAPE '\' OR s.name LIKE ?1 ESCAPE '\' OR s.input_json LIKE ?1 ESCAPE '\' OR s.output_json LIKE ?1 ESCAPE '\'
		ORDER BY t.created_at DESC LIMIT ?2`, like, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []tracer.Trace
	for rows.Next() {
		var tr tracer.Trace
		var tagsJSON, metaJSON, ca, ua string
		if err := rows.Scan(&tr.ID, &tr.Name, &tr.UserID, &tr.SessionID, &tagsJSON, &metaJSON, &ca, &ua); err != nil {
			return nil, err
		}
		tr.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
		tr.UpdatedAt, _ = time.Parse(time.RFC3339Nano, ua)
		json.Unmarshal([]byte(tagsJSON), &tr.Tags)
		json.Unmarshal([]byte(metaJSON), &tr.Metadata)
		out = append(out, tr)
	}
	return out, nil
}

// ---- internal scan helpers ----

func scanSpan(row interface{ Scan(...interface{}) error }) (tracer.Span, error) {
	var sp tracer.Span
	var endedAt, metaJSON, sa, ca string
	err := row.Scan(&sp.ID, &sp.TraceID, &sp.ParentSpanID, &sp.Name, &sp.Kind, &sp.Status,
		&sa, &endedAt, &sp.Model, &sp.Provider, &sp.InputJSON, &sp.OutputJSON,
		&sp.PromptTokens, &sp.CompletionTokens, &sp.TotalTokens, &sp.Cost,
		&sp.ErrorMessage, &metaJSON, &ca)
	if err != nil {
		return sp, err
	}
	sp.StartedAt, _ = time.Parse(time.RFC3339Nano, sa)
	sp.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
	if endedAt != "" {
		t, _ := time.Parse(time.RFC3339Nano, endedAt)
		sp.EndedAt = &t
	}
	json.Unmarshal([]byte(metaJSON), &sp.Metadata)
	return sp, nil
}

func scanSpans(rows *sql.Rows) ([]tracer.Span, error) {
	var spans []tracer.Span
	for rows.Next() {
		sp, err := scanSpan(rows)
		if err != nil {
			return nil, err
		}
		spans = append(spans, sp)
	}
	return spans, nil
}
