// Package tracer provides utilities for creating and manipulating traces and spans.
package tracer

import (
	"strings"
	"crypto/rand"
	"encoding/hex"
	"time"
)

// NewID generates a random 16-byte hex identifier for spans.
func NewID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// NewTraceID generates a random 16-byte hex identifier, compatible
// with OTel TraceID format (16 bytes = 32 hex chars).
func NewTraceID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// NewTrace creates a new Trace with sensible defaults.
func NewTrace(name string) Trace {
	now := time.Now().UTC()
	return Trace{
		ID:        NewTraceID(),
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// NewSpan creates a new Span within a trace. kind defaults to LLM.
func NewSpan(traceID, parentSpanID, name string, kind SpanKind) Span {
	if kind == "" {
		kind = KindLLM
	}
	return Span{
		ID:           NewID(),
		TraceID:      traceID,
		ParentSpanID: parentSpanID,
		Name:         name,
		Kind:         kind,
		Status:       StatusOK,
		StartedAt:    time.Now().UTC(),
		CreatedAt:    time.Now().UTC(),
	}
}

// Finish marks a span as completed, recording its output and token usage.
func (s *Span) Finish(outputJSON string, promptTokens, completionTokens int, model string) {
	now := time.Now().UTC()
	s.EndedAt = &now
	s.OutputJSON = outputJSON
	s.PromptTokens = promptTokens
	s.CompletionTokens = completionTokens
	s.TotalTokens = promptTokens + completionTokens
	s.Model = model
	s.Cost = EstimateCost(model, promptTokens, completionTokens)
}

// Fail marks a span as errored.
func (s *Span) Fail(err error) {
	now := time.Now().UTC()
	s.EndedAt = &now
	s.Status = StatusError
	s.ErrorMessage = err.Error()
}

// DurationMs returns the span's duration in milliseconds.
func (s *Span) DurationMs() float64 {
	if s.EndedAt == nil {
		return 0
	}
	return float64(s.EndedAt.Sub(s.StartedAt).Microseconds()) / 1000.0
}

// EstimateCost calculates LLM cost based on known pricing models.
// Prices are in USD per 1M tokens. Updated as of 2025-07.
func EstimateCost(model string, promptTokens, completionTokens int) float64 {
	// Pricing in USD per 1M tokens, updated 2025-07
	// Default pricing (GPT-4o rates)
	promptPrice := 2.50  // $/1M input tokens
	completionPrice := 10.0 // $/1M output tokens

	switch {
	case contains(model, "gpt-4o-mini"):
		promptPrice, completionPrice = 0.15, 0.60
	case contains(model, "gpt-4o"):
		promptPrice, completionPrice = 2.50, 10.0
	case contains(model, "gpt-4.1"):
		promptPrice, completionPrice = 2.00, 8.00
	case contains(model, "gpt-4"):
		promptPrice, completionPrice = 30.0, 60.0
	case contains(model, "gpt-3.5"):
		promptPrice, completionPrice = 0.50, 1.50
	case contains(model, "claude-sonnet-4"), contains(model, "claude-3.5-sonnet"):
		promptPrice, completionPrice = 3.0, 15.0
	case contains(model, "claude-3-opus"):
		promptPrice, completionPrice = 15.0, 75.0
	case contains(model, "claude"):
		promptPrice, completionPrice = 3.0, 15.0
	case contains(model, "gemini"):
		promptPrice, completionPrice = 0.50, 1.50
	case contains(model, "deepseek-r1"), contains(model, "deepseek-reasoner"):
		promptPrice, completionPrice = 0.55, 2.19
	case contains(model, "deepseek"):
		promptPrice, completionPrice = 0.27, 1.10
	}

	inputCost := float64(promptTokens) / 1_000_000 * promptPrice
	outputCost := float64(completionTokens) / 1_000_000 * completionPrice
	return inputCost + outputCost
}

func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
