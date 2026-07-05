// Package otel provides OTLP/HTTP export for AgentTrace data.
//
// Supports two directions:
//  1. Export: Convert AgentTrace spans to OTel spans, encode as OTLP/JSON, POST to collector.
//  2. Ingest: Accept OTLP/JSON payloads from external OTel SDKs, store in AgentTrace.
//
// Uses OTLP over HTTP with JSON encoding (no protobuf dependency).
// Endpoint: POST /v1/traces  (standard OTLP trace endpoint)
package otel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/iFurySt/agenttrace/internal/storage"
	"github.com/iFurySt/agenttrace/internal/tracer"
)

// Exporter sends AgentTrace spans to an OTLP-compatible collector.
type Exporter struct {
	endpoint   string
	httpClient *http.Client
}

// NewExporter creates an OTLP/HTTP exporter.
// endpoint should be the full OTLP endpoint, e.g. "http://localhost:4318" (no trailing /v1/traces).
func NewExporter(endpoint string) *Exporter {
	return &Exporter{
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Export converts a trace and its spans to OTLP/JSON and sends them to the collector.
func (e *Exporter) Export(tr tracer.Trace, spans []tracer.Span) error {
	rs := make([]resourceSpan, 1)
	rs[0] = resourceSpan{
		Resource: resource{
			Attributes: []attribute{
				{Key: "service.name", Value: stringValue("agenttrace")},
				{Key: "agenttrace.trace_name", Value: stringValue(tr.Name)},
			},
		},
		ScopeSpans: []scopeSpan{{
			Scope: instrumentationScope{
				Name:    "agenttrace",
				Version: "0.2.0",
			},
			Spans: make([]otlpSpan, 0, len(spans)),
		}},
	}

	for _, sp := range spans {
		rs[0].ScopeSpans[0].Spans = append(rs[0].ScopeSpans[0].Spans, toOTLPSpan(sp))
	}

	payload := otlpPayload{ResourceSpans: rs}
	return e.send(payload)
}

// ExportTraceOnly sends a single trace (no spans) �?mainly for trace-level metadata.
func (e *Exporter) ExportTraceOnly(tr tracer.Trace) error {
	return e.Export(tr, nil)
}

// ExportSpans sends spans for an existing trace.
func (e *Exporter) ExportSpans(tr tracer.Trace, spans []tracer.Span) error {
	return e.Export(tr, spans)
}

func (e *Exporter) send(payload otlpPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("otlp: marshal: %w", err)
	}

	url := e.endpoint + "/v1/traces"
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("otlp: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("otlp: send: %w", err)
	}
	defer resp.Body.Close()
	defer io.Copy(io.Discard, resp.Body) // drain for connection reuse

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("otlp: http %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// ── OTLP/JSON data model ──

type otlpPayload struct {
	ResourceSpans []resourceSpan `json:"resourceSpans"`
}

type resourceSpan struct {
	Resource   resource    `json:"resource"`
	ScopeSpans []scopeSpan `json:"scopeSpans"`
}

type resource struct {
	Attributes []attribute `json:"attributes,omitempty"`
}

type scopeSpan struct {
	Scope instrumentationScope `json:"scope"`
	Spans []otlpSpan           `json:"spans"`
}

type instrumentationScope struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type otlpSpan struct {
	TraceID           string        `json:"traceId"`
	SpanID            string        `json:"spanId"`
	ParentSpanID      string        `json:"parentSpanId,omitempty"`
	Name              string        `json:"name"`
	Kind              int           `json:"kind"`
	StartTimeUnixNano string        `json:"startTimeUnixNano"`
	EndTimeUnixNano   string        `json:"endTimeUnixNano"`
	Attributes        []attribute   `json:"attributes,omitempty"`
	Status            otlpStatus    `json:"status,omitempty"`
	Events            []otlpEvent   `json:"events,omitempty"`
}

type otlpStatus struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
}

type otlpEvent struct {
	Name       string      `json:"name"`
	TimeUnixNano string    `json:"timeUnixNano"`
	Attributes []attribute `json:"attributes,omitempty"`
}

type attribute struct {
	Key   string     `json:"key"`
	Value valueUnion `json:"value"`
}

type valueUnion struct {
	StringValue string  `json:"stringValue,omitempty"`
	IntValue    string  `json:"intValue,omitempty"`
	DoubleValue float64 `json:"doubleValue,omitempty"`
	BoolValue   bool    `json:"boolValue,omitempty"`
}

func stringValue(s string) valueUnion  { return valueUnion{StringValue: s} }
func intValue(i int) valueUnion        { return valueUnion{IntValue: fmt.Sprintf("%d", i)} }
func doubleValue(f float64) valueUnion { return valueUnion{DoubleValue: f} }
func boolValue(b bool) valueUnion      { return valueUnion{BoolValue: b} }

// ── OTel span kind mapping ──
// AgentTrace kinds �?OTel SpanKind (1=INTERNAL, 2=SERVER, 3=CLIENT, 4=PRODUCER, 5=CONSUMER)
func otelSpanKind(k tracer.SpanKind) int {
	switch k {
	case tracer.KindLLM:
		return 3 // CLIENT �?calling an external LLM service
	case tracer.KindTool:
		return 1 // INTERNAL �?tool execution within agent
	case tracer.KindRetrieval:
		return 3 // CLIENT �?calling vector DB
	case tracer.KindChain:
		return 1 // INTERNAL �?orchestration
	case tracer.KindAgent:
		return 1 // INTERNAL �?agent loop
	default:
		return 1
	}
}

// ── Gen AI semantic convention mapping ──

func toOTLPSpan(sp tracer.Span) otlpSpan {
	// OTel requires 16-char hex for TraceID (already 32 hex = 16 bytes)
	// OTel requires 8-char hex for SpanID (already 16 hex = 8 bytes)
	os := otlpSpan{
		TraceID:           sp.TraceID,
		SpanID:            sp.ID,
		ParentSpanID:      sp.ParentSpanID,
		Name:              sp.Name,
		Kind:              otelSpanKind(sp.Kind),
		StartTimeUnixNano: fmt.Sprintf("%d", sp.StartedAt.UnixNano()),
		Status:            otlpStatus{Code: statusCode(sp.Status), Message: sp.ErrorMessage},
		Attributes:        []attribute{},
	}

	// End time
	if sp.EndedAt != nil {
		os.EndTimeUnixNano = fmt.Sprintf("%d", sp.EndedAt.UnixNano())
	} else {
		os.EndTimeUnixNano = fmt.Sprintf("%d", sp.StartedAt.UnixNano())
	}

	// Required: gen_ai.operation.name �?maps AgentTrace SpanKind �?OTel gen_ai operation
	os.Attributes = append(os.Attributes,
		kv("gen_ai.operation.name", stringValue(genAIOperation(sp.Kind))),
	)

	// Required: gen_ai.provider.name (only if we have a provider field)
	if sp.Provider != "" {
		os.Attributes = append(os.Attributes, kv("gen_ai.provider.name", stringValue(sp.Provider)))
	}

	// Conditionally required: gen_ai.request.model
	if sp.Model != "" {
		os.Attributes = append(os.Attributes, kv("gen_ai.request.model", stringValue(sp.Model)))
	}

	// Usage: gen_ai.usage.input_tokens / output_tokens
	if sp.TotalTokens > 0 {
		os.Attributes = append(os.Attributes,
			kv("gen_ai.usage.input_tokens", intValue(sp.PromptTokens)),
			kv("gen_ai.usage.output_tokens", intValue(sp.CompletionTokens)),
		)
	}

	// Cost: we store this as custom attributes since cost isn't in the OTel spec yet
	if sp.Cost > 0 {
		os.Attributes = append(os.Attributes,
			kv("gen_ai.cost.total", doubleValue(sp.Cost)),
		)
	}

	// Input/output content (opt-in, PII-sensitive �?we emit them for full observability)
	if sp.InputJSON != "" && sp.InputJSON != "{}" {
		os.Attributes = append(os.Attributes, kv("gen_ai.input.messages", stringValue(sp.InputJSON)))
	}
	if sp.OutputJSON != "" && sp.OutputJSON != "{}" {
		os.Attributes = append(os.Attributes, kv("gen_ai.output.messages", stringValue(sp.OutputJSON)))
	}

	return os
}

func statusCode(s tracer.SpanStatus) int {
	if s == tracer.StatusError {
		return 2 // OTel STATUS_CODE_ERROR
	}
	return 1 // OTel STATUS_CODE_OK
}

// genAIOperation maps AgentTrace SpanKind �?gen_ai.operation.name values
func genAIOperation(k tracer.SpanKind) string {
	switch k {
	case tracer.KindLLM:
		return "chat" // gen_ai.operation.name = "chat"
	case tracer.KindTool:
		return "execute_tool"
	case tracer.KindRetrieval:
		return "retrieval"
	case tracer.KindChain:
		return "chain"
	case tracer.KindAgent:
		return "invoke_agent"
	default:
		return "chat"
	}
}

func kv(key string, value valueUnion) attribute {
	return attribute{Key: key, Value: value}
}

// ── OTLP Ingest ──

// IngestHandler is an http.Handler that accepts OTLP/JSON spans and stores them.
type IngestHandler struct {
	Store *storage.Store
}

func (h *IngestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var payload otlpPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, fmt.Sprintf("invalid OTLP/JSON: %v", err), http.StatusBadRequest)
		return
	}

	count := 0
	for _, rs := range payload.ResourceSpans {
		// Extract service name from resource attributes
		svcName := "unknown"
		for _, attr := range rs.Resource.Attributes {
			if attr.Key == "service.name" {
				svcName = attr.Value.StringValue
			}
		}

		for _, ss := range rs.ScopeSpans {
			for _, s := range ss.Spans {
				sp := fromOTLPSpan(s)
				// Ensure trace exists with a readable name
				if _, err := h.Store.GetTrace(sp.TraceID); err != nil {
					traceName := svcName
					if s.Name != "" {
						traceName = svcName + "/" + s.Name
					}
					tr := tracer.NewTrace(traceName)
					tr.ID = sp.TraceID
					h.Store.SaveTrace(tr)
				}
				h.Store.SaveSpan(sp)
				h.Store.RefreshStats(sp.TraceID)
				count++
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"partialSuccess": map[string]any{}})
}

func fromOTLPSpan(s otlpSpan) tracer.Span {
	sa := parseUnixNano(s.StartTimeUnixNano)
	var ea *time.Time
	if s.EndTimeUnixNano != "" {
		t := parseUnixNano(s.EndTimeUnixNano)
		ea = &t
	}

	sp := tracer.Span{
		ID:           s.SpanID,
		TraceID:      s.TraceID,
		ParentSpanID: s.ParentSpanID,
		Name:         s.Name,
		Kind:         tracer.KindLLM,
		Status:       tracer.StatusOK,
		StartedAt:    sa,
		EndedAt:      ea,
		CreatedAt:    time.Now().UTC(),
	}

	if s.Status.Code == 2 {
		sp.Status = tracer.StatusError
		sp.ErrorMessage = s.Status.Message
	}

	for _, attr := range s.Attributes {
		switch attr.Key {
		case "gen_ai.provider.name":
			sp.Provider = attr.Value.StringValue
		case "gen_ai.request.model":
			sp.Model = attr.Value.StringValue
		case "gen_ai.usage.input_tokens":
			sp.PromptTokens = parseAttrInt(attr.Value.IntValue)
		case "gen_ai.usage.output_tokens":
			sp.CompletionTokens = parseAttrInt(attr.Value.IntValue)
		case "gen_ai.input.messages":
			sp.InputJSON = attr.Value.StringValue
		case "gen_ai.output.messages":
			sp.OutputJSON = attr.Value.StringValue
		case "gen_ai.operation.name":
			sp.Kind = genAIToKind(attr.Value.StringValue)
		case "gen_ai.cost.total":
			sp.Cost = attr.Value.DoubleValue
		}
	}

	sp.TotalTokens = sp.PromptTokens + sp.CompletionTokens
	if sp.Kind == "" {
		sp.Kind = tracer.KindLLM
	}
	return sp
}

func genAIToKind(op string) tracer.SpanKind {
	switch op {
	case "chat", "generate_content", "text_completion", "embeddings":
		return tracer.KindLLM
	case "execute_tool":
		return tracer.KindTool
	case "retrieval":
		return tracer.KindRetrieval
	case "chain":
		return tracer.KindChain
	case "invoke_agent", "create_agent":
		return tracer.KindAgent
	default:
		return tracer.KindLLM
	}
}

func parseUnixNano(s string) time.Time {
	var ns int64
	fmt.Sscanf(s, "%d", &ns)
	return time.Unix(0, ns)
}

func parseAttrInt(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}
