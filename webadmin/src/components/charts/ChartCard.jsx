import { useState } from 'react'
import { Table2, BarChart3 } from 'lucide-react'
import { cn } from '../../utils/cn'

// A titled chart surface with the accessible table twin one click away.
// `table` = { columns: string[], rows: (string|number)[][] }.
export default function ChartCard({ title, subtitle, legend, table, busy, className, children }) {
  const [asTable, setAsTable] = useState(false)
  return (
    <section className={cn('rounded-lg border border-surface-border bg-surface-card p-4', className)}>
      <header className="mb-3 flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="text-sm font-semibold text-text-primary">{title}</h3>
          {subtitle && <p className="mt-0.5 text-xs text-text-tertiary">{subtitle}</p>}
        </div>
        {table && (
          <button
            type="button"
            onClick={() => setAsTable((v) => !v)}
            aria-pressed={asTable}
            title={asTable ? 'Show chart' : 'Show data table'}
            className="shrink-0 rounded-md p-1.5 text-text-tertiary transition-colors hover:bg-surface-hover hover:text-text-primary"
          >
            {asTable ? <BarChart3 size={15} /> : <Table2 size={15} />}
          </button>
        )}
      </header>
      {legend && !asTable && <div className="mb-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">{legend}</div>}
      <div className={cn('transition-opacity duration-150', busy && 'opacity-60')}>
        {asTable && table ? (
          <div className="max-h-64 overflow-auto">
            <table className="w-full text-xs">
              <thead className="sticky top-0 bg-surface-card text-left text-text-tertiary">
                <tr>{table.columns.map((c) => <th key={c} className="pb-1.5 pr-3 font-medium">{c}</th>)}</tr>
              </thead>
              <tbody className="text-text-secondary">
                {table.rows.map((r, i) => (
                  <tr key={i} className="border-t border-surface-border">
                    {r.map((cell, j) => <td key={j} className={cn('py-1 pr-3 tabular-nums', j === 0 && 'text-text-primary')}>{cell}</td>)}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : children}
      </div>
    </section>
  )
}

// Legend entry: a colored mark beside secondary-ink text (text never wears the series color).
export function LegendItem({ color, label, value, shape = 'square' }) {
  return (
    <span className="inline-flex items-center gap-1.5 text-text-secondary">
      <span className={cn('inline-block shrink-0', shape === 'line' ? 'h-0.5 w-3.5 rounded' : 'h-2.5 w-2.5 rounded-[3px]')} style={{ backgroundColor: color }} />
      {label}
      {value !== undefined && <span className="font-medium text-text-primary">{value}</span>}
    </span>
  )
}
