const BASE = ''

export interface Trace {
  id: string
  name: string
  user_id: string
  session_id: string
  tags: string[]
  metadata: Record<string, any>
  created_at: string
  updated_at: string
}

export interface TraceWithSummary {
  trace: Trace
  summary: SpanSummary
}

export interface SpanSummary {
  total_spans: number
  total_cost: number
  total_tokens: number
  avg_latency_ms: number
  error_count: number
}

export interface Span {
  id: string
  trace_id: string
  parent_span_id: string
  name: string
  kind: 'LLM' | 'TOOL' | 'RETRIEVAL' | 'CHAIN' | 'AGENT'
  status: 'ok' | 'error'
  started_at: string
  ended_at: string
  model: string
  provider: string
  input_json: string
  output_json: string
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  cost: number
  error_message: string
  metadata: Record<string, any>
}

export async function fetchTraces(limit = 50): Promise<TraceWithSummary[]> {
  const res = await fetch(`${BASE}/api/traces?limit=${limit}`)
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  return res.json()
}

export async function fetchTrace(id: string): Promise<{ trace: Trace; spans: Span[] }> {
  const res = await fetch(`${BASE}/api/traces/${id}`)
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  return res.json()
}

export async function deleteTrace(id: string): Promise<boolean> {
  const res = await fetch(`${BASE}/api/traces/${id}`, { method: 'DELETE' })
  return res.ok
}

export async function fetchStats(): Promise<Record<string, number>> {
  const res = await fetch(`${BASE}/api/stats`)
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  return res.json()
}

export function formatCost(c: number): string {
  if (c < 0.01) return '<$0.01'
  return `$${c.toFixed(2)}`
}

export function formatMs(ms: number): string {
  if (ms < 1) return '<1ms'
  if (ms < 1000) return `${Math.round(ms)}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

export function formatTime(t: string): string {
  return new Date(t).toLocaleTimeString('en-US', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

export function kindColor(kind: string): string {
  const map: Record<string, string> = { LLM: '#6c8cff', TOOL: '#4ade80', RETRIEVAL: '#fbbf24', CHAIN: '#c084fc', AGENT: '#f472b6' }
  return map[kind] || '#8b90a0'
}
