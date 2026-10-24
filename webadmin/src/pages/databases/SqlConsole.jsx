import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { Play, History } from 'lucide-react'
import Button from '../../components/ui/Button'
import * as api from '../../api/databases'
import DataGrid from './DataGrid'

const errMsg = (e) => e.response?.data?.message || 'Query failed'
const histKey = 'rpanel-sql-history'
const loadHist = () => { try { return JSON.parse(localStorage.getItem(histKey)) || [] } catch { return [] } }

// One statement at a time, run as the server admin, so it can modify anything in
// the selected database. Statements are kept in this browser only.
function SqlConsole({ sid, db }) {
  const [sql, setSql] = useState('')
  const [hist, setHist] = useState(loadHist)
  const [showHist, setShowHist] = useState(false)
  const run = useMutation({
    mutationFn: () => api.runQuery(sid, db, sql),
    onSuccess: () => {
      const next = [sql.trim(), ...hist.filter((h) => h !== sql.trim())].slice(0, 20)
      setHist(next)
      localStorage.setItem(histKey, JSON.stringify(next))
    },
  })
  const res = run.data?.data

  return (
    <div className="space-y-3">
      <div className="overflow-hidden rounded-lg border border-surface-border bg-[#14130f]">
        <textarea
          rows={7}
          spellCheck={false}
          autoFocus
          value={sql}
          onChange={(e) => setSql(e.target.value)}
          onKeyDown={(e) => { if ((e.metaKey || e.ctrlKey) && e.key === 'Enter' && sql.trim()) run.mutate() }}
          placeholder={`SELECT * FROM … -- one statement, runs in "${db}"`}
          className="w-full resize-y bg-transparent p-4 font-mono text-[13px] leading-relaxed text-gray-200 outline-none placeholder:text-gray-600"
        />
        <div className="flex items-center justify-between border-t border-white/10 bg-[#1d1b17] px-3 py-2">
          <span className="text-[11px] text-gray-500">⌘/Ctrl + Enter to run</span>
          <div className="flex gap-2">
            <button onClick={() => setShowHist(!showHist)} className="flex items-center gap-1 rounded-md px-2 py-1 text-xs text-gray-400 hover:bg-white/10"><History size={13} />History</button>
            <Button className="h-8" disabled={run.isPending || !sql.trim()} onClick={() => run.mutate()}><Play size={13} />{run.isPending ? 'Running…' : 'Run'}</Button>
          </div>
        </div>
      </div>

      {showHist && (
        <div className="max-h-48 overflow-auto rounded-lg border border-surface-border bg-surface-card">
          {hist.length ? hist.map((h, i) => (
            <button key={i} onClick={() => { setSql(h); setShowHist(false) }} className="block w-full truncate border-b border-surface-border px-3 py-2 text-left font-mono text-xs last:border-0 hover:bg-surface-hover">{h}</button>
          )) : <p className="p-4 text-center text-xs text-text-secondary">No statements yet</p>}
        </div>
      )}

      {run.error && <pre className="whitespace-pre-wrap rounded-lg border border-danger/40 bg-danger-bg p-3 font-mono text-xs text-danger">{errMsg(run.error)}</pre>}
      {res && (
        res.is_query ? (
          <div className="space-y-2">
            <p className="text-xs text-text-secondary">{res.rows.length} row{res.rows.length === 1 ? '' : 's'} · {res.elapsed_ms} ms{res.truncated && ' · output limited to the first 1000 rows'}</p>
            <DataGrid columns={res.columns} rows={res.rows} />
          </div>
        ) : (
          <p className="rounded-lg border border-surface-border bg-surface-card p-4 text-sm text-success">Done — {res.affected} row{res.affected === 1 ? '' : 's'} affected · {res.elapsed_ms} ms</p>
        )
      )}
    </div>
  )
}

export default SqlConsole
