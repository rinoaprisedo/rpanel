import { ChevronUp, ChevronDown, Search, ChevronLeft, ChevronRight } from 'lucide-react'
import { TableSkeleton } from './Skeleton'
import EmptyState from './EmptyState'
import Input from './Input'

// Builds a compact page list like [1, '…', 4, 5, 6, '…', 12] around the
// current page, always keeping the first/last page visible.
function pageRange(current, total) {
  const delta = 1
  const range = []
  const withDots = []
  let last

  for (let i = 1; i <= total; i++) {
    if (i === 1 || i === total || (i >= current - delta && i <= current + delta)) {
      range.push(i)
    }
  }

  for (const i of range) {
    if (last !== undefined) {
      if (i - last === 2) withDots.push(last + 1)
      else if (i - last > 2) withDots.push('...')
    }
    withDots.push(i)
    last = i
  }

  return withDots
}

// columns: [{ key, label, sortable, render }]
function Table({
  columns,
  data = [],
  loading,
  search,
  onSearchChange,
  sortBy,
  sortDir,
  onSortChange,
  page = 1,
  limit = 10,
  total = 0,
  onPageChange,
  onLimitChange,
  onRowClick,
}) {
  const totalPages = Math.max(1, Math.ceil(total / limit))

  const handleSort = (col) => {
    if (!col.sortable || !onSortChange) return
    if (sortBy === col.key) {
      onSortChange(col.key, sortDir === 'asc' ? 'desc' : 'asc')
    } else {
      onSortChange(col.key, 'asc')
    }
  }

  return (
    <div className="rounded-lg border border-surface-border bg-surface-card shadow-card">
      {onSearchChange && (
        <div className="border-b border-surface-border p-[14px_22px]">
          <div className="relative max-w-xs">
            <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-text-tertiary" />
            <Input
              value={search}
              onChange={(e) => onSearchChange(e.target.value)}
              placeholder="Search..."
              className="h-9 pl-9 text-[13px]"
            />
          </div>
        </div>
      )}

      {loading ? (
        <TableSkeleton cols={columns.length} />
      ) : data.length === 0 ? (
        <EmptyState title="No records found" />
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-left text-[13px]">
            <thead>
              <tr className="border-b border-surface-border text-text-tertiary">
                {columns.map((col) => (
                  <th
                    key={col.key}
                    onClick={() => handleSort(col)}
                    className={`px-5 py-[11px] text-[11px] font-semibold uppercase tracking-[.07em] ${col.sortable ? 'cursor-pointer select-none' : ''}`}
                  >
                    <div className="flex items-center gap-1">
                      {col.label}
                      {col.sortable && sortBy === col.key && (sortDir === 'asc' ? <ChevronUp size={14} /> : <ChevronDown size={14} />)}
                    </div>
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {data.map((row, i) => (
                <tr
                  key={row.uuid || row.id || i}
                  onClick={() => onRowClick?.(row)}
                  className={`border-b border-surface-border transition-colors duration-100 last:border-0 ${onRowClick ? 'cursor-pointer hover:bg-surface-hover' : ''}`}
                >
                  {columns.map((col) => (
                    <td key={col.key} className="px-5 py-[13px] text-text-primary">
                      {col.render ? col.render(row, i) : row[col.key]}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {onPageChange && total > 0 && (
        <div className="flex flex-wrap items-center justify-between gap-3 border-t border-surface-border p-4 text-[13px]">
          <div className="flex items-center gap-2 text-text-secondary">
            <span>Rows per page</span>
            <select
              value={limit}
              onChange={(e) => onLimitChange?.(Number(e.target.value))}
              className="rounded-md border border-surface-border bg-white px-2 py-1"
            >
              {[10, 20, 25, 50, 100].map((n) => (
                <option key={n} value={n}>
                  {n}
                </option>
              ))}
            </select>
            <span>
              Page {page} of {totalPages} ({total} total)
            </span>
          </div>
          <div className="flex items-center gap-1">
            <button
              disabled={page <= 1}
              onClick={() => onPageChange(page - 1)}
              aria-label="Previous page"
              className="flex h-[30px] w-[30px] items-center justify-center rounded-[7px] border border-surface-border text-text-secondary transition-colors duration-100 hover:bg-surface-hover hover:border-surface-border-hover disabled:opacity-40"
            >
              <ChevronLeft size={14} />
            </button>
            {pageRange(page, totalPages).map((p, i) =>
              p === '...' ? (
                <span key={`dots-${i}`} className="flex h-[30px] w-[30px] items-center justify-center text-xs text-text-tertiary">
                  ...
                </span>
              ) : (
                <button
                  key={p}
                  onClick={() => onPageChange(p)}
                  className={`h-[30px] w-[30px] rounded-[7px] border text-xs transition-colors duration-100 ${
                    p === page
                      ? 'border-primary bg-primary text-white'
                      : 'border-surface-border text-text-secondary hover:bg-surface-hover hover:border-surface-border-hover'
                  }`}
                >
                  {p}
                </button>
              ),
            )}
            <button
              disabled={page >= totalPages}
              onClick={() => onPageChange(page + 1)}
              aria-label="Next page"
              className="flex h-[30px] w-[30px] items-center justify-center rounded-[7px] border border-surface-border text-text-secondary transition-colors duration-100 hover:bg-surface-hover hover:border-surface-border-hover disabled:opacity-40"
            >
              <ChevronRight size={14} />
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

export default Table
