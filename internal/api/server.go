package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/iFurySt/agenttrace/internal/otel"
	"github.com/iFurySt/agenttrace/internal/storage"
	"github.com/iFurySt/agenttrace/internal/tracer"
)

type Server struct {
	store    *storage.Store
	mux      *http.ServeMux
	exporter *otel.Exporter
}

func NewServer(store *storage.Store) *Server {
	s := &Server{store: store, mux: http.NewServeMux()}
	s.registerRoutes()
	return s
}

// SetExporter attaches an OTLP exporter �?every stored span will also be forwarded.
func (s *Server) SetExporter(exp *otel.Exporter) {
	s.exporter = exp
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	// Limit request body size for mutating requests
	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
		r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10MB
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/api/traces", s.handleTraces)
	s.mux.HandleFunc("/api/traces/", s.handleTraceByID)
	s.mux.HandleFunc("/api/spans", s.handleSpans)
	s.mux.HandleFunc("/api/spans/", s.handleSpanByID)
	s.mux.HandleFunc("/api/stats", s.handleStats)
	s.mux.HandleFunc("/api/search", s.handleSearch)
	s.mux.HandleFunc("/api/health", s.handleHealth)
	// OTLP ingest endpoint
	s.mux.Handle("/v1/traces", &otel.IngestHandler{Store: s.store})
}

func (s *Server) handleTraces(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 { limit = 50 }
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		traces, err := s.store.ListTraces(limit, offset)
		if err != nil { writeError(w, http.StatusInternalServerError, err); return }
		if traces == nil { traces = []tracer.TraceWithSummary{} }
		writeJSON(w, http.StatusOK, traces)

	case http.MethodPost:
		var input struct {
			ID string `json:"id,omitempty"`
			Name string `json:"name"`
			UserID string `json:"user_id,omitempty"`
			SessionID string `json:"session_id,omitempty"`
			Tags []string `json:"tags,omitempty"`
			Metadata tracer.JSONMap `json:"metadata,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil { writeError(w, http.StatusBadRequest, err); return }
		tr := tracer.NewTrace(input.Name)
		if input.ID != "" { tr.ID = input.ID }
		if input.UserID != "" { tr.UserID = input.UserID }
		if input.SessionID != "" { tr.SessionID = input.SessionID }
		if input.Tags != nil { tr.Tags = input.Tags }
		if input.Metadata != nil { tr.Metadata = input.Metadata }
		if err := s.store.SaveTrace(tr); err != nil { writeError(w, http.StatusInternalServerError, err); return }
		writeJSON(w, http.StatusCreated, tr)

	default: writeError(w, http.StatusMethodNotAllowed, nil)
	}
}

func (s *Server) handleTraceByID(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/traces/"):]
	if id == "" { writeError(w, http.StatusBadRequest, nil); return }

	switch r.Method {
	case http.MethodGet:
		tr, err := s.store.GetTrace(id)
		if err != nil { writeError(w, http.StatusNotFound, err); return }
		spans, _ := s.store.GetSpansByTrace(tr.ID)
		if spans == nil { spans = []tracer.Span{} }
		writeJSON(w, http.StatusOK, map[string]any{"trace": tr, "spans": spans})

	case http.MethodDelete:
		if err := s.store.DeleteTrace(id); err != nil { writeError(w, http.StatusInternalServerError, err); return }
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default: writeError(w, http.StatusMethodNotAllowed, nil)
	}
}

func (s *Server) handleSpans(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost { writeError(w, http.StatusMethodNotAllowed, nil); return }

	var raw map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil { writeError(w, http.StatusBadRequest, err); return }

	sp := tracer.Span{}
	sp.ID = strField(raw, "id")
	if sp.ID == "" { sp.ID = tracer.NewID() }
	sp.TraceID = strField(raw, "trace_id")
	if sp.TraceID == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("trace_id is required"))
		return
	}
	sp.ParentSpanID = strField(raw, "parent_span_id")
	sp.Name = strField(raw, "name")
	if sp.Name == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("name is required"))
		return
	}
	sp.Kind = tracer.SpanKind(strField(raw, "kind"))
	sp.Status = tracer.SpanStatus(strField(raw, "status"))
	if sp.Status == "" { sp.Status = tracer.StatusOK }
	sp.Model = strField(raw, "model")
	sp.Provider = strField(raw, "provider")
	sp.InputJSON = strField(raw, "input_json")
	sp.OutputJSON = strField(raw, "output_json")
	sp.PromptTokens = intField(raw, "prompt_tokens")
	sp.CompletionTokens = intField(raw, "completion_tokens")
	sp.TotalTokens = intField(raw, "total_tokens")
	sp.Cost = floatField(raw, "cost")
	sp.ErrorMessage = strField(raw, "error_message")
	sp.StartedAt = parseTimeField(raw, "started_at")
	sp.EndedAt = parseTimePtrField(raw, "ended_at")
	sp.CreatedAt = time.Now().UTC()
	if sp.StartedAt.IsZero() { sp.StartedAt = time.Now().UTC() }

	if _, err := s.store.GetTrace(sp.TraceID); err != nil {
		name := "auto"
		if sp.Name != "" {
			name = "auto: " + sp.Name
		}
		tr := tracer.NewTrace(name)
		tr.ID = sp.TraceID
		s.store.SaveTrace(tr)
	}

if err := s.store.SaveSpan(sp); err != nil { writeError(w, http.StatusInternalServerError, err); return }

	if tr, err := s.store.GetTrace(sp.TraceID); err == nil { tr.UpdatedAt = time.Now().UTC(); s.store.SaveTrace(tr) }
	s.store.RefreshStats(sp.TraceID)

	// Forward to OTLP exporter if configured (async, single span only)
	if s.exporter != nil {
		if tr, err := s.store.GetTrace(sp.TraceID); err == nil {
			spanCopy := sp
			go func() {
				if err := s.exporter.Export(tr, []tracer.Span{spanCopy}); err != nil {
					log.Printf("OTLP export error: %v", err)
				}
			}()
		}
	}

	writeJSON(w, http.StatusCreated, sp)
}

func (s *Server) handleSpanByID(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/spans/"):]
	if id == "" { writeError(w, http.StatusBadRequest, nil); return }
	if r.Method != http.MethodGet { writeError(w, http.StatusMethodNotAllowed, nil); return }
	sp, err := s.store.GetSpan(id)
	if err != nil { writeError(w, http.StatusNotFound, err); return }
	writeJSON(w, http.StatusOK, sp)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.Stats()
	if err != nil { writeError(w, http.StatusInternalServerError, err); return }
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" { writeJSON(w, http.StatusOK, []tracer.Trace{}); return }
	traces, err := s.store.SearchTraces(q, 20)
	if err != nil { writeError(w, http.StatusInternalServerError, err); return }
	if traces == nil { traces = []tracer.Trace{} }
	writeJSON(w, http.StatusOK, traces)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "time": time.Now().UTC().Format(time.RFC3339)})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	msg := ""
	if err != nil { msg = err.Error() }
	writeJSON(w, status, map[string]string{"error": msg})
}

func strField(m map[string]interface{}, key string) string { v, _ := m[key].(string); return v }

func intField(m map[string]interface{}, key string) int {
	if v, ok := m[key].(float64); ok { return int(v) }
	return 0
}

func floatField(m map[string]interface{}, key string) float64 {
	if v, ok := m[key].(float64); ok { return v }
	return 0
}

func parseTimeField(m map[string]interface{}, key string) time.Time {
	s := strField(m, key)
	if s == "" { return time.Time{} }
	t, _ := time.Parse(time.RFC3339Nano, s)
	if t.IsZero() { t, _ = time.Parse(time.RFC3339, s) }
	return t
}

func parseTimePtrField(m map[string]interface{}, key string) *time.Time {
	t := parseTimeField(m, key)
	if t.IsZero() { return nil }
	return &t
}