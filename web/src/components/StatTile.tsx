import type { ReactNode } from 'react'

interface StatTileProps {
  label: string
  value: string
  sub?: string
  accent: string
}

export default function StatTile({ label, value, sub, accent }: StatTileProps) {
  return (
    <div className="stat-tile">
      <div className={`stat-tile-accent accent-${accent}`} />
      <div className="stat-tile-label">{label}</div>
      <div className="stat-tile-value">{value}</div>
      {sub && <div className="stat-tile-sub">{sub}</div>}
    </div>
  )
}
