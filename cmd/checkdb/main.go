package main

import (
	"fmt"
	"os"

	"github.com/iFurySt/agenttrace/internal/storage"
)

func main() {
	dbPath := os.Getenv("TEMP") + "\\debug2.db"
	s, _ := storage.Open(dbPath)
	defer s.Close()

	ts, _ := s.ListTraces(10, 0)
	fmt.Printf("Traces: %d\n", len(ts))
	for _, t := range ts {
		ss, err := s.GetSpansByTrace(t.Trace.ID)
		fmt.Printf("  %s | %s | spans=%d err=%v\n", t.Trace.ID, t.Trace.Name, len(ss), err)
		for _, sp := range ss {
			fmt.Printf("    %s | %s | kind=%s | tokens=%d\n", sp.ID, sp.Name, sp.Kind, sp.TotalTokens)
		}
	}

	sp, err := s.GetSpan("S1")
	fmt.Printf("GetSpan(S1): %+v err=%v\n", sp, err)
}
