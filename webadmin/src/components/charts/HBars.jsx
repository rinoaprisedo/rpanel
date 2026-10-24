import { cn } from '../../utils/cn'

// Ranked horizontal bars: label and value stay in text ink, the bar carries the color.
// items: [{ key, label, value, text, sub, color }] (value drives the length, text is what is shown).
export default function HBars({ items, color = 'var(--viz-1)', empty = 'No data in this period' }) {
  const max = Math.max(1, ...items.map((i) => i.value))
  if (!items.length) return <p className="py-8 text-center text-xs text-text-tertiary">{empty}</p>
  return (
    <ul className="space-y-2.5">
      {items.map((it) => (
        <li key={it.key} className="group -mx-1.5 rounded-md px-1.5 py-0.5 transition-colors hover:bg-surface-hover" title={`${it.label}: ${it.text ?? it.value}${it.sub ? ` · ${it.sub}` : ''}`}>
          <div className="mb-1 flex items-baseline justify-between gap-3 text-xs">
            <span className="min-w-0 truncate font-mono text-text-primary">{it.label}</span>
            <span className="shrink-0 tabular-nums text-text-secondary"><b className="font-semibold text-text-primary">{it.text ?? it.value}</b>{it.sub && <span className="ml-1.5 text-text-tertiary">{it.sub}</span>}</span>
          </div>
          <div className="h-2 rounded-full bg-surface-hover group-hover:bg-surface-border">
            <div className={cn('h-2 rounded-full transition-[width] duration-300')} style={{ width: `${Math.max(2, (it.value / max) * 100)}%`, backgroundColor: it.color || color }} />
          </div>
        </li>
      ))}
    </ul>
  )
}
