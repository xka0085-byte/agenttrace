// Package tracer defines the core data model for AgentTrace.
// Follows OpenTelemetry semantic conventions for Gen AI spans where applicable.
package tracer

import "time"

// SpanKind classifies the type of operation this span represents.
type SpanKind string

const (
	KindLLM       SpanKind = "LLM"       // direct LLM inference call
	KindTool      SpanKind = "TOOL"      // agent tool/function invocation
	KindRetrieval SpanKind = "RETRIEVAL" // vector search / RAG retrieval
	KindChain     SpanKind = "CHAIN"     // orchestration wrapper (LangChain chain)
	KindAgent     SpanKind = "AGENT"     // agent decision loop iteration
)

// SpanStatus indicates whether the span completed successfully.
type SpanStatus string

const (
	StatusOK    SpanStatus = "ok"
	StatusError SpanStatus = "error"
)

// Trace represents a complete Agent execution session.
type Trace struct {
	ID        string    `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	UserID    string    `json:"user_id,omitempty" db:"user_id"`
	SessionID string    `json:"session_id,omitempty" db:"session_id"`
	Tags      []string  `json:"tags,omitempty"`
	Metadata  JSONMap   `json:"metadata,omitempty"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// Span represents a single operation within a trace.
// A span can have child spans via ParentSpanID, forming a tree.
type Span struct {
	ID           string     `json:"id" db:"id"`
	TraceID      string     `json:"trace_id" db:"trace_id"`
	ParentSpanID string     `json:"parent_span_id,omitempty" db:"parent_span_id"`
	Name         string     `json:"name" db:"name"`
	Kind         SpanKind   `json:"kind" db:"kind"`
	Status       SpanStatus `json:"status" db:"status"`

	// Timing
	StartedAt time.Time  `json:"started_at" db:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty" db:"ended_at"`

	// LLM-specific fields
	Model      string  `json:"model,omitempty" db:"model"`
	Provider   string  `json:"provider,omitempty" db:"provider"`
	InputJSON  string  `json:"input_json,omitempty" db:"input_json"`
	OutputJSON string  `json:"output_json,omitempty" db:"output_json"`

	// Token usage (OpenAI/Anthropic format)
	PromptTokens     int `json:"prompt_tokens" db:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens" db:"completion_tokens"`
	TotalTokens      int `json:"total_tokens" db:"total_tokens"`

	// Cost estimation (USD)
	Cost float64 `json:"cost" db:"cost"`

	// Error
	ErrorMessage string  `json:"error_message,omitempty" db:"error_message"`

	// Flexible metadata
	Metadata JSONMap `json:"metadata,omitempty"`

	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// JSONMap is a flexible key-value store for metadata fields.
type JSONMap map[string]any

// SpanSummary is a lightweight aggregation used for list views and dashboards.
type SpanSummary struct {
	TotalSpans        int     `json:"total_spans"`
	TotalCost         float64 `json:"total_cost"`
	TotalTokens       int     `json:"total_tokens"`
	AvgLatencyMs      float64 `json:"avg_latency_ms"`
	ErrorCount        int     `json:"error_count"`
	FirstSpanStarted  time.Time `json:"first_span_started"`
	LastSpanEnded     time.Time `json:"last_span_ended"`
}

// TraceWithSummary combines a trace with its aggregated span data.
type TraceWithSummary struct {
	Trace   Trace       `json:"trace"`
	Summary SpanSummary `json:"summary"`
}
