import { useEffect, useState, useMemo } from 'react'
import { Link } from 'react-router-dom'
import { fetchTraces, fetchStats, deleteTrace, formatCost, formatMs, formatTime } from '../api'
import { DashboardSkeleton } from '../Skeleton'
import type { TraceWithSummary } from '../api'

const EMPTY = { total_traces: 0, total_spans: 0, total_cost: 0, total_tokens: 0, error_count: 0 }

export default function Dashboard() {
  const [traces, setTraces] = useState<TraceWithSummary[]>([])
  const [stats, setStats] = useState<Record<string, number>>(EMPTY)
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')

  useEffect(() => {
    Promise.all([fetchTraces(200), fetchStats().catch(() => EMPTY)])
      .then(([t, s]) => { setTraces(t); setStats(s) })
      .finally(() => setLoading(false))
  }, [])

  const handleDelete = async (id: string) => {
    const ok = await deleteTrace(id)
    if (ok) {
      setTraces(prev => prev.filter(t => t.trace.id !== id))
      fetchStats().then(setStats).catch(() => {})
    }
  }

  const filtered = useMemo(() => {
    if (!search.trim()) return traces
    const q = search.toLowerCase()
    return traces.filter(t => t.trace.name.toLowerCase().includes(q) || t.trace.id.toLowerCase().includes(q))
  }, [traces, search])

  if (loading) return <DashboardSkeleton />

  return (
    <div>
      {/* Stat Tiles */}
      <div className="stat-grid">
        <StatTile label="Traces" value={stats.total_traces.toLocaleString()} accent="indigo" />
        <StatTile label="Total Cost" value={formatCost(stats.total_cost)} sub={`${stats.total_tokens.toLocaleString()} tokens`} accent="cyan" />
        <StatTile label="Spans" value={stats.total_spans.toLocaleString()} accent="emerald" />
        <StatTile label="Errors" value={stats.error_count.toLocaleString()} accent={stats.error_count > 0 ? 'crimson' : 'emerald'} />
      </div>

      {/* Traces Table */}
      <div className="table-wrap">
        <div className="table-wrap-header">
          <span className="table-wrap-title">Traces</span>
          <span className="table-wrap-count">{filtered.length} total</span>
        </div>

        {traces.length > 5 && (
          <div className="action-bar">
            <input
              className="search-input"
              placeholder="Filter traces by name or ID..."
              value={search}
              onChange={e => setSearch(e.target.value)}
            />
          </div>
        )}

        {filtered.length === 0 ? (
          <EmptyState />
        ) : (
          <div style={{ overflowX: 'auto' }}>
            <table>
              <thead>
                <tr>
                  <th>Trace</th>
                  <th style={{ width: 60 }}>Spans</th>
                  <th style={{ width: 90 }}>Cost</th>
                  <th style={{ width: 100 }}>Tokens</th>
                  <th style={{ width: 85 }}>Avg Latency</th>
                  <th style={{ width: 65 }}>Errors</th>
                  <th style={{ width: 90 }}>Time</th>
                  <th style={{ width: 60 }}></th>
                </tr>
              </thead>
              <tbody>
                {filtered.map(({ trace, summary }) => (
                  <tr key={trace.id}>
                    <td>
                      <Link to={`/trace/${trace.id}`} className="trace-name">
                        {trace.name || trace.id.slice(0, 8)}
                      </Link>
                      <span className="trace-name-id">{trace.id.slice(0, 8)}</span>
                    </td>
                    <td className="mono">{summary.total_spans || 0}</td>
                    <td className="cost-cell">{formatCost(summary.total_cost)}</td>
                    <td className="token-cell">{(summary.total_tokens || 0).toLocaleString()}</td>
                    <td className="mono">{formatMs(summary.avg_latency_ms)}</td>
                    <td>
                      {summary.error_count > 0 ? (
                        <span className="error-badge">{summary.error_count} err</span>
                      ) : (
                        <span className="time-cell">–</span>
                      )}
                    </td>
                    <td className="time-cell">{formatTime(trace.created_at)}</td>
                    <td>
                      <button className="btn btn-danger btn-xs" onClick={() => handleDelete(trace.id)}>Del</button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}

function StatTile({ label, value, sub, accent }: { label: string; value: string; sub?: string; accent: string }) {
  return (
    <div className="stat-tile">
      <div className={`stat-tile-accent accent-${accent}`} />
      <div className="stat-tile-label">{label}</div>
      <div className="stat-tile-value">{value}</div>
      {sub && <div className="stat-tile-sub">{sub}</div>}
    </div>
  )
}

function EmptyState() {
  return (
    <div className="empty-state">
      <div className="empty-state-icon">&#9889;</div>
      <div className="empty-state-title">No traces recorded</div>
      <div className="empty-state-desc">
        Start tracing your AI agents by adding the Python SDK to your code.
        Traces will appear here in real time.
      </div>
      <pre className="empty-state-code">{`import agenttrace
agenttrace.init()

# Your OpenAI calls are now traced
from openai import OpenAI
client = OpenAI()
client.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "..."}]
)`}</pre>
    </div>
  )
}
