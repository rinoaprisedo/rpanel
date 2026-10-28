// One stacked horizontal bar with a legend: parts of a whole (device types, memory, human vs bot).
// items: [{ key, label, value, color, text }]
export default function SplitBar({ items, empty = 'No data in this period' }) {
  const total = items.reduce((s, i) => s + i.value, 0)
  if (!total) return <p className="py-6 text-center text-xs text-text-tertiary">{empty}</p>
  return (
    <div>
      <div className="flex h-3 overflow-hidden rounded-full bg-surface-hover" role="img" aria-label={items.map((i) => `${i.label} ${((i.value / total) * 100).toFixed(0)}%`).join(', ')}>
        {items.filter((i) => i.value > 0).map((i) => (
          <div key={i.key} title={`${i.label}: ${i.text ?? i.value} (${((i.value / total) * 100).toFixed(1)}%)`} className="h-full border-r border-surface-card last:border-r-0 transition-[width] duration-300" style={{ width: `${(i.value / total) * 100}%`, backgroundColor: i.color }} />
        ))}
      </div>
      <ul className="mt-3 grid gap-x-4 gap-y-1.5 text-xs sm:grid-cols-2">
        {items.map((i) => (
          <li key={i.key} className="flex items-center justify-between gap-2">
            <span className="inline-flex min-w-0 items-center gap-2 text-text-secondary"><span className="h-2.5 w-2.5 shrink-0 rounded-sm" style={{ backgroundColor: i.color }} /><span className="truncate">{i.label}</span></span>
            <span className="shrink-0 tabular-nums text-text-primary"><b className="font-semibold">{i.text ?? i.value}</b> <span className="text-text-tertiary">{((i.value / total) * 100).toFixed(0)}%</span></span>
          </li>
        ))}
      </ul>
    </div>
  )
}
