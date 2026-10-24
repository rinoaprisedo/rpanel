import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Database, DatabaseBackup, Plus, Search, Table2, Trash2, Eye } from 'lucide-react'
import Button from '../../components/ui/Button'
import Skeleton from '../../components/ui/Skeleton'
import EmptyState from '../../components/ui/EmptyState'
import { cn } from '../../utils/cn'
import { formatBytes } from '../../utils/format'
import * as api from '../../api/databases'
import TableView from './TableView'
import SqlConsole from './SqlConsole'

const errMsg = (e) => e.response?.data?.message || 'Operation failed'

// phpMyAdmin-style explorer: databases on the left, tables / SQL of the selected one on the right.
function Explorer({ sid, dbs, onNew, onBackup, onDrop, backupBusy }) {
  const [sel, setSel] = useState(null)
  const [tab, setTab] = useState('Tables')
  const [table, setTable] = useState(null)
  const [filter, setFilter] = useState('')
  const [tf, setTf] = useState('')

  const list = (dbs.data?.data || [])
  const shown = list.filter((d) => d.name.toLowerCase().includes(filter.toLowerCase()))
  const db = list.find((d) => d.name === sel) ? sel : null

  const tables = useQuery({ queryKey: ['db-tables', sid, db], queryFn: () => api.getTables(sid, db), enabled: !!db && tab === 'Tables' && !table, retry: false })
  const tlist = (tables.data?.data || []).filter((t) => t.name.toLowerCase().includes(tf.toLowerCase()))

  const pick = (name) => { setSel(name); setTable(null); setTab('Tables'); setTf('') }

  return (
    <div className="grid gap-4 lg:grid-cols-[280px_1fr]">
      <aside className="flex flex-col overflow-hidden rounded-xl border border-surface-border bg-surface-card lg:max-h-[calc(100vh-260px)]">
        <div className="flex items-center justify-between border-b border-surface-border px-4 py-3">
          <span className="text-sm font-semibold text-text-primary">Databases <span className="font-normal text-text-tertiary">({list.length})</span></span>
          <Button className="h-8 px-3" onClick={onNew}><Plus size={13} />New</Button>
        </div>
        <div className="relative border-b border-surface-border p-3">
          <Search size={14} className="pointer-events-none absolute left-5 top-1/2 -translate-y-1/2 text-text-tertiary" />
          <input value={filter} onChange={(e) => setFilter(e.target.value)} placeholder="Filter…" className="h-9 w-full rounded-md border-[1.5px] border-surface-border bg-field pl-8 pr-3 text-sm outline-none focus:border-primary" />
        </div>
        <div className="min-h-0 flex-1 space-y-1 overflow-y-auto p-2">
          {dbs.isLoading && <Skeleton className="h-32 w-full" />}
          {dbs.error && <p className="p-3 text-xs text-danger">{errMsg(dbs.error)}</p>}
          {shown.map((d) => (
            <div key={d.name} onClick={() => pick(d.name)}
              className={cn('group flex cursor-pointer items-center gap-2.5 rounded-lg px-3 py-2.5 transition-colors', db === d.name ? 'bg-primary/10' : 'hover:bg-surface-hover')}>
              <Database size={16} className={db === d.name ? 'text-primary' : 'text-text-tertiary'} />
              <div className="min-w-0 flex-1">
                <div className={cn('truncate text-sm font-medium', db === d.name ? 'text-primary' : 'text-text-primary')}>{d.name}</div>
                <div className="text-[11px] text-text-tertiary">{formatBytes(d.size)}</div>
              </div>
              <div className="flex opacity-0 transition-opacity group-hover:opacity-100">
                <button title="Backup" disabled={backupBusy} onClick={(e) => { e.stopPropagation(); onBackup(d.name) }} className="rounded p-1 text-text-secondary hover:bg-surface-card"><DatabaseBackup size={13} /></button>
                <button title="Drop" onClick={(e) => { e.stopPropagation(); onDrop(d.name) }} className="rounded p-1 text-danger hover:bg-surface-card"><Trash2 size={13} /></button>
              </div>
            </div>
          ))}
          {!dbs.isLoading && !shown.length && <p className="p-6 text-center text-xs text-text-secondary">{list.length ? 'No match' : 'No databases yet'}</p>}
        </div>
      </aside>

      <section className="min-w-0 rounded-xl border border-surface-border bg-surface-card p-5">
        {!db ? (
          <EmptyState icon={Database} title="Select a database" message="Browse tables, edit rows and run SQL." />
        ) : table ? (
          <TableView key={`${db}/${table}`} sid={sid} db={db} table={table} onBack={() => setTable(null)} onDropped={() => setTable(null)} />
        ) : (
          <div className="space-y-4">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <h2 className="flex items-center gap-2 text-base font-semibold text-text-primary"><Database size={18} className="text-primary" />{db}</h2>
              <div className="flex gap-1 rounded-lg bg-surface-hover p-1">
                {['Tables', 'SQL'].map((t) => (
                  <button key={t} onClick={() => setTab(t)} className={cn('rounded-md px-4 py-1.5 text-sm font-medium transition-colors', tab === t ? 'bg-surface-card text-primary shadow-sm' : 'text-text-secondary hover:text-text-primary')}>{t}</button>
                ))}
              </div>
            </div>

            {tab === 'SQL' ? <SqlConsole key={db} sid={sid} db={db} /> : (
              <>
                <div className="relative max-w-sm">
                  <Search size={14} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-text-tertiary" />
                  <input value={tf} onChange={(e) => setTf(e.target.value)} placeholder="Filter tables…" className="h-9 w-full rounded-md border-[1.5px] border-surface-border bg-field pl-8 pr-3 text-sm outline-none focus:border-primary" />
                </div>
                {tables.isLoading ? <Skeleton className="h-48 w-full" /> : tables.error ? (
                  <p className="text-sm text-danger">{errMsg(tables.error)}</p>
                ) : (
                  <div className="overflow-x-auto rounded-lg border border-surface-border">
                    <table className="w-full text-sm">
                      <thead className="bg-surface-hover text-left text-xs uppercase text-text-secondary">
                        <tr><th className="p-3">Table</th><th className="p-3 text-right">Rows (approx.)</th><th className="p-3 text-right">Size</th></tr>
                      </thead>
                      <tbody>
                        {tlist.map((t) => (
                          <tr key={t.name} onClick={() => setTable(t.name)} className="cursor-pointer border-t border-surface-border hover:bg-surface-hover">
                            <td className="p-3 font-medium text-text-primary">
                              <span className="flex items-center gap-2">{t.type === 'VIEW' ? <Eye size={14} className="text-text-tertiary" /> : <Table2 size={14} className="text-primary" />}{t.name}</span>
                            </td>
                            <td className="p-3 text-right tabular-nums text-text-secondary">{t.rows.toLocaleString()}</td>
                            <td className="p-3 text-right tabular-nums text-text-secondary">{formatBytes(t.size)}</td>
                          </tr>
                        ))}
                        {!tlist.length && <tr><td colSpan={3} className="p-10 text-center text-text-secondary">{tables.data?.data?.length ? 'No match' : 'This database has no tables. Create some in the SQL tab.'}</td></tr>}
                      </tbody>
                    </table>
                  </div>
                )}
              </>
            )}
          </div>
        )}
      </section>
    </div>
  )
}

export default Explorer
