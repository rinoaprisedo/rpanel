import { ArrowUp, ArrowDown, Minus } from 'lucide-react'
import { cn } from '../../utils/cn'
import { loadColor } from './severity'

// Fill follows severity; the unfilled track is a lighter wash of the same color.
export function Meter({ percent, warnAt = 75, critAt = 90 }) {
  const p = Math.max(0, Math.min(100, percent || 0))
  const color = loadColor(percent || 0, warnAt, critAt) // raw value: load can exceed 100%
  return (
    <div className="h-1.5 w-full overflow-hidden rounded-full" style={{ backgroundColor: `color-mix(in srgb, ${color} 18%, transparent)` }} role="meter" aria-valuenow={Math.round(p)} aria-valuemin={0} aria-valuemax={100}>
      <div className="h-full rounded-full transition-[width] duration-500" style={{ width: `${p}%`, backgroundColor: color }} />
    </div>
  )
}

// 2px trend line in the de-emphasis ink with the current point in the accent.
export function Spark({ values, max, height = 28, color = 'var(--viz-1)' }) {
  if (!values || values.length < 2) return <div style={{ height }} />
  const w = 120
  const top = max || Math.max(1, ...values) * 1.1
  const pts = values.map((v, i) => [(i / (values.length - 1)) * w, height - 3 - (Math.min(v, top) / top) * (height - 6)])
  const last = pts[pts.length - 1]
  return (
    <svg viewBox={`0 0 ${w} ${height}`} preserveAspectRatio="none" className="block w-full" style={{ height }} aria-hidden="true">
      <path d={pts.map((p, i) => `${i ? 'L' : 'M'}${p[0].toFixed(1)},${p[1].toFixed(1)}`).join('')} fill="none" stroke="rgb(var(--c-text-3))" strokeWidth="1.5" vectorEffect="non-scaling-stroke" strokeLinejoin="round" />
      <circle cx={last[0] - 2} cy={last[1]} r="2.5" fill={color} />
    </svg>
  )
}

// delta: { text, dir: 'up'|'down'|'flat', good: 'up'|'down'|null } — color = direction × whether that direction is good.
export function Delta({ delta }) {
  if (!delta) return null
  const tone = delta.dir === 'flat' || !delta.good ? 'text-text-tertiary' : delta.dir === delta.good ? 'text-success' : 'text-danger'
  const Icon = delta.dir === 'up' ? ArrowUp : delta.dir === 'down' ? ArrowDown : Minus
  return <span className={cn('inline-flex items-center gap-0.5 text-xs font-medium', tone)}><Icon size={12} strokeWidth={2.4} />{delta.text}</span>
}

export default function StatTile({ label, value, unit, sub, delta, deltaNote, meter, warnAt = 75, critAt = 90, trend, trendMax, className }) {
  return (
    <div className={cn('flex min-w-0 flex-col rounded-lg border border-surface-border bg-surface-card p-4', className)}>
      <p className="text-xs font-medium text-text-secondary">{label}</p>
      <p className="mt-1.5 flex items-baseline gap-1 text-[26px] font-semibold leading-none text-text-primary">
        {value}{unit && <span className="text-sm font-medium text-text-tertiary">{unit}</span>}
      </p>
      <div className="mt-2 min-h-[18px] truncate text-xs text-text-tertiary">
        {delta ? <><Delta delta={delta} />{deltaNote && <span className="ml-1.5">{deltaNote}</span>}</> : sub}
      </div>
      {meter !== undefined && <div className="mt-2.5"><Meter percent={meter} warnAt={warnAt} critAt={critAt} /></div>}
      {trend && <div className="mt-2.5"><Spark values={trend} max={trendMax} color={meter !== undefined ? loadColor(meter, warnAt, critAt) : undefined} /></div>}
      {delta && sub && <p className="mt-1.5 truncate text-xs text-text-tertiary">{sub}</p>}
    </div>
  )
}
