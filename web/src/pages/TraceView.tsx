import { useEffect, useState, useMemo, useCallback } from 'react'
import { useParams, Link } from 'react-router-dom'
import { fetchTrace, formatCost, formatMs, formatTime, kindColor } from '../api'
import { TraceViewSkeleton } from '../Skeleton'
import type { Span, Trace } from '../api'
import StatTile from '../components/StatTile'

export default function TraceView() {
  const { id } = useParams<{ id: string }>()
  const [trace, setTrace] = useState<Trace | null>(null)
  const [spans, setSpans] = useState<Span[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [selectedSpan, setSelectedSpan] = useState<Span | null>(null)

  useEffect(() => {
    if (!id) return
    fetchTrace(id)
      .then(({ trace, spans }) => { setTrace(trace); setSpans(spans); if (spans.length > 0) setSelectedSpan(spans[0]) })
      .catch(e => setError(e.message))
      .finally(() => setLoading(false))
  }, [id])

  // Build span tree
  const { rootSpans, childrenOf } = useMemo(() => {
    const childrenOf = (parentId: string) => spans.filter(sp => sp.parent_span_id === parentId)
    const rootSpans = spans.filter(sp => !sp.parent_span_id)
    return { rootSpans, childrenOf }
  }, [spans])

  // Aggregate metrics
  const totalCost = spans.reduce((s, sp) => s + (sp.cost || 0), 0)
  const totalTokens = spans.reduce((s, sp) => s + (sp.total_tokens || 0), 0)
  const errorCount = spans.filter(sp => sp.status === 'error').length

  // Timeline range
  const { minTime, totalMs } = useMemo(() => {
    const times = spans.map(sp => new Date(sp.started_at).getTime()).filter(Boolean)
    const maxTimes = spans.map(sp => sp.ended_at ? new Date(sp.ended_at).getTime() : new Date(sp.started_at).getTime() + 1)
    const min = times.length > 0 ? Math.min(...times) : Date.now()
    const max = maxTimes.length > 0 ? Math.max(...maxTimes) : Date.now() + 1
    return { minTime: min, totalMs: Math.max(max - min, 1) }
  }, [spans])

  if (loading) return <TraceViewSkeleton />
  if (error) return <div className="empty-state"><div className="empty-state-icon">&#9888;</div><div className="empty-state-title">Failed to load</div><div className="empty-state-desc">{error}</div></div>
  if (!trace) return <div className="empty-state"><div className="empty-state-icon">&#128269;</div><div className="empty-state-title">Trace not found</div></div>

  return (
    <div>
      {/* Breadcrumb */}
      <div className="breadcrumb">
        <Link to="/">Traces</Link>
        <span className="breadcrumb-sep">/</span>
        <span className="breadcrumb-current">{trace.name}</span>
        <span style={{ marginLeft: 12, fontFamily: 'var(--font-mono)', fontSize: 11 }}>{trace.id.slice(0, 8)}</span>
      </div>

      {/* Header */}
      <div className="trace-header">
        <div className="trace-meta">
          <h1 className="trace-meta-name">{trace.name}</h1>
        </div>
        <div className="live-indicator">
          <span className="live-dot" />
          local trace
        </div>
      </div>

      {/* Stat tiles */}
      <div className="stat-grid">
        <StatTile label="Total Cost" value={formatCost(totalCost)} accent="indigo" />
        <StatTile label="Tokens" value={totalTokens.toLocaleString()} accent="cyan" />
        <StatTile label="Spans" value={spans.length.toString()} accent="emerald" />
        <StatTile label="Errors" value={errorCount.toString()} accent={errorCount > 0 ? 'crimson' : 'emerald'} />
      </div>

      {/* Split panels: Tree + Timeline */}
      <div className="panel-grid">
        {/* Span Tree */}
        <div className="panel">
          <div className="panel-header">
            <span className="panel-title">Span Tree</span>
            <span style={{ fontSize: 11, color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>{spans.length} nodes</span>
          </div>
          <div className="panel-body">
            {rootSpans.length === 0 ? (
              <div style={{ padding: 20, textAlign: 'center', color: 'var(--text-muted)', fontSize: 13 }}>No spans</div>
            ) : (
              rootSpans.map(sp => (
                <SpanNode
                  key={sp.id}
                  span={sp}
                  getChildren={childrenOf}
                  depth={0}
                  selectedId={selectedSpan?.id || ''}
                  onSelect={setSelectedSpan}
                />
              ))
            )}
          </div>
        </div>

        {/* Timeline */}
        <div className="panel">
          <div className="panel-header">
            <span className="panel-title">Timeline</span>
            <span style={{ fontSize: 11, color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
              {formatTime(new Date(minTime).toISOString())} &rarr; {formatTime(new Date(minTime + totalMs).toISOString())}
            </span>
          </div>
          <div className="panel-body">
            {spans.map(sp => {
              const startPct = ((new Date(sp.started_at).getTime() - minTime) / totalMs) * 100
              const endMs = sp.ended_at ? new Date(sp.ended_at).getTime() : new Date(sp.started_at).getTime() + 1
              const widthPct = Math.max(((endMs - new Date(sp.started_at).getTime()) / totalMs) * 100, 0.5)
              const isSelected = selectedSpan?.id === sp.id
              return (
                <div
                  key={sp.id}
                  className="timeline-bar-row"
                  onClick={() => setSelectedSpan(sp)}
                  style={{ cursor: 'pointer', background: isSelected ? 'var(--indigo-bg)' : 'transparent', borderRadius: 4, margin: '0 4px' }}
                >
                  <span className="timeline-label" title={sp.name}>{sp.name}</span>
                  <div className="timeline-bar-bg">
                    <div
                      className="timeline-bar-fill"
                      style={{
                        left: `${startPct}%`,
                        width: `${widthPct}%`,
                        background: sp.status === 'error' ? 'var(--crimson)' : kindColor(sp.kind),
                        opacity: sp.status === 'error' ? 0.6 : 0.85,
                      }}
                    >
                      {widthPct > 6 && (
                        <span style={{ fontSize: 9, fontWeight: 700, color: '#fff', textShadow: '0 1px 2px rgba(0,0,0,0.5)' }}>
                          {sp.kind}
                        </span>
                      )}
                    </div>
                  </div>
                </div>
              )
            })}
            <div className="timeline-axis">
              <span>{formatTime(new Date(minTime).toISOString())}</span>
              <span>{formatTime(new Date(minTime + totalMs).toISOString())}</span>
            </div>
          </div>
        </div>
      </div>

      {/* Detail Panel */}
      <div className="detail-grid">
        {/* Span list */}
        <div className="detail-spans">
          <div style={{ padding: '10px 14px', borderBottom: '1px solid var(--nebula)', fontFamily: 'var(--font-display)', fontSize: 12, fontWeight: 600, color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: '0.04em' }}>
            All Spans
          </div>
          {spans.map(sp => (
            <div
              key={sp.id}
              className={`detail-spans-item${selectedSpan?.id === sp.id ? ' selected' : ''}`}
              onClick={() => setSelectedSpan(sp)}
            >
              <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', flex: 1 }}>
                {sp.name}
              </span>
              <span style={{
                fontSize: 9, fontWeight: 700, color: kindColor(sp.kind),
                textTransform: 'uppercase', letterSpacing: '0.04em',
                padding: '1px 5px', borderRadius: 3, marginLeft: 8
              }}>
                {sp.kind}
              </span>
            </div>
          ))}
        </div>

        {/* Detail card */}
        <div className="detail-info">
          {selectedSpan ? (
            <div>
              <div className="detail-info-title">{selectedSpan.name}</div>

              <div className="detail-grid-inner">
                <DetailField label="Kind" value={selectedSpan.kind} />
                <DetailField label="Status" value={selectedSpan.status === 'error'
                  ? <span style={{ color: 'var(--crimson)', fontWeight: 600 }}>Error: {selectedSpan.error_message}</span>
                  : <span style={{ color: 'var(--emerald)' }}>OK</span>
                } />
                <DetailField label="Model" value={selectedSpan.model || '–'} />
                <DetailField label="Provider" value={selectedSpan.provider || '–'} />
                <DetailField label="Tokens" value={`${(selectedSpan.total_tokens || 0).toLocaleString()} (${selectedSpan.prompt_tokens || 0} in / ${selectedSpan.completion_tokens || 0} out)`} />
                <DetailField label="Cost" value={formatCost(selectedSpan.cost)} />
                <DetailField label="Duration" value={selectedSpan.ended_at
                  ? formatMs(new Date(selectedSpan.ended_at).getTime() - new Date(selectedSpan.started_at).getTime())
                  : 'Running...'
                } />
                <DetailField label="Span ID" value={selectedSpan.id.slice(0, 16)} />
              </div>

              {selectedSpan.input_json && (
                <div className="detail-io">
                  <button className="detail-io-toggle" onClick={e => {
                    const target = (e.currentTarget.nextElementSibling as HTMLElement)
                    if (target) target.style.display = target.style.display === 'none' ? 'block' : 'none'
                  }}>
                    <span style={{ fontSize: 11, transition: 'transform 0.2s' }}>&#9654;</span>
                    Input
                  </button>
                  <pre className="detail-io-content" style={{ display: 'none' }}>{tryPretty(selectedSpan.input_json)}</pre>
                </div>
              )}
              {selectedSpan.output_json && (
                <div className="detail-io">
                  <button className="detail-io-toggle" onClick={e => {
                    const target = (e.currentTarget.nextElementSibling as HTMLElement)
                    if (target) target.style.display = target.style.display === 'none' ? 'block' : 'none'
                  }}>
                    <span style={{ fontSize: 11, transition: 'transform 0.2s' }}>&#9654;</span>
                    Output
                  </button>
                  <pre className="detail-io-content" style={{ display: 'none' }}>{tryPretty(selectedSpan.output_json)}</pre>
                </div>
              )}
            </div>
          ) : (
            <div style={{ textAlign: 'center', color: 'var(--text-muted)', padding: '40px 0', fontSize: 13 }}>
              Select a span to inspect
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

// ── Span Tree Node ──

function SpanNode({ span, getChildren, depth, selectedId, onSelect }: {
  span: Span; getChildren: (id: string) => Span[]; depth: number; selectedId: string; onSelect: (s: Span) => void
}) {
  const children = getChildren(span.id)
  const [expanded, setExpanded] = useState(depth < 2)
  const hasChildren = children.length > 0
  const isSelected = selectedId === span.id
  const durMs = span.ended_at ? new Date(span.ended_at).getTime() - new Date(span.started_at).getTime() : 0

  return (
    <>
      <div
        className={`span-row${isSelected ? ' selected' : ''}${span.status === 'error' ? ' error' : ''}`}
        style={{ paddingLeft: 12 + depth * 18 }}
        onClick={() => onSelect(span)}
      >
        <span
          className={`span-row-toggle${expanded ? ' expanded' : ''}`}
          onClick={e => { e.stopPropagation(); hasChildren && setExpanded(!expanded) }}
        >
          {hasChildren ? '▶' : ''}
        </span>
        <span className="span-row-indicator" style={{ background: span.status === 'error' ? 'var(--crimson)' : kindColor(span.kind) }} />
        <span className="span-row-name">{span.name}</span>
        <span className="span-row-kind" style={{ color: kindColor(span.kind), background: `${kindColor(span.kind)}18` }}>
          {span.kind}
        </span>
        <span className="span-row-metrics">
          <span className="span-row-metric-cost">{span.cost > 0 ? formatCost(span.cost) : ''}</span>
          <span>{durMs > 0 ? formatMs(durMs) : ''}</span>
          {span.status === 'error' && <span style={{ color: 'var(--crimson)', fontWeight: 700 }}>✕</span>}
        </span>
      </div>
      {hasChildren && expanded && children.map(ch => (
        <SpanNode key={ch.id} span={ch} getChildren={getChildren} depth={depth + 1} selectedId={selectedId} onSelect={onSelect} />
      ))}
    </>
  )
}

// ── Helpers ──

function DetailField({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="detail-field">
      <div className="detail-field-label">{label}</div>
      <div className="detail-field-value">{value}</div>
    </div>
  )
}

const tryPretty = (json: string): string => {
  try { return JSON.stringify(JSON.parse(json), null, 2) } catch { return json }
}
