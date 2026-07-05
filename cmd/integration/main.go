package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/iFurySt/agenttrace/internal/api"
	"github.com/iFurySt/agenttrace/internal/storage"
)

var serverURL string

func main() {
	// Start in-process server
	store, _ := storage.Open(":memory:")
	handler := api.NewServer(store)

	// Use httptest-style direct calls
	fmt.Println("=== AgentTrace Integration Test Suite ===")
	fmt.Println()

	// Test 1: Create trace + span
	fmt.Print("Test 1: Basic create trace + span ... ")
	traceJSON := `{"id":"t1","name":"Customer Support Agent"}`
	tr := doPost(handler, "/api/traces", traceJSON, 201)
	if tr["id"] != "t1" {
		fail("trace id mismatch")
		return
	}

	spanJSON := `{"id":"s1","trace_id":"t1","name":"LLM: chat.completions.create(gpt-4o)","kind":"LLM","status":"ok","model":"gpt-4o","provider":"openai","input_json":"{}","output_json":"{}","prompt_tokens":1200,"completion_tokens":450,"total_tokens":1650,"cost":0.0075,"started_at":"2026-07-05T12:00:00Z","ended_at":"2026-07-05T12:00:01Z"}`
	sp := doPost(handler, "/api/spans", spanJSON, 201)
	if sp["id"] != "s1" {
		fail("span id mismatch")
		return
	}
	pass()

	// Test 2: Read back trace with spans
	fmt.Print("Test 2: Read trace detail with spans ... ")
	detail := doGet(handler, "/api/traces/t1", 200).(map[string]interface{})
	spans := detail["spans"].([]interface{})
	if len(spans) != 1 {
		fail(fmt.Sprintf("expected 1 span, got %d", len(spans)))
		return
	}
	s1 := spans[0].(map[string]interface{})
	if s1["name"] != "LLM: chat.completions.create(gpt-4o)" {
		fail("span name mismatch")
		return
	}
	if s1["total_tokens"].(float64) != 1650 {
		fail("token count mismatch")
		return
	}
	pass()

	// Test 3: Create nested span tree
	fmt.Print("Test 3: Nested span tree (agent ???tool ???llm) ... ")
	doPost(handler, "/api/traces", `{"id":"t2","name":"Agent Workflow"}`, 201)
	doPost(handler, "/api/spans", `{"id":"s2_root","trace_id":"t2","name":"Agent: process_request","kind":"AGENT","status":"ok","model":"gpt-4o","prompt_tokens":200,"completion_tokens":80,"total_tokens":280,"cost":0.0013,"input_json":"{}","output_json":"{}","started_at":"2026-07-05T12:00:00Z","ended_at":"2026-07-05T12:00:03Z"}`, 201)
	doPost(handler, "/api/spans", `{"id":"s2_tool","trace_id":"t2","parent_span_id":"s2_root","name":"Tool: search_database","kind":"TOOL","status":"ok","prompt_tokens":0,"completion_tokens":0,"total_tokens":0,"cost":0,"input_json":"{}","output_json":"{}","started_at":"2026-07-05T12:00:01Z","ended_at":"2026-07-05T12:00:02Z"}`, 201)
	doPost(handler, "/api/spans", `{"id":"s2_llm","trace_id":"t2","parent_span_id":"s2_root","name":"LLM: final_response","kind":"LLM","status":"ok","model":"gpt-4o","prompt_tokens":100,"completion_tokens":50,"total_tokens":150,"cost":0.00075,"input_json":"{}","output_json":"{}","started_at":"2026-07-05T12:00:02Z","ended_at":"2026-07-05T12:00:03Z"}`, 201)

	t2 := doGet(handler, "/api/traces/t2", 200).(map[string]interface{})
	t2spans := t2["spans"].([]interface{})
	if len(t2spans) != 3 {
		fail(fmt.Sprintf("expected 3 spans, got %d", len(t2spans)))
		return
	}
	// Verify tree: 1 root, 2 children
	rootCount := 0
	childCount := 0
	for _, s := range t2spans {
		sm := s.(map[string]interface{})
		if sm["parent_span_id"] == nil || sm["parent_span_id"] == "" {
			rootCount++
		} else {
			childCount++
		}
	}
	if rootCount != 1 || childCount != 2 {
		fail(fmt.Sprintf("tree structure wrong: %d roots, %d children", rootCount, childCount))
		return
	}
	pass()

	// Test 4: Error span
	fmt.Print("Test 4: Error span handling ... ")
	doPost(handler, "/api/traces", `{"id":"t3","name":"Error Test"}`, 201)
	doPost(handler, "/api/spans", `{"id":"s3_err","trace_id":"t3","name":"LLM: rate_limited","kind":"LLM","status":"error","error_message":"RateLimitError: too many requests","model":"gpt-4o","prompt_tokens":50,"completion_tokens":0,"total_tokens":50,"cost":0.000125,"input_json":"{}","output_json":"{}","started_at":"2026-07-05T12:00:00Z"}`, 201)

	t3 := doGet(handler, "/api/traces/t3", 200).(map[string]interface{})
	t3spans := t3["spans"].([]interface{})
	if len(t3spans) == 0 {
		fail("no spans found")
		return
	}
	errSpan := t3spans[0].(map[string]interface{})
	if errSpan["status"] != "error" {
		fail("status not error")
		return
	}
	if errSpan["error_message"].(string) == "" {
		fail("error_message empty")
		return
	}
	pass()

	// Test 5: Stats accuracy
	fmt.Print("Test 5: Dashboard stats accuracy ... ")
	stats := doGet(handler, "/api/stats", 200).(map[string]interface{})
	traces := int(stats["total_traces"].(float64))
	totalSpans := int(stats["total_spans"].(float64))
	totalCost := stats["total_cost"].(float64)
	if traces != 3 {
		fail(fmt.Sprintf("expected 3 traces, got %d", traces))
		return
	}
	expectedSpans := 1 + 3 + 1 // t1:1, t2:3, t3:1
	if totalSpans != expectedSpans {
		fail(fmt.Sprintf("expected %d spans, got %d", expectedSpans, totalSpans))
		return
	}
	// Expected cost: 0.0075 + 0.0013 + 0.00075 + 0.000125 = 0.009675
	if totalCost < 0.009 || totalCost > 0.010 {
		fail(fmt.Sprintf("cost out of range: %.6f", totalCost))
		return
	}
	pass()

	// Test 6: Delete trace
	fmt.Print("Test 6: Delete trace ... ")
	doDelete(handler, "/api/traces/t3", 200)
	stats2 := doGet(handler, "/api/stats", 200).(map[string]interface{})
	if int(stats2["total_traces"].(float64)) != 2 {
		fail("trace count didn't decrease")
		return
	}
	// Try to get deleted trace
	_ = doGet(handler, "/api/traces/t3", 404)
	pass()

	// Test 7: Search
	fmt.Print("Test 7: Search functionality ... ")
	searchResult := doGet(handler, "/api/search?q=Agent", 200)
	results := searchResult.([]interface{})
	if len(results) < 1 {
		fail("search found nothing")
		return
	}
	searchResult2 := doGet(handler, "/api/search?q=database", 200)
	results2 := searchResult2.([]interface{})
	if len(results2) < 1 {
		fail("content search found nothing")
		return
	}
	pass()

	// Test 8: Empty states
	fmt.Print("Test 8: Empty/missing data handling ... ")
	_ = doGet(handler, "/api/traces/nonexistent", 404)
	emptySearch := doGet(handler, "/api/search?q=", 200)
	if emptySearch == nil {
		fail("empty search should return []")
		return
	}
	pass()

	// Test 9: List with pagination
	fmt.Print("Test 9: List traces with limit/offset ... ")
	list := doGet(handler, "/api/traces?limit=1&offset=0", 200)
	listArr := list.([]interface{})
	if len(listArr) != 1 {
		fail(fmt.Sprintf("expected 1 trace with limit=1, got %d", len(listArr)))
		return
	}
	pass()

	// Test 10: Span without started_at (auto-generated)
	fmt.Print("Test 10: Span without started_at ... ")
	doPost(handler, "/api/spans", `{"id":"s_auto","trace_id":"t1","name":"auto-time-span","kind":"LLM","status":"ok","model":"gpt-4o","input_json":"{}","output_json":"{}","prompt_tokens":1,"completion_tokens":1,"total_tokens":2,"cost":0.001}`, 201)
	gotSpan := doGet(handler, "/api/spans/s_auto", 200).(map[string]interface{})
	if gotSpan["started_at"] == nil || gotSpan["started_at"].(string) == "" {
		fail("started_at not auto-generated")
		return
	}
	if gotSpan["created_at"] == nil || gotSpan["created_at"].(string) == "" {
		fail("created_at not set")
		return
	}
	pass()

	fmt.Println()
	fmt.Println("=" + strings.Repeat("=", 59))
	fmt.Println("  ALL 10 INTEGRATION TESTS PASSED")
	fmt.Println("=" + strings.Repeat("=", 59))
	fmt.Println()
	fmt.Printf("  Traces: %d | Spans: %d | Cost: $%.4f\n",
		int(stats["total_traces"].(float64)), int(stats["total_spans"].(float64)), stats["total_cost"].(float64))
}

func doPost(handler http.Handler, path, body string, expectStatus int) map[string]interface{} {
	req, _ := http.NewRequest("POST", path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := &responseRecorder{header: make(http.Header)}
	handler.ServeHTTP(w, req)
	if w.status != expectStatus {
		panic(fmt.Sprintf("POST %s: expected %d, got %d: %s", path, expectStatus, w.status, w.body.String()))
	}
	var result map[string]interface{}
	json.Unmarshal(w.body.Bytes(), &result)
	return result
}

func doGet(handler http.Handler, path string, expectStatus int) interface{} {
	req, _ := http.NewRequest("GET", path, nil)
	w := &responseRecorder{header: make(http.Header)}
	handler.ServeHTTP(w, req)
	if w.status != expectStatus {
		panic(fmt.Sprintf("GET %s: expected %d, got %d", path, expectStatus, w.status))
	}
	var result interface{}
	json.Unmarshal(w.body.Bytes(), &result)
	return result
}

func doDelete(handler http.Handler, path string, expectStatus int) {
	req, _ := http.NewRequest("DELETE", path, nil)
	w := &responseRecorder{header: make(http.Header)}
	handler.ServeHTTP(w, req)
	if w.status != expectStatus {
		panic(fmt.Sprintf("DELETE %s: expected %d, got %d", path, expectStatus, w.status))
	}
}

type responseRecorder struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (r *responseRecorder) Header() http.Header         { return r.header }
func (r *responseRecorder) Write(b []byte) (int, error) { return r.body.Write(b) }
func (r *responseRecorder) WriteHeader(status int)       { r.status = status }

func fail(msg string) { fmt.Printf("FAIL: %s\n", msg) }
func pass()           { fmt.Println("PASS") }

// Go embedded struct embeds time -> unused but needed for compile
var _ = time.Now
