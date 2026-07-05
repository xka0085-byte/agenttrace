import sys, json, urllib.request, time

sys.path.insert(0, 'sdk/python')
import agenttrace

# Test 1: verify init() sets _ENABLED
print(f"Before init: _ENABLED={agenttrace._ENABLED}")
agenttrace.init('http://127.0.0.1:8080')
print(f"After init: _ENABLED={agenttrace._ENABLED}")

from agenttrace import _make_id, _send

# Test 2: send trace + span with started_at
tid = _make_id()
_send('/api/traces', {'id': tid, 'name': 'Full Flow Test'})
print(f"Trace sent: {tid}")

sid = _make_id()
ok = _send('/api/spans', {
    'id': sid, 'trace_id': tid, 'name': 'span-1',
    'kind': 'LLM', 'model': 'gpt-4o', 'provider': 'openai',
    'input_json': '{}', 'output_json': '{}',
    'prompt_tokens': 10, 'completion_tokens': 5, 'total_tokens': 15,
    'cost': 0.001, 'status': 'ok', 'started_at': '2026-07-05T12:00:00Z',
})
print(f"Span sent: {ok}")

# Give server time to process
time.sleep(0.5)

# Test 3: verify via API
r = urllib.request.urlopen('http://127.0.0.1:8080/api/stats')
stats = json.loads(r.read())
print(f"Stats: {stats}")

r2 = urllib.request.urlopen(f'http://127.0.0.1:8080/api/traces/{tid}')
detail = json.loads(r2.read())
print(f"Trace detail: {len(detail['spans'])} spans")
for s in detail['spans']:
    print(f"  span={s['name']} kind={s['kind']} tokens={s['total_tokens']}")

# Test 4: verify in list
r3 = urllib.request.urlopen('http://127.0.0.1:8080/api/traces?limit=5')
traces = json.loads(r3.read())
for t in traces:
    if t['trace']['id'] == tid:
        print(f"List: spans={t['summary']['total_spans']}")
        break

print("\nALL OK!" if stats['total_spans'] >= 1 else "\nFAILED")
