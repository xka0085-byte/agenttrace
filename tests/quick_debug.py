import sys, json, urllib.request
sys.path.insert(0, 'sdk/python')
import agenttrace
from agenttrace import _send, _make_id

agenttrace._ENABLED = True
agenttrace._SERVER_URL = 'http://127.0.0.1:8080'

# Test without started_at
tid = _make_id()
_send('/api/traces', {'id': tid, 'name': 'no-started-at test'})
result = _send('/api/spans', {
    'id': _make_id(), 'trace_id': tid, 'name': 'span-no-time',
    'kind': 'LLM', 'model': 'g', 'provider': 'o',
    'input_json': '{}', 'output_json': '{}',
    'prompt_tokens': 1, 'completion_tokens': 1, 'total_tokens': 2,
    'cost': 0.001, 'status': 'ok',
})
print(f'Send result: {result}')

r = urllib.request.urlopen(f'http://127.0.0.1:8080/api/traces/{tid}')
d = json.loads(r.read())
print(f'Spans without started_at: {len(d["spans"])}')
if len(d["spans"]) == 0:
    print("BUG REPRODUCED: span without started_at is not persisted!")
else:
    print("OK: span persisted correctly")
