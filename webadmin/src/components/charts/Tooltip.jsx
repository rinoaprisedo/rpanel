// Floating readout used by every chart. Values lead, series names follow; rows are keyed by a
// short line of the series color. Text comes from React children, never innerHTML.
export default function Tooltip({ x, width, title, rows, footer }) {
  const flip = x > width * 0.6
  return (
    <div
      className="pointer-events-none absolute z-20 min-w-[150px] rounded-md border border-surface-border bg-surface-card px-3 py-2 text-xs shadow-md"
      style={{ left: flip ? undefined : x + 14, right: flip ? width - x + 14 : undefined, top: 6 }}
    >
      <p className="mb-1.5 font-medium text-text-secondary">{title}</p>
      <div className="space-y-1">
        {rows.map((r) => (
          <div key={r.label} className="flex items-center justify-between gap-4">
            <span className="flex items-center gap-1.5 text-text-secondary">
              <span className="h-0.5 w-3 rounded" style={{ backgroundColor: r.color }} />{r.label}
            </span>
            <span className="font-semibold tabular-nums text-text-primary">{r.value}</span>
          </div>
        ))}
      </div>
      {footer && <p className="mt-1.5 border-t border-surface-border pt-1.5 text-text-tertiary">{footer}</p>}
    </div>
  )
}
