import { cn } from '../../utils/cn'

// Single-series column chart for a small fixed set of categories (hours of the day, weekdays).
// values: number[]; labels: string[] (same length); peak is highlighted. fmt formats the tooltip value.
export default function Columns({ values, labels, fmt = (v) => v.toLocaleString(), color = 'var(--viz-1)', height = 150, every = 1, ariaLabel }) {
  const max = Math.max(1, ...values)
  const peak = values.indexOf(Math.max(...values))
  const total = values.reduce((a, b) => a + b, 0)
  return (
    <div role="img" aria-label={ariaLabel}>
      <div className="flex items-end gap-[3px]" style={{ height }}>
        {values.map((v, i) => (
          <div key={i} className="group relative flex h-full flex-1 items-end" title={`${labels[i]}: ${fmt(v)}${total ? ` (${((v / total) * 100).toFixed(1)}%)` : ''}`}>
            <div
              className={cn('w-full rounded-t-[3px] transition-[height,opacity] duration-300 group-hover:opacity-100', i === peak && v > 0 ? 'opacity-100' : 'opacity-70')}
              style={{ height: `${v ? Math.max(3, (v / max) * 100) : 0}%`, backgroundColor: color }}
            />
          </div>
        ))}
      </div>
      <div className="mt-1.5 flex gap-[3px]">
        {labels.map((l, i) => <span key={i} className="flex-1 text-center text-[10px] tabular-nums text-text-tertiary">{i % every === 0 ? l : ''}</span>)}
      </div>
    </div>
  )
}
