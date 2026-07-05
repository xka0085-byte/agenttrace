package main

import (
	"fmt"
	"os"
	"time"

	"github.com/iFurySt/agenttrace/internal/storage"
	"github.com/iFurySt/agenttrace/internal/tracer"
)

func main() {
	dbPath := os.Getenv("TEMP") + "\\agenttrace_demo.db"
	os.Remove(dbPath)
	s, _ := storage.Open(dbPath)
	defer s.Close()

	type spanDef struct{ id, pid, name, kind, model string; pt, ct int; cost float64; status string; durMs int }
	type traceDef struct{ id, name string; spans []spanDef }

	defs := []traceDef{
		{"t1", "Customer Support Agent", []spanDef{
			{"s1a","","LLM: gpt-4o","LLM","gpt-4o",1200,450,0.0075,"ok",1200},
			{"s1b","s1a","Tool: search_knowledge_base","TOOL","",0,0,0,"ok",450},
			{"s1c","s1a","LLM: gpt-4o (reply gen)","LLM","gpt-4o",850,620,0.0083,"ok",2100},
			{"s1d","s1a","Tool: send_email_reply","TOOL","",0,0,0,"ok",300},
		}},
		{"t2", "Data Analysis - Q2 Sales Report", []spanDef{
			{"s2a","","LLM: claude-sonnet","LLM","claude-sonnet-4-6",2100,800,0.0183,"ok",2800},
			{"s2b","s2a","Tool: query_database (ERROR)","TOOL","",0,0,0,"error",5000},
			{"s2c","s2a","LLM: claude-sonnet (retry)","LLM","claude-sonnet-4-6",950,350,0.0081,"ok",3100},
		}},
		{"t3", "RAG Document Q&A", []spanDef{
			{"s3a","","Retrieval: vector_search","RETRIEVAL","text-embedding-3-small",300,0,0.00002,"ok",180},
			{"s3b","s3a","LLM: gpt-4o","LLM","gpt-4o",1600,520,0.0092,"ok",3400},
		}},
		{"t4", "Code Review Agent", []spanDef{
			{"s4a","","LLM: gpt-4o (analyze)","LLM","gpt-4o",3400,1200,0.0205,"ok",4200},
			{"s4b","s4a","Tool: read_file (main.go)","TOOL","",0,0,0,"ok",150},
			{"s4c","s4a","Tool: read_file (store.go)","TOOL","",0,0,0,"ok",120},
			{"s4d","s4a","LLM: gpt-4o (review)","LLM","gpt-4o",2800,950,0.0165,"ok",3800},
		}},
		{"t5", "Multi-Agent: Competitive Analysis", []spanDef{
			{"s5a","","Agent: research_agent","AGENT","gpt-4o",5000,1500,0.0275,"ok",6500},
			{"s5b","s5a","LLM: web search parsing","LLM","gpt-4o",1200,400,0.0070,"ok",2200},
			{"s5c","s5a","Agent: writer_agent","AGENT","claude-sonnet-4-6",3200,1800,0.0196,"ok",5200},
			{"s5d","s5a","Agent: reviewer","AGENT","gpt-4o",2500,900,0.0153,"ok",3100},
		}},
	}

	now := time.Now().UTC()
	for _, td := range defs {
		tr := tracer.NewTrace(td.name)
		tr.ID = td.id
		s.SaveTrace(tr)

		t0 := now.Add(-time.Duration(300+len(td.spans)*60) * time.Second)
		for _, sd := range td.spans {
			sp := tracer.NewSpan(td.id, sd.pid, sd.name, tracer.SpanKind(sd.kind))
			sp.ID = sd.id
			sp.Model = sd.model
			sp.Provider = "openai"
			if len(sd.model) >= 6 && (sd.model[:6] == "claude" || sd.model[:6] == "Claude") {
				sp.Provider = "anthropic"
			}
			sp.InputJSON = `[{"role":"user","content":"Example input"}]`
			sp.OutputJSON = `{"content":"Example output"}`
			sp.PromptTokens = sd.pt
			sp.CompletionTokens = sd.ct
			sp.TotalTokens = sd.pt + sd.ct
			sp.Cost = sd.cost
			sp.Status = tracer.SpanStatus(sd.status)
			if sd.status == "error" {
				sp.ErrorMessage = "connection refused: database unreachable"
			}
			start := t0
			end := t0.Add(time.Duration(sd.durMs) * time.Millisecond)
			sp.StartedAt = start
			sp.EndedAt = &end
			s.SaveSpan(sp)
			t0 = end.Add(50 * time.Millisecond)
		}
		s.RefreshStats(td.id)
	}

	// Print results
	ts, _ := s.ListTraces(10, 0)
	fmt.Printf("=== %d traces seeded ===\n", len(ts))
	for _, t := range ts {
		fmt.Printf("  %-40s %2d spans  %7s tok  %9s  %5.0fms  %d err\n",
			t.Trace.Name, t.Summary.TotalSpans,
			fmt.Sprintf("%d", t.Summary.TotalTokens),
			fmt.Sprintf("$%.4f", t.Summary.TotalCost),
			t.Summary.AvgLatencyMs, t.Summary.ErrorCount)
	}
	stats, _ := s.Stats()
	fmt.Printf("\nGlobal: %v traces | %v spans | $%.4f cost | %v tokens\n",
		stats["total_traces"], stats["total_spans"], stats["total_cost"], stats["total_tokens"])
}
