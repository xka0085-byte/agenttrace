package main

import (
	"fmt"
	"os"

	"github.com/iFurySt/agenttrace/internal/storage"
)

func main() {
	dbPath := os.Getenv("TEMP") + "\\agt4.db"
	s, _ := storage.Open(dbPath)
	defer s.Close()

	ts, _ := s.ListTraces(10, 0)
	fmt.Printf("Traces: %d\n", len(ts))
	for _, t := range ts {
		spans, _ := s.GetSpansByTrace(t.Trace.ID)
		fmt.Printf("  %s | %s | %d spans\n", t.Trace.ID, t.Trace.Name, len(spans))
		for _, sp := range spans {
			fmt.Printf("    %s | %s | kind=%s | status=%s\n", sp.ID, sp.Name, sp.Kind, sp.Status)
		}
	}

	stats, _ := s.Stats()
	fmt.Printf("Stats: total_spans=%v\n", stats["total_spans"])
}
