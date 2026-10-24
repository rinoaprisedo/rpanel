import { ArrowDown, ArrowUp } from 'lucide-react'
import { cn } from '../../utils/cn'

export function Cell({ v }) {
  if (v === null || v === undefined) return <span className="rounded bg-surface-hover px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-text-tertiary">null</span>
  if (typeof v === 'boolean') return <span className={v ? 'text-success' : 'text-danger'}>{String(v)}</span>
  const str = String(v)
  return (
    <span title={str.length > 40 ? str : undefined} className={cn('block max-w-[320px] truncate', typeof v === 'number' && 'text-right tabular-nums')}>
      {str === '' ? <span className="text-text-tertiary">(empty)</span> : str}
    </span>
  )
}

// Read-only grid shared by the table browser and the SQL console. `lead` renders
// the first (action) cell of each row.
function DataGrid({ columns, rows, sort, onSort, lead, leadWidth = 'w-20', pk = [] }) {
  return (
    <div className="max-h-[60vh] overflow-auto rounded-lg border border-surface-border">
      <table className="w-full border-collapse text-xs">
        <thead className="sticky top-0 z-10 bg-surface-hover text-left">
          <tr>
            {lead && <th className={cn('border-b border-surface-border p-2', leadWidth)} />}
            {columns.map((c) => {
              const active = sort?.col === c
              return (
                <th key={c} className="whitespace-nowrap border-b border-surface-border p-2 font-semibold text-text-secondary">
                  {onSort ? (
                    <button onClick={() => onSort(c)} className={cn('inline-flex items-center gap-1 hover:text-text-primary', active && 'text-primary')}>
                      {pk.includes(c) && <span title="Primary key">🔑</span>}
                      {c}
                      {active && (sort.dir === 'desc' ? <ArrowDown size={12} /> : <ArrowUp size={12} />)}
                    </button>
                  ) : c}
                </th>
              )
            })}
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={i} className="group border-b border-surface-border last:border-0 hover:bg-surface-hover/60">
              {lead && <td className="p-1.5">{lead(r, i)}</td>}
              {r.map((v, j) => <td key={j} className="whitespace-nowrap p-2 font-mono text-text-primary"><Cell v={v} /></td>)}
            </tr>
          ))}
          {!rows.length && <tr><td colSpan={columns.length + (lead ? 1 : 0)} className="p-10 text-center text-sm text-text-secondary">No rows</td></tr>}
        </tbody>
      </table>
    </div>
  )
}

export default DataGrid
