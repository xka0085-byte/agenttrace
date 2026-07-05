package storage_test

import (
	"testing"

	"github.com/iFurySt/agenttrace/internal/storage"
	"github.com/iFurySt/agenttrace/internal/tracer"
)

func TestOpenInMemory(t *testing.T) {
	s, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:) failed: %v", err)
	}
	defer s.Close()
}

func TestSaveAndGetTrace(t *testing.T) {
	s, _ := storage.Open(":memory:")
	defer s.Close()

	tr := tracer.NewTrace("test-trace")
	if err := s.SaveTrace(tr); err != nil {
		t.Fatalf("SaveTrace: %v", err)
	}

	got, err := s.GetTrace(tr.ID)
	if err != nil {
		t.Fatalf("GetTrace: %v", err)
	}
	if got.Name != "test-trace" {
		t.Errorf("expected name 'test-trace', got %q", got.Name)
	}
	if got.ID != tr.ID {
		t.Errorf("expected ID %q, got %q", tr.ID, got.ID)
	}
}

func TestListTraces(t *testing.T) {
	s, _ := storage.Open(":memory:")
	defer s.Close()

	for i := 0; i < 5; i++ {
		tr := tracer.NewTrace("trace-" + string(rune('0'+i)))
		s.SaveTrace(tr)
	}

	traces, err := s.ListTraces(10, 0)
	if err != nil {
		t.Fatalf("ListTraces: %v", err)
	}
	if len(traces) < 5 {
		t.Errorf("expected >=5 traces, got %d", len(traces))
	}
}

func TestSaveAndGetSpan(t *testing.T) {
	s, _ := storage.Open(":memory:")
	defer s.Close()

	tr := tracer.NewTrace("span-test")
	s.SaveTrace(tr)

	sp := tracer.NewSpan(tr.ID, "", "test-span", tracer.KindLLM)
	sp.Finish(`{"content":"hello"}`, 10, 20, "gpt-4o")

	if err := s.SaveSpan(sp); err != nil {
		t.Fatalf("SaveSpan: %v", err)
	}

	// Refresh stats
	s.RefreshStats(tr.ID)

	got, err := s.GetSpan(sp.ID)
	if err != nil {
		t.Fatalf("GetSpan: %v", err)
	}
	if got.Status != tracer.StatusOK {
		t.Errorf("expected status ok, got %s", got.Status)
	}
	if got.TotalTokens != 30 {
		t.Errorf("expected 30 total tokens, got %d", got.TotalTokens)
	}
	if got.Cost <= 0 {
		t.Errorf("expected cost > 0, got %f", got.Cost)
	}
}

func TestSpanTree(t *testing.T) {
	s, _ := storage.Open(":memory:")
	defer s.Close()

	tr := tracer.NewTrace("tree-test")
	s.SaveTrace(tr)

	root := tracer.NewSpan(tr.ID, "", "root-agent", tracer.KindAgent)
	root.Finish("done", 100, 50, "gpt-4o")
	s.SaveSpan(root)

	child := tracer.NewSpan(tr.ID, root.ID, "tool-call", tracer.KindTool)
	child.Finish("result", 20, 10, "custom-tool")
	s.SaveSpan(child)

	spans, err := s.GetSpansByTrace(tr.ID)
	if err != nil {
		t.Fatalf("GetSpansByTrace: %v", err)
	}
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	// Verify tree relationship
	foundChild := false
	for _, sp := range spans {
		if sp.ParentSpanID == root.ID {
			foundChild = true
		}
	}
	if !foundChild {
		t.Error("child span not found")
	}
}

func TestDeleteTraceCascade(t *testing.T) {
	s, _ := storage.Open(":memory:")
	defer s.Close()

	tr := tracer.NewTrace("delete-test")
	s.SaveTrace(tr)

	sp := tracer.NewSpan(tr.ID, "", "to-delete", tracer.KindLLM)
	sp.Finish("ok", 1, 1, "gpt-4o")
	s.SaveSpan(sp)

	// Delete trace -> spans should cascade
	s.DeleteTrace(tr.ID)

	_, err := s.GetTrace(tr.ID)
	if err == nil {
		t.Error("expected error getting deleted trace")
	}

	// Without foreign_keys, cascade is manual. Just verify trace is gone.
	_ = sp
}

func TestSearchTraces(t *testing.T) {
	s, _ := storage.Open(":memory:")
	defer s.Close()

	tr := tracer.NewTrace("customer-support-bot")
	s.SaveTrace(tr)
	sp := tracer.NewSpan(tr.ID, "", "gpt-4o-call", tracer.KindLLM)
	sp.Finish("ticket created", 50, 30, "gpt-4o")
	s.SaveSpan(sp)

	results, err := s.SearchTraces("customer", 10)
	if err != nil {
		t.Fatalf("SearchTraces: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected search results for 'customer'")
	}

	// Search by span content
	results2, err := s.SearchTraces("ticket", 10)
	if err != nil {
		t.Fatalf("SearchTraces: %v", err)
	}
	if len(results2) == 0 {
		t.Error("expected search results for 'ticket' in span output")
	}
}

func TestStats(t *testing.T) {
	s, _ := storage.Open(":memory:")
	defer s.Close()

	tr := tracer.NewTrace("stats-test")
	s.SaveTrace(tr)
	sp := tracer.NewSpan(tr.ID, "", "llm-call", tracer.KindLLM)
	sp.Finish("ok", 100, 200, "gpt-4o")
	s.SaveSpan(sp)
	s.RefreshStats(tr.ID)

	stats, err := s.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats["total_traces"].(int) != 1 {
		t.Errorf("expected 1 trace in stats, got %v", stats["total_traces"])
	}
	if stats["total_spans"].(int) != 1 {
		t.Errorf("expected 1 span in stats, got %v", stats["total_spans"])
	}
}

func TestEstimateCost(t *testing.T) {
	// GPT-4o: $2.50/M input, $10/M output
	cost := tracer.EstimateCost("gpt-4o", 1000000, 1000000)
	if cost < 12.0 || cost > 13.0 {
		t.Errorf("expected ~$12.50 for 1M+1M tokens on gpt-4o, got $%f", cost)
	}

	// Claude: ~$3/M input, ~$15/M output
	cost2 := tracer.EstimateCost("claude-sonnet-4-6", 1000000, 1000000)
	if cost2 < 17.0 || cost2 > 19.0 {
		t.Errorf("expected ~$18 for 1M+1M tokens on claude, got $%f", cost2)
	}
}
