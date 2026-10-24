import { useState } from 'react'
import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { ChevronLeft, ChevronRight, Pencil, Plus, Search, Trash2, Eraser, Table2 } from 'lucide-react'
import Button from '../../components/ui/Button'
import Skeleton from '../../components/ui/Skeleton'
import ConfirmDialog from '../../components/ui/ConfirmDialog'
import useDebouncedValue from '../../hooks/useDebouncedValue'
import usePermission from '../../hooks/usePermission'
import { cn } from '../../utils/cn'
import * as api from '../../api/databases'
import DataGrid from './DataGrid'
import RowForm from './RowForm'

const errMsg = (e) => e.response?.data?.message || 'Operation failed'
const sizes = [25, 50, 100, 250]

function TableView({ sid, db, table, onBack, onDropped }) {
  const qc = useQueryClient()
  const { canCreate, canEdit, canDelete } = usePermission()
  const [view, setView] = useState('Browse')
  const [page, setPage] = useState(0)
  const [limit, setLimit] = useState(50)
  const [sort, setSort] = useState(null) // { col, dir }
  const [search, setSearch] = useState('')
  const q = useDebouncedValue(search)
  const [form, setForm] = useState(null) // { row? }
  const [confirm, setConfirm] = useState(null)

  const key = ['db-table', sid, db, table]
  const { data, isLoading, error, isFetching } = useQuery({
    queryKey: [...key, page, limit, sort, q],
    queryFn: () => api.getTable(sid, db, table, { limit, offset: page * limit, order: sort?.col, dir: sort?.dir, q }),
    placeholderData: keepPreviousData,
    retry: false,
  })
  const d = data?.data
  const cols = d?.columns || []
  const pkCols = cols.filter((c) => c.primary).map((c) => c.name)
  const total = d?.total || 0
  const pages = Math.max(1, Math.ceil(total / limit))

  const done = (msg, after) => () => { toast.success(msg); qc.invalidateQueries({ queryKey: key }); qc.invalidateQueries({ queryKey: ['db-tables', sid, db] }); after?.() }
  const fail = (e) => toast.error(errMsg(e))
  const save = useMutation({
    mutationFn: ({ values, row }) => (row ? api.updateRow(sid, db, table, pkOf(row), values) : api.insertRow(sid, db, table, values)),
    onSuccess: done('Saved', () => setForm(null)), onError: fail,
  })
  const remove = useMutation({ mutationFn: (row) => api.deleteRow(sid, db, table, pkOf(row)), onSuccess: done('Row deleted', () => setConfirm(null)), onError: fail })
  const truncate = useMutation({ mutationFn: () => api.truncateTable(sid, db, table), onSuccess: done('Table emptied', () => setConfirm(null)), onError: fail })
  const drop = useMutation({ mutationFn: () => api.dropTable(sid, db, table), onSuccess: done('Table dropped', () => { setConfirm(null); onDropped() }), onError: fail })

  const pkOf = (row) => Object.fromEntries(cols.map((c, i) => [c.name, row[i]]).filter(([n]) => pkCols.includes(n)).map(([n, v]) => [n, String(v)]))
  const toggleSort = (col) => {
    setPage(0)
    setSort((s) => (s?.col !== col ? { col, dir: 'asc' } : s.dir === 'asc' ? { col, dir: 'desc' } : null))
  }
  const editable = pkCols.length > 0

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2 text-sm">
          <button onClick={onBack} className="text-text-secondary hover:text-primary">{db}</button>
          <span className="text-text-tertiary">/</span>
          <span className="flex items-center gap-1.5 font-semibold text-text-primary"><Table2 size={15} className="text-primary" />{table}</span>
          {d && <span className="rounded-full bg-surface-hover px-2 py-0.5 text-[11px] text-text-secondary">{total.toLocaleString()} rows</span>}
        </div>
        <div className="flex gap-2">
          {canCreate('databases') && <Button onClick={() => setForm({})}><Plus size={14} />Insert row</Button>}
          {canDelete('databases') && (
            <>
              <Button variant="secondary" onClick={() => setConfirm({ title: `Empty ${table}`, message: `Remove ALL rows from "${table}"? This cannot be undone.`, label: 'Empty table', run: () => truncate.mutate() })}><Eraser size={14} />Empty</Button>
              <Button variant="secondary" onClick={() => setConfirm({ title: `Drop ${table}`, message: `Permanently delete "${table}" including its structure?`, label: 'Drop', run: () => drop.mutate() })}><Trash2 size={14} className="text-danger" />Drop</Button>
            </>
          )}
        </div>
      </div>

      <div className="flex gap-1 border-b border-surface-border">
        {['Browse', 'Structure'].map((t) => (
          <button key={t} onClick={() => setView(t)} className={cn('px-4 py-2 text-sm font-medium', view === t ? 'border-b-2 border-primary text-primary' : 'text-text-secondary hover:text-text-primary')}>{t}</button>
        ))}
      </div>

      {isLoading ? <Skeleton className="h-64 w-full" /> : error ? (
        <p className="rounded-lg border border-surface-border bg-surface-card p-6 text-sm text-danger">{errMsg(error)}</p>
      ) : view === 'Structure' ? (
        <div className="overflow-x-auto rounded-lg border border-surface-border bg-surface-card">
          <table className="w-full text-sm">
            <thead className="border-b border-surface-border text-left text-xs uppercase text-text-secondary">
              <tr><th className="p-3">Column</th><th className="p-3">Type</th><th className="p-3">Null</th><th className="p-3">Default</th><th className="p-3">Extra</th></tr>
            </thead>
            <tbody>
              {cols.map((c) => (
                <tr key={c.name} className="border-b border-surface-border last:border-0">
                  <td className="p-3 font-medium">{c.primary && '🔑 '}{c.name}</td>
                  <td className="p-3 font-mono text-xs text-text-secondary">{c.type}</td>
                  <td className="p-3">{c.nullable ? 'YES' : 'NO'}</td>
                  <td className="p-3 font-mono text-xs">{c.default ?? <span className="text-text-tertiary">—</span>}</td>
                  <td className="p-3 text-xs text-text-secondary">{c.extra}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <>
          <div className="relative max-w-sm">
            <Search size={14} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-text-tertiary" />
            <input value={search} onChange={(e) => { setSearch(e.target.value); setPage(0) }} placeholder="Search in all columns…"
              className="h-9 w-full rounded-md border-[1.5px] border-surface-border bg-field pl-8 pr-3 text-sm outline-none focus:border-primary" />
          </div>
          <div className={cn(isFetching && 'opacity-70 transition-opacity')}>
            <DataGrid
              columns={d.columns?.length ? cols.map((c) => c.name) : []}
              pk={pkCols}
              rows={d.rows || []}
              sort={sort}
              onSort={toggleSort}
              lead={(canEdit('databases') || canDelete('databases')) && editable ? (row) => (
                <div className="flex gap-0.5 opacity-60 group-hover:opacity-100">
                  {canEdit('databases') && <button title="Edit" onClick={() => setForm({ row })} className="rounded p-1 hover:bg-surface-hover"><Pencil size={13} /></button>}
                  {canDelete('databases') && <button title="Delete" onClick={() => setConfirm({ title: 'Delete row', message: `Delete the row where ${pkCols.map((c) => `${c} = ${row[cols.findIndex((x) => x.name === c)]}`).join(', ')}?`, label: 'Delete', run: () => remove.mutate(row) })} className="rounded p-1 text-danger hover:bg-surface-hover"><Trash2 size={13} /></button>}
                </div>
              ) : undefined}
            />
          </div>
          {!editable && <p className="text-xs text-text-secondary">This table has no primary key, so rows cannot be edited here. Use the SQL tab instead.</p>}
          <div className="flex flex-wrap items-center justify-between gap-3 text-xs text-text-secondary">
            <div className="flex items-center gap-2">
              Rows per page
              <select value={limit} onChange={(e) => { setLimit(Number(e.target.value)); setPage(0) }} className="h-8 rounded-md border border-surface-border bg-field px-2">
                {sizes.map((n) => <option key={n}>{n}</option>)}
              </select>
            </div>
            <div className="flex items-center gap-2">
              <span>{total ? `${page * limit + 1}–${Math.min(total, (page + 1) * limit)} of ${total.toLocaleString()}` : '0 rows'}</span>
              <Button variant="secondary" className="h-8 px-2" disabled={page === 0} onClick={() => setPage(page - 1)}><ChevronLeft size={14} /></Button>
              <Button variant="secondary" className="h-8 px-2" disabled={page + 1 >= pages} onClick={() => setPage(page + 1)}><ChevronRight size={14} /></Button>
            </div>
          </div>
        </>
      )}

      {form && <RowForm columns={cols} row={form.row} busy={save.isPending} onClose={() => setForm(null)} onSubmit={(values) => save.mutate({ values, row: form.row })} />}
      <ConfirmDialog open={!!confirm} onClose={() => setConfirm(null)} title={confirm?.title} message={confirm?.message} confirmLabel={confirm?.label}
        loading={remove.isPending || truncate.isPending || drop.isPending} loadingLabel="Working…" onConfirm={() => confirm.run()} />
    </div>
  )
}

export default TableView
