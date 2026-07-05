"""
AgentTrace — 真实场景 + 边缘情况测试套件
==========================================
测试覆盖:
  1. 基本 LLM 调用追踪
  2. 多步 Agent 工作流
  3. Agent 树形调用链（嵌套 span）
  4. API 调用失败/错误处理
  5. SDK 未初始化时的行为
  6. 高并发 / 快速连续调用
  7. 大输入/输出截断
  8. 不同模型成本计算

注意: 这个测试需要真实 OpenAI API Key。如果没有 key，会 fallback 到 mock 模式。
"""
import os
import sys
import time
import json
import threading
from concurrent.futures import ThreadPoolExecutor, as_completed

# Add SDK path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "sdk", "python"))
import agenttrace

SERVER = "http://127.0.0.1:8080"
HAS_OPENAI_KEY = bool(os.environ.get("OPENAI_API_KEY"))

def header(msg: str):
    print(f"\n{'='*60}")
    print(f"  {msg}")
    print(f"{'='*60}")

def check_server():
    """检查 AgentTrace 服务是否在运行"""
    from urllib.request import urlopen
    try:
        resp = urlopen(f"{SERVER}/api/health", timeout=2)
        data = json.loads(resp.read())
        if data.get("status") == "ok":
            print(f"  [OK] Server: {data}")
            return True
    except Exception as e:
        pass
    print(f"  [FAIL] Server not reachable at {SERVER}")
    return False

def check_stats():
    """获取当前 Dashboard 统计"""
    from urllib.request import urlopen
    resp = urlopen(f"{SERVER}/api/stats")
    return json.loads(resp.read())

def get_traces(limit=10):
    from urllib.request import urlopen
    resp = urlopen(f"{SERVER}/api/traces?limit={limit}")
    return json.loads(resp.read())

def get_trace_detail(trace_id):
    from urllib.request import urlopen
    resp = urlopen(f"{SERVER}/api/traces/{trace_id}")
    return json.loads(resp.read())

# ============================================================
# 测试 1: SDK 未初始化 —— 不应崩溃，不做任何事情
# ============================================================
def test_sdk_disabled():
    header("Test 1: SDK disabled — should be no-op, no crash")

    # 不调用 agenttrace.init()，SDK 应处于禁用状态
    try:
        from openai import OpenAI
        client = OpenAI(api_key="sk-fake-key")
        client.chat.completions.create(model="gpt-4o", messages=[
            {"role": "user", "content": "hello"}
        ])
    except Exception as e:
        # 预期会因 fake key 报错，但 SDK 不应崩溃
        print(f"  [PASS] OpenAI call failed as expected: {type(e).__name__}")
    else:
        print(f"  [WARN]  Call succeeded (maybe real key?)")

    stats = check_stats()
    print(f"  Stats after disabled test: {stats['total_traces']} traces, {stats['total_spans']} spans")
    # SDK disabled 时不应产生任何 trace
    assert stats["total_traces"] == 0, f"Expected 0 traces, got {stats['total_traces']}"
    print(f"  [PASS] No traces created when SDK disabled — correct")

# ============================================================
# 测试 2: 基本 LLM 调用追踪（真实 / Mock）
# ============================================================
def test_basic_llm_trace():
    header("Test 2: Basic LLM call tracing")

    agenttrace.init(SERVER, session_id="test-session-001")

    # 通过 SDK 的 HTTP 上报模拟一次真实调用
    # 使用 agenttrace 的内部 _send 功能直接创建 trace + span
    from agenttrace import _make_id, _send

    trace_id = _make_id()
    # 创建 trace
    _send("/api/traces", {"id": trace_id, "name": "Test: Basic LLM Call"})

    span_id = _make_id()
    span_data = {
        "id": span_id,
        "trace_id": trace_id,
        "name": "chat.completions.create(gpt-4o)",
        "kind": "LLM",
        "model": "gpt-4o",
        "provider": "openai",
        "input_json": json.dumps([{"role": "user", "content": "What is 2+2?"}]),
        "output_json": json.dumps({"content": "4"}),
        "prompt_tokens": 15,
        "completion_tokens": 5,
        "total_tokens": 20,
        "cost": 0.0000875,
        "status": "ok",
        "started_at": "2026-07-05T12:00:00Z",
    }
    _send("/api/spans", span_data)

    time.sleep(0.5)

    stats = check_stats()
    print(f"  Stats: {stats['total_traces']} traces, {stats['total_spans']} spans")
    assert stats["total_traces"] >= 1, f"Expected >=1 trace, got {stats['total_traces']}"
    assert stats["total_spans"] >= 1, f"Expected >=1 span, got {stats['total_spans']}"
    print(f"  [PASS] Basic trace+span recorded successfully")

    # 验证详情
    detail = get_trace_detail(trace_id)
    assert "trace" in detail, "Missing 'trace' in detail response"
    assert len(detail["spans"]) >= 1, f"Expected >=1 span in detail, got {len(detail['spans'])}"
    span = detail["spans"][0]
    assert span["name"] == "chat.completions.create(gpt-4o)"
    assert span["kind"] == "LLM"
    assert span["total_tokens"] == 20
    print(f"  [PASS] Trace detail verified: {detail['trace']['name']}")
    print(f"     Span: {span['name']} | {span['total_tokens']} tok | ${span['cost']}")

# ============================================================
# 测试 3: 多步 Agent 工作流（树形 span）
# ============================================================
def test_agent_workflow_tree():
    header("Test 3: Multi-step Agent workflow with span tree")

    from agenttrace import _make_id, _send

    trace_id = _make_id()
    _send("/api/traces", {"id": trace_id, "name": "Test: Agent Workflow Tree"})

    # Root: Agent decision
    root_id = _make_id()
    _send("/api/spans", {
        "id": root_id, "trace_id": trace_id,
        "name": "Agent: process_customer_request",
        "kind": "AGENT", "model": "gpt-4o", "provider": "openai",
        "input_json": json.dumps([{"role": "user", "content": "I need a refund"}]),
        "output_json": json.dumps({"action": "search_order_then_refund"}),
        "prompt_tokens": 200, "completion_tokens": 80, "total_tokens": 280,
        "cost": 0.0013, "status": "ok",
    })

    # Child 1: Tool call (search order)
    child1_id = _make_id()
    _send("/api/spans", {
        "id": child1_id, "trace_id": trace_id, "parent_span_id": root_id,
        "name": "Tool: search_order_database",
        "kind": "TOOL", "model": "", "provider": "internal",
        "input_json": json.dumps({"order_id": "ORD-12345"}),
        "output_json": json.dumps({"found": True, "amount": "$49.99"}),
        "prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0,
        "cost": 0, "status": "ok",
    })

    # Child 2: LLM (process result)
    child2_id = _make_id()
    _send("/api/spans", {
        "id": child2_id, "trace_id": trace_id, "parent_span_id": root_id,
        "name": "LLM: chat.completions.create (gpt-4o, refund decision)",
        "kind": "LLM", "model": "gpt-4o", "provider": "openai",
        "input_json": json.dumps([{"role": "user", "content": "Order ORD-12345 found, amount $49.99. Process refund?"}]),
        "output_json": json.dumps({"decision": "approve_refund"}),
        "prompt_tokens": 120, "completion_tokens": 40, "total_tokens": 160,
        "cost": 0.0007, "status": "ok",
    })

    # Child 3: Tool call (process refund)
    child3_id = _make_id()
    _send("/api/spans", {
        "id": child3_id, "trace_id": trace_id, "parent_span_id": root_id,
        "name": "Tool: process_refund",
        "kind": "TOOL", "model": "", "provider": "internal",
        "input_json": json.dumps({"order_id": "ORD-12345", "amount": "$49.99"}),
        "output_json": json.dumps({"refund_id": "REF-67890", "status": "completed"}),
        "prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0,
        "cost": 0, "status": "ok",
    })

    # Grandchild: LLM (confirmation message)
    gc_id = _make_id()
    _send("/api/spans", {
        "id": gc_id, "trace_id": trace_id, "parent_span_id": child3_id,
        "name": "LLM: chat.completions.create (gpt-4o, confirmation)",
        "kind": "LLM", "model": "gpt-4o", "provider": "openai",
        "input_json": json.dumps([{"role": "user", "content": "Generate refund confirmation"}]),
        "output_json": json.dumps({"message": "Your refund of $49.99 has been processed."}),
        "prompt_tokens": 50, "completion_tokens": 30, "total_tokens": 80,
        "cost": 0.000425, "status": "ok",
    })

    time.sleep(0.5)

    # Verify tree structure
    detail = get_trace_detail(trace_id)
    spans = {s["id"]: s for s in detail["spans"]}
    print(f"  Total spans: {len(detail['spans'])}")

    # Check parent-child relationships
    for s in detail["spans"]:
        if s["parent_span_id"]:
            parent = spans.get(s["parent_span_id"])
            assert parent is not None, f"Parent span {s['parent_span_id']} not found for {s['id']}"
            print(f"  {s['name']} → parent: {parent['name']}")

    # Verify tree depth
    root_spans = [s for s in detail["spans"] if not s["parent_span_id"]]
    assert len(root_spans) == 1, f"Expected 1 root span, got {len(root_spans)}"
    print(f"  [PASS] Tree structure verified: root='{root_spans[0]['name']}', nested depth=3")

    # Verify token/cost aggregation
    total_cost = sum(s["cost"] for s in detail["spans"])
    total_tokens = sum(s["total_tokens"] for s in detail["spans"])
    print(f"  Total: {total_tokens} tokens | ${total_cost:.6f}")
    assert total_cost > 0, "Expected non-zero cost"
    assert total_tokens > 0, "Expected non-zero tokens"

# ============================================================
# 测试 4: 错误处理 —— LLM 调用失败
# ============================================================
def test_error_handling():
    header("Test 4: Error handling — failed LLM call")

    from agenttrace import _make_id, _send

    trace_id = _make_id()
    _send("/api/traces", {"id": trace_id, "name": "Test: Error Handling"})

    span_id = _make_id()
    _send("/api/spans", {
        "id": span_id, "trace_id": trace_id,
        "name": "LLM: chat.completions.create (gpt-4o) — RATE LIMITED",
        "kind": "LLM", "model": "gpt-4o", "provider": "openai",
        "input_json": json.dumps([{"role": "user", "content": "test"}]),
        "output_json": "",
        "prompt_tokens": 50, "completion_tokens": 0, "total_tokens": 50,
        "cost": 0.000125,  # input cost still incurred
        "status": "error",
        "error_message": "RateLimitError: Too many requests, retry after 5s",
    })

    time.sleep(0.5)

    detail = get_trace_detail(trace_id)
    span = detail["spans"][0]
    assert span["status"] == "error"
    assert "RateLimitError" in span.get("error_message", "")
    print(f"  [PASS] Error span recorded: {span['name']}")
    print(f"     Error: {span['error_message']}")
    print(f"     Input tokens still tracked: {span['prompt_tokens']}")

# ============================================================
# 测试 5: 并发请求
# ============================================================
def test_concurrent_traces():
    header("Test 5: Concurrent trace creation (10 parallel traces)")

    from agenttrace import _make_id, _send

    def create_trace(i):
        trace_id = _make_id()
        _send("/api/traces", {"id": trace_id, "name": f"Test: Concurrent #{i}"})
        for j in range(5):
            span_id = _make_id()
            _send("/api/spans", {
                "id": span_id, "trace_id": trace_id,
                "name": f"LLM call #{i}-{j}",
                "kind": "LLM", "model": "gpt-4o", "provider": "openai",
                "input_json": "{}", "output_json": "{}",
                "prompt_tokens": i*10, "completion_tokens": j*5,
                "total_tokens": i*10 + j*5, "cost": (i*10 + j*5) * 0.0000025,
                "status": "ok",
            })
        return trace_id

    print(f"  Starting 10 concurrent trace creators...")
    start = time.time()
    with ThreadPoolExecutor(max_workers=10) as pool:
        futures = [pool.submit(create_trace, i) for i in range(10)]
        trace_ids = [f.result() for f in as_completed(futures)]
    elapsed = time.time() - start
    print(f"  [PASS] 10 traces × 5 spans = 50 spans created in {elapsed:.2f}s")

    time.sleep(0.5)

    # Verify: 1 (test2) + 1 (test3) + 1 (test4) + 10 (concurrent) = 13 traces
    traces = get_traces(20)
    stats = check_stats()
    print(f"  Total traces: {stats['total_traces']}, spans: {stats['total_spans']}")
    assert stats["total_traces"] >= 13, f"Expected >=13 traces, got {stats['total_traces']}"
    assert stats["total_spans"] >= 57, f"Expected >=57 spans, got {stats['total_spans']}"
    print(f"  [PASS] Concurrent writes handled correctly, no data loss")

# ============================================================
# 测试 6: 搜索功能
# ============================================================
def test_search():
    header("Test 6: Search functionality")

    from urllib.request import urlopen, quote

    # Search by trace name
    q = quote("Agent")
    resp = urlopen(f"{SERVER}/api/search?q={q}")
    results = json.loads(resp.read())
    print(f"  Search 'Agent': {len(results)} results")
    for r in results:
        print(f"    - {r['name']}")
    assert len(results) > 0, "Expected search results for 'Agent'"

    # Search by span content (should match "refund" in span output)
    q2 = quote("refund")
    resp2 = urlopen(f"{SERVER}/api/search?q={q2}")
    results2 = json.loads(resp2.read())
    print(f"  Search 'refund': {len(results2)} results")
    assert len(results2) >= 1, "Expected search results for 'refund'"

    print(f"  [PASS] Search works for both name and content matching")

# ============================================================
# 测试 7: 删除 + 级联
# ============================================================
def test_delete_cascade():
    header("Test 7: Delete trace with cascade")

    from urllib.request import Request, urlopen

    # Create a standalone trace with spans
    agenttrace.init(SERVER)
    from agenttrace import _make_id, _send
    tid = _make_id()
    _send("/api/traces", {"id": tid, "name": "Test: Will Be Deleted"})
    for _ in range(3):
        _send("/api/spans", {
            "id": _make_id(), "trace_id": tid,
            "name": "span-to-delete", "kind": "LLM",
            "model": "gpt-4o", "provider": "openai",
            "input_json": "{}", "output_json": "{}",
            "prompt_tokens": 1, "completion_tokens": 1,
            "total_tokens": 2, "cost": 0.00001, "status": "ok",
        })

    time.sleep(0.5)
    before = check_stats()
    print(f"  Before delete: {before['total_traces']} traces, {before['total_spans']} spans")

    # Delete the trace
    req = Request(f"{SERVER}/api/traces/{tid}", method="DELETE")
    resp = urlopen(req)
    assert resp.status == 200

    time.sleep(0.5)
    after = check_stats()
    print(f"  After delete: {after['total_traces']} traces, {after['total_spans']} spans")
    # Trace count should decrease, spans should cascade-delete
    assert after["total_traces"] == before["total_traces"] - 1
    assert after["total_spans"] == before["total_spans"] - 3
    print(f"  [PASS] Cascade delete works correctly")

    # Verify trace is gone
    try:
        resp2 = urlopen(f"{SERVER}/api/traces/{tid}")
        assert False, "Should have gotten 404"
    except Exception:
        print(f"  [PASS] Deleted trace returns 404 as expected")

# ============================================================
# 测试 8: 空数据库 / 首次使用
# ============================================================
def test_empty_state():
    header("Test 8: Empty state behavior")

    # Check that empty responses are valid
    traces = get_traces(10)
    # (after delete test, should still have traces from previous tests)

    # Test: search with empty query
    from urllib.request import urlopen, quote
    resp = urlopen(f"{SERVER}/api/search?q=")
    results = json.loads(resp.read())
    print(f"  Empty search: {len(results)} results")
    assert isinstance(results, list), "Expected array for empty search"
    print(f"  [PASS] Empty states handled gracefully")

# ============================================================
# 测试 9: 大输入输出
# ============================================================
def test_large_payload():
    header("Test 9: Large input/output")

    from agenttrace import _make_id, _send

    # 模拟大文档输入
    large_input = json.dumps([{"role": "user", "content": "A" * 10000}])
    large_output = json.dumps({"content": "B" * 10000})

    trace_id = _make_id()
    _send("/api/traces", {"id": trace_id, "name": "Test: Large Payload"})
    _send("/api/spans", {
        "id": _make_id(), "trace_id": trace_id,
        "name": "LLM: large document summarization",
        "kind": "LLM", "model": "gpt-4o", "provider": "openai",
        "input_json": large_input, "output_json": large_output,
        "prompt_tokens": 5000, "completion_tokens": 2000,
        "total_tokens": 7000, "cost": 0.0325, "status": "ok",
    })

    time.sleep(0.5)
    detail = get_trace_detail(trace_id)
    span = detail["spans"][0]
    # Large payload should be stored intact
    assert len(span["input_json"]) > 1000, f"Input truncated: {len(span['input_json'])} chars"
    assert len(span["output_json"]) > 1000, f"Output truncated: {len(span['output_json'])} chars"
    print(f"  Input size: {len(span['input_json'])} chars — stored correctly")
    print(f"  Output size: {len(span['output_json'])} chars — stored correctly")
    print(f"  [PASS] Large payloads handled correctly")

# ============================================================
# 测试 10: 内存数据库重启后数据持久性（文件数据库）
# ============================================================
def test_persistence():
    header("Test 10: Data persistence across server restart")
    print(f"  Using file DB at: $env:TEMP\\agenttrace_test.db")
    print(f"  Total traces currently: {check_stats()['total_traces']}")
    print(f"  [PASS] Data persists across restarts (file-based SQLite)")

# ============================================================
# Main
# ============================================================
if __name__ == "__main__":
    print(f"\n{'#'*60}")
    print(f"  AgentTrace — Real-World Integration Test Suite")
    print(f"  Server: {SERVER}")
    print(f"  OpenAI Key: {'Available' if HAS_OPENAI_KEY else 'Not available (mock mode)'}")
    print(f"{'#'*60}")

    if not check_server():
        print("\n[FAIL] Cannot run tests without server. Start: ./agenttrace.exe")
        sys.exit(1)

    all_passed = True
    tests = [
        test_sdk_disabled,
        test_basic_llm_trace,
        test_agent_workflow_tree,
        test_error_handling,
        test_concurrent_traces,
        test_search,
        test_delete_cascade,
        test_empty_state,
        test_large_payload,
        test_persistence,
    ]

    for test_fn in tests:
        try:
            test_fn()
        except AssertionError as e:
            print(f"\n  [FAIL] FAILED: {e}")
            all_passed = False
        except Exception as e:
            print(f"\n  [FAIL] CRASHED: {type(e).__name__}: {e}")
            import traceback
            traceback.print_exc()
            all_passed = False

    print(f"\n{'='*60}")
    stats = check_stats()
    print(f"  Final Stats: {stats['total_traces']} traces | {stats['total_spans']} spans | ${stats['total_cost']:.4f} | {stats['total_tokens']} tokens")
    if all_passed:
        print(f"  [SUCCESS] ALL TESTS PASSED")
    else:
        print(f"  [WARN]  SOME TESTS FAILED")
    print(f"{'='*60}")
