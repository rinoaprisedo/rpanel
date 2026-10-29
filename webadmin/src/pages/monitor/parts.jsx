import { cn } from '../../utils/cn'

export function Section({ title, hint, children, right }) {
  return (
    <section className="space-y-3">
      <div className="flex flex-wrap items-end justify-between gap-2">
        <div>
          <h2 className="text-[15px] font-semibold text-text-primary">{title}</h2>
          {hint && <p className="text-xs text-text-tertiary">{hint}</p>}
        </div>
        {right}
      </div>
      {children}
    </section>
  )
}

export function Panel({ title, children, action, className, hint }) {
  return (
    <section className={cn('rounded-lg border border-surface-border bg-surface-card p-4', className)}>
      <header className="mb-3 flex items-start justify-between gap-2">
        <div className="min-w-0">
          <h3 className="text-sm font-semibold text-text-primary">{title}</h3>
          {hint && <p className="mt-0.5 text-xs text-text-tertiary">{hint}</p>}
        </div>
        {action}
      </header>
      {children}
    </section>
  )
}

export const RANGES = [['1h', 'Last hour', '1h'], ['6h', 'Last 6 hours', '6h'], ['24h', 'Last 24 hours', '24h'], ['7d', 'Last 7 days', '7d']]
// Server history is aggregated, so it can look back further than the access logs behind the traffic page.
export const HISTORY_RANGES = [...RANGES, ['30d', 'Last month', '1 month'], ['90d', 'Last 3 months', '3 months'], ['180d', 'Last 6 months', '6 months'], ['365d', 'Last year', '1 year']]
export const rangeLabel = (r) => HISTORY_RANGES.find((x) => x[0] === r)?.[1].toLowerCase()

export function RangeBar({ range, setRange, right, children, ranges = RANGES }) {
  return (
    <div className="sticky top-[60px] z-10 -mx-1 flex flex-wrap items-center gap-3 rounded-lg border border-surface-border bg-surface-card/95 px-3 py-2 backdrop-blur">
      <div className="inline-flex rounded-md border border-surface-border p-0.5" role="group" aria-label="Time range">
        {ranges.map(([k, , short]) => (
          <button key={k} onClick={() => setRange(k)} aria-pressed={range === k} className={cn('h-7 rounded-[8px] px-3 text-xs font-medium transition-colors', range === k ? 'bg-primary text-white' : 'text-text-secondary hover:bg-surface-hover')}>{short}</button>
        ))}
      </div>
      {children}
      <span className="ml-auto text-xs text-text-tertiary">{right}</span>
    </div>
  )
}

export const pctDelta = (cur, prev) => {
  if (!prev) return null
  const d = ((cur - prev) / prev) * 100
  if (Math.abs(d) < 0.5) return { text: '0%', dir: 'flat' }
  return { text: `${d > 0 ? '+' : ''}${Math.abs(d) >= 100 ? Math.round(d) : d.toFixed(0)}%`, dir: d > 0 ? 'up' : 'down' }
}

// Label / value pairs for the "System" card.
export function Facts({ items }) {
  return (
    <dl className="grid gap-x-6 gap-y-3 sm:grid-cols-2 lg:grid-cols-4">
      {items.filter(Boolean).map(([k, v]) => (
        <div key={k} className="min-w-0">
          <dt className="text-[11px] font-medium uppercase tracking-wide text-text-tertiary">{k}</dt>
          <dd className="mt-0.5 truncate text-sm text-text-primary" title={typeof v === 'string' ? v : undefined}>{v}</dd>
        </div>
      ))}
    </dl>
  )
}
