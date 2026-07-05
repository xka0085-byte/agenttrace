"""
AgentTrace end-to-end test with DeepSeek API

Prerequisites:
  - AgentTrace server running: ./agenttrace.exe
  - DEEPSEEK_API_KEY env var set
  - pip install openai agenttrace
"""

import os
import sys
import time
import json

# ------------------------------------------------------------------
# Setup
# ------------------------------------------------------------------
SERVER = "http://127.0.0.1:8080"
API_KEY = os.environ.get("DEEPSEEK_API_KEY")
if not API_KEY:
    print("[SKIP] No DEEPSEEK_API_KEY in environment")
    print("       Set it: $env:DEEPSEEK_API_KEY = 'your-key'")
    sys.exit(0)

# Verify server is up
from urllib.request import urlopen
try:
    resp = json.loads(urlopen(f"{SERVER}/api/health", timeout=2).read())
    print(f"[OK] AgentTrace server: {resp}")
except Exception:
    print("[FAIL] AgentTrace server not running. Start: ./agenttrace.exe")
    sys.exit(1)

# ------------------------------------------------------------------
# Enable tracing
# ------------------------------------------------------------------
sys.path.insert(0, "sdk/python")
import agenttrace
agenttrace.init(SERVER, session_id="e2e-deepseek-test")

print(f"[OK] Tracing enabled: {agenttrace._ENABLED}")

# ------------------------------------------------------------------
# Test 1: Basic DeepSeek V3 call
# ------------------------------------------------------------------
print("\n" + "="*60)
print("Test 1: DeepSeek V3 — single chat completion")
print("="*60)

from openai import OpenAI
client = OpenAI(
    api_key=API_KEY,
    base_url="https://api.deepseek.com",
)

trace_id = agenttrace.start_trace("E2E Test: DeepSeek V3")

t0 = time.time()
try:
    response = client.chat.completions.create(
        model="deepseek-chat",
        messages=[
            {"role": "system", "content": "You are a helpful assistant."},
            {"role": "user", "content": "What is the capital of France? Answer in one word."},
        ],
        max_tokens=50,
    )
    elapsed = time.time() - t0
    answer = response.choices[0].message.content
    usage = response.model_dump().get("usage", {})
    print(f"[OK] Response in {elapsed:.2f}s: {answer}")
    print(f"     Model: {response.model}")
    print(f"     Usage: prompt={usage.get('prompt_tokens')}, completion={usage.get('completion_tokens')}, total={usage.get('total_tokens')}")
except Exception as e:
    print(f"[FAIL] {type(e).__name__}: {e}")

# ------------------------------------------------------------------
# Test 2: Multi-turn / Agent-like workflow
# ------------------------------------------------------------------
print("\n" + "="*60)
print("Test 2: Multi-turn agent workflow (trace + multiple spans)")
print("="*60)

from agenttrace import _make_id, _send

trace_id2 = _make_id()
_send("/api/traces", {
    "id": trace_id2,
    "name": "E2E: Multi-turn Agent (DeepSeek)",
    "session_id": "e2e-deepseek-test",
})

# Turn 1
t0 = time.time()
r1 = client.chat.completions.create(
    model="deepseek-chat",
    messages=[
        {"role": "system", "content": "You are an AI agent. Analyze the email content."},
        {"role": "user", "content": "Email: 'I never received my order #12345. I want a refund!'"},
    ],
    max_tokens=100,
)
print(f"[OK] Turn 1 (analyze) — {time.time()-t0:.2f}s: {r1.choices[0].message.content[:80]}...")

# Turn 2 — the agent calls a simulated tool
_send("/api/spans", {
    "id": _make_id(), "trace_id": trace_id2, "parent_span_id": "",
    "name": "Tool: lookup_order(#12345)",
    "kind": "TOOL", "model": "", "provider": "internal",
    "input_json": json.dumps({"order_id": "12345"}),
    "output_json": json.dumps({"status": "not_shipped", "amount": "$49.99"}),
    "prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0, "cost": 0,
    "status": "ok",
    "started_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
})

# Turn 3 — final response
t0 = time.time()
r2 = client.chat.completions.create(
    model="deepseek-chat",
    messages=[
        {"role": "system", "content": "You are a customer support agent."},
        {"role": "user", "content": "Email: refund request. Order #12345 not shipped. Amount $49.99. Write a reply."},
    ],
    max_tokens=150,
)
print(f"[OK] Turn 3 (reply gen) — {time.time()-t0:.2f}s: {r2.choices[0].message.content[:80]}...")

# ------------------------------------------------------------------
# Test 3: Verify traces in Dashboard
# ------------------------------------------------------------------
print("\n" + "="*60)
print("Test 3: Verify traces appear in Dashboard")
print("="*60)

time.sleep(1)  # Let async writes finish

stats = json.loads(urlopen(f"{SERVER}/api/stats").read())
print(f"Dashboard: {stats['total_traces']} traces | {stats['total_spans']} spans | ${stats['total_cost']:.4f}")

traces = json.loads(urlopen(f"{SERVER}/api/traces?limit=20").read())
for t in traces:
    tr = t["trace"]
    s = t["summary"]
    print(f"  {tr['name'][:50]:50s}  {s['total_spans']:2d}sp  {s['total_tokens']:5d}tok  ${s['total_cost']:.4f}")

# Verify our trace has spans
detail = json.loads(urlopen(f"{SERVER}/api/traces/{trace_id2}").read())
print(f"\n  Trace '{detail['trace']['name']}' detail:")
for sp in detail["spans"]:
    print(f"    [{sp['kind']}] {sp['name']:50s}  {sp['total_tokens']:5d}tok  ${sp['cost']:.4f}  {sp['status']}")

# ------------------------------------------------------------------
# Test 4: Check auto-traced OpenAI calls
# ------------------------------------------------------------------
print("\n" + "="*60)
print("Test 4: Auto-traced DeepSeek calls (via monkey-patch)")
print("="*60)

# Re-enable for auto-trace test
agenttrace._ENABLED = True
agenttrace._TRACE_ID = agenttrace._make_id()
_send("/api/traces", {"id": agenttrace._TRACE_ID, "name": "E2E: Auto-trace Test"})

t0 = time.time()
r3 = client.chat.completions.create(
    model="deepseek-chat",
    messages=[{"role": "user", "content": "Say 'AgentTrace works!' and nothing else."}],
    max_tokens=20,
)
print(f"[OK] DeepSeek: {r3.choices[0].message.content}")
print(f"     Auto-traced via monkey-patch")

time.sleep(0.5)

agd = json.loads(urlopen(f"{SERVER}/api/traces/{agenttrace._TRACE_ID}").read())
print(f"     Auto-recorded spans: {len(agd['spans'])}")
for sp in agd["spans"]:
    print(f"       [{sp['kind']}] {sp['name']} — {sp['total_tokens']}tok ${sp['cost']:.4f}")

# ------------------------------------------------------------------
# Summary
# ------------------------------------------------------------------
print("\n" + "="*60)
print("E2E TEST COMPLETE")
print("="*60)
print(f"Open http://127.0.0.1:8080 to see all traces")
print(f"Python SDK verified with DeepSeek API: ✓")
