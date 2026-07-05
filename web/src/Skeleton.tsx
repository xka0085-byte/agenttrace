export function DashboardSkeleton() {
  return (
    <div>
      <div className="stat-grid">
        {[1, 2, 3, 4].map(i => (
          <div key={i} className={`skeleton-box${i === 1 ? ' accent-left accent-indigo' : i === 2 ? ' accent-left accent-cyan' : i === 3 ? ' accent-left accent-emerald' : ' accent-left accent-emerald'}`} style={{ height: 72 }} />
        ))}
      </div>
      <div className="skeleton-box" style={{ height: 40, marginBottom: 14, borderRadius: 'var(--radius-lg)' }} />
      <div className="skeleton-box" style={{ height: 200, borderRadius: 'var(--radius-lg)' }} />
    </div>
  )
}

export function TraceViewSkeleton() {
  return (
    <div>
      <div className="skeleton-box" style={{ height: 24, width: 200, marginBottom: 20 }} />
      <div className="skeleton-box" style={{ height: 32, width: 320, marginBottom: 20 }} />
      <div className="stat-grid">
        {[1, 2, 3, 4].map(i => (
          <div key={i} className="skeleton-box accent-left accent-indigo" style={{ height: 72 }} />
        ))}
      </div>
      <div className="panel-grid">
        <div className="skeleton-box" style={{ height: 320, borderRadius: 'var(--radius-lg)' }} />
        <div className="skeleton-box" style={{ height: 320, borderRadius: 'var(--radius-lg)' }} />
      </div>
      <div className="detail-grid" style={{ marginTop: 14 }}>
        <div className="skeleton-box" style={{ height: 240, borderRadius: 'var(--radius)' }} />
        <div className="skeleton-box" style={{ height: 240, borderRadius: 'var(--radius)' }} />
      </div>
    </div>
  )
}

export function TableSkeleton({ rows = 5 }: { rows?: number }) {
  return (
    <div style={{ overflow: 'hidden', borderRadius: 'var(--radius-lg)', border: '1px solid var(--nebula)' }}>
      <div className="skeleton-box" style={{ height: 40, borderBottom: '1px solid var(--nebula)' }} />
      {Array.from({ length: rows }).map((_, i) => (
        <div
          key={i}
          className="skeleton-box"
          style={{
            height: 40,
            borderBottom: i < rows - 1 ? '1px solid var(--nebula-dim)' : 'none',
          }}
        />
      ))}
    </div>
  )
}
