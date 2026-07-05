"""
AgentTrace - Python SDK

Monkey-patches the openai module to automatically record traces.
Usage:
  import agenttrace
  agenttrace.init("http://localhost:8080")  # one line to enable

  # All subsequent OpenAI calls are automatically traced
  from openai import OpenAI
  client = OpenAI()
  response = client.chat.completions.create(model="gpt-4o", messages=[...])
  # -> trace + span automatically sent to AgentTrace server
"""

import os
import time
import json
import threading
from typing import Optional, Any
from urllib.request import Request, urlopen
from urllib.error import URLError

__version__ = "0.1.0"

_TRACE_ID: Optional[str] = None
_SERVER_URL: str = "http://localhost:8080"
_SESSION_ID: str = ""
_ENABLED: bool = False


def init(server_url: str = "http://localhost:8080", session_id: str = ""):
    """Enable tracing. Call once at the start of your application."""
    global _SERVER_URL, _SESSION_ID, _ENABLED
    _SERVER_URL = server_url.rstrip("/")
    _SESSION_ID = session_id
    _ENABLED = True
    _patch_openai()


def start_trace(name: str) -> str:
    """Start a new trace and return its ID."""
    global _TRACE_ID
    trace_id = _make_trace_id()
    _send("/api/traces", {
        "id": trace_id,
        "name": name,
        "session_id": _SESSION_ID,
    })
    _TRACE_ID = trace_id
    return trace_id


def end_trace():
    """End the current trace."""
    global _TRACE_ID
    _TRACE_ID = None


def _make_id() -> str:
    """Generate 16-char hex string."""
    return os.urandom(8).hex()

def _make_trace_id() -> str:
    """Generate 32-char hex string — OTel-compatible 16-byte TraceID."""
    return os.urandom(16).hex()

def _make_span_id() -> str:
    """Generate 16-char hex string — OTel-compatible 8-byte SpanID."""
    return os.urandom(8).hex()


def _send(path: str, data: dict) -> bool:
    if not _ENABLED:
        return False
    try:
        body = json.dumps(data).encode("utf-8")
        req = Request(f"{_SERVER_URL}{path}", data=body, headers={
            "Content-Type": "application/json"
        })
        urlopen(req, timeout=2)
        return True
    except URLError:
        return False  # Best-effort: don't crash if server is down


def _estimate_cost(model: str, prompt_tokens: int, completion_tokens: int) -> float:
    """Quick cost estimate per 1M tokens. Covers major providers."""
    prompt_price = 2.50
    completion_price = 10.0
    model_lower = model.lower()
    if "gpt-4o" in model_lower:
        prompt_price, completion_price = 2.50, 10.0
    elif "gpt-4" in model_lower:
        prompt_price, completion_price = 30.0, 60.0
    elif "gpt-3.5" in model_lower:
        prompt_price, completion_price = 0.50, 1.50
    elif "claude" in model_lower:
        prompt_price, completion_price = 3.0, 15.0
    elif "deepseek" in model_lower:
        # DeepSeek: $0.27/M input, $1.10/M output for V3; ~$0.14/M for R1 (cache hit rate varies)
        if "r1" in model_lower or "reasoner" in model_lower:
            prompt_price, completion_price = 0.55, 2.19
        else:
            prompt_price, completion_price = 0.27, 1.10
    return (prompt_tokens / 1_000_000 * prompt_price) + (completion_tokens / 1_000_000 * completion_price)

def _detect_provider(model: str) -> str:
    """Detect provider from model name."""
    ml = model.lower()
    if "deepseek" in ml: return "deepseek"
    if "claude" in ml: return "anthropic"
    if "gpt" in ml or "o1" in ml or "o3" in ml or "o4" in ml: return "openai"
    return "openai"


def _patch_openai():
    """Monkey-patch OpenAI's chat.completions.create to record spans."""
    try:
        import openai
        from openai.types.chat import ChatCompletion
    except ImportError:
        return  # openai not installed, nothing to patch

    original_create = openai.resources.chat.completions.Completions.create

    def traced_create(self, **kwargs):
        if not _ENABLED:
            return original_create(self, **kwargs)

        trace_id = _TRACE_ID or _make_trace_id()
        span_id = _make_span_id()
        request_model = kwargs.get("model", "unknown")
        messages = kwargs.get("messages", [])

        # Record input
        input_json = json.dumps([{"role": m.get("role", ""), "content": str(m.get("content", ""))[:500]} for m in messages], ensure_ascii=False)

        start_time = time.time()
        error_msg = ""
        result = None
        try:
            result = original_create(self, **kwargs)
            return result
        except Exception as e:
            error_msg = str(e)
            raise
        finally:
            elapsed_ms = (time.time() - start_time) * 1000
            output_json = ""
            prompt_tokens = 0
            completion_tokens = 0
            # Always use the response model (true model), fall back to request model
            actual_model = getattr(result, "model", None) or request_model

            if not error_msg and result is not None:
                usage = getattr(result, "usage", None)
                if usage:
                    prompt_tokens = getattr(usage, "prompt_tokens", 0) or 0
                    completion_tokens = getattr(usage, "completion_tokens", 0) or 0
                output_snippet = ""
                choices = getattr(result, "choices", [])
                if choices:
                    msg = getattr(choices[0], "message", None)
                    if msg:
                        output_snippet = str(getattr(msg, "content", ""))[:500]
                output_json = json.dumps({"content": output_snippet}, ensure_ascii=False)

            cost = _estimate_cost(actual_model, prompt_tokens, completion_tokens)

            span_data = {
                "id": span_id,
                "trace_id": trace_id,
                "name": f"chat.completions.create({actual_model})",
                "kind": "LLM",
                "model": actual_model,
                "provider": _detect_provider(actual_model),
                "input_json": input_json,
                "output_json": output_json,
                "prompt_tokens": prompt_tokens,
                "completion_tokens": completion_tokens,
                "total_tokens": prompt_tokens + completion_tokens,
                "cost": cost,
                "metadata": {"sdk": "agenttrace-python", "duration_ms": round(elapsed_ms, 2)},
            }
            if error_msg:
                span_data["status"] = "error"
                span_data["error_message"] = error_msg

            _send("/api/spans", span_data)

    openai.resources.chat.completions.Completions.create = traced_create
