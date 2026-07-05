package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"

	"github.com/iFurySt/agenttrace/internal/api"
	"github.com/iFurySt/agenttrace/internal/storage"
)

func main() {
	dbPath := os.Getenv("TEMP") + "\\agt_api_test.db"
	os.Remove(dbPath)
	s, _ := storage.Open(dbPath)
	defer s.Close()

	server := api.NewServer(s)

	// 1. Create trace via HTTP handler
	traceJSON := `{"id":"t1","name":"Test Trace"}`
	req := httptest.NewRequest("POST", "/api/traces", bytes.NewReader([]byte(traceJSON)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	fmt.Printf("POST /api/traces -> %d\n", w.Code)

	// 2. Create span via HTTP handler
	spanJSON := `{"id":"s1","trace_id":"t1","name":"test-span","kind":"LLM","status":"ok","model":"gpt-4o","provider":"openai","input_json":"{}","output_json":"{}","prompt_tokens":1,"completion_tokens":1,"total_tokens":2,"cost":0.001,"started_at":"2026-07-05T12:00:00Z"}`
	req2 := httptest.NewRequest("POST", "/api/spans", bytes.NewReader([]byte(spanJSON)))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	server.ServeHTTP(w2, req2)
	fmt.Printf("POST /api/spans -> %d: %s\n", w2.Code, w2.Body.String())

	// 3. Get trace detail
	req3 := httptest.NewRequest("GET", "/api/traces/t1", nil)
	w3 := httptest.NewRecorder()
	server.ServeHTTP(w3, req3)
	fmt.Printf("GET /api/traces/t1 -> %d\n", w3.Code)

	var resp map[string]interface{}
	json.Unmarshal(w3.Body.Bytes(), &resp)
	spans := resp["spans"]
	if spans == nil {
		fmt.Println("BUG: spans is nil")
	} else if arr, ok := spans.([]interface{}); ok {
		fmt.Printf("OK: %d spans returned\n", len(arr))
	} else {
		fmt.Printf("BUG: spans is not array, got %T\n", spans)
	}
}
