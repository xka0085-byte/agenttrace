package main

import (
	"fmt"
	"os"
	"time"

	"github.com/iFurySt/agenttrace/internal/storage"
	"github.com/iFurySt/agenttrace/internal/tracer"
)

func main() {
	dbPath := os.Getenv("TEMP") + "\\file_test.db"
	os.Remove(dbPath)
	s, _ := storage.Open(dbPath)
	defer s.Close()

	tr := tracer.NewTrace("file-test")
	tr.ID = "ft"
	s.SaveTrace(tr)

	sp := tracer.NewSpan("ft", "", "file-span", tracer.KindLLM)
	sp.ID = "fs"
	sp.StartedAt = time.Now()
	sp.CreatedAt = time.Now()
	sp.InputJSON = "{}"
	sp.OutputJSON = "{}"
	end := time.Now()
	sp.EndedAt = &end
	sp.Model = "gpt-4o"
	sp.PromptTokens = 1
	sp.CompletionTokens = 1
	sp.TotalTokens = 2
	sp.Cost = 0.001

	err := s.SaveSpan(sp)
	fmt.Printf("SaveSpan err: %v\n", err)

	// Read back from SAME store
	spans, err := s.GetSpansByTrace("ft")
	fmt.Printf("GetSpansByTrace: %d spans, err=%v\n", len(spans), err)
	for _, sp := range spans {
		fmt.Printf("  id=%s name=%s kind=%s\n", sp.ID, sp.Name, sp.Kind)
	}

	// Re-open and read
	s2, _ := storage.Open(dbPath)
	defer s2.Close()
	spans2, _ := s2.GetSpansByTrace("ft")
	fmt.Printf("After reopen: %d spans\n", len(spans2))
}
