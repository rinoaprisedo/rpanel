import { useEffect, useMemo, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { Search, RefreshCw, Download, Trash2, Globe, Layers, X } from 'lucide-react'
import Button from '../../components/ui/Button'
import ConfirmDialog from '../../components/ui/ConfirmDialog'
import Skeleton from '../../components/ui/Skeleton'
import usePermission from '../../hooks/usePermission'
import { cn } from '../../utils/cn'
import { parseAccess, statusTone, splitHits } from '../../utils/logLine'
import * as api from '../../api/logs'

const errMsg = (e) => e.response?.data?.message || 'Operation failed'
const limits = [['50', 50], ['100', 100], ['500', 500], ['All', 0]]

function fmtSize(b) {
  if (!b) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB']
  const i = Math.min(u.length - 1, Math.floor(Math.log(b) / Math.log(1024)))
  return `${(b / 1024 ** i).toFixed(i ? 2 : 0)} ${u[i]}`
}

function Hit({ text, kw }) {
  return splitHits(text, kw).map((s, i) => (s.hit ? <mark key={i} className="rounded bg-yellow-400/30 text-yellow-200">{s.text}</mark> : <span key={i}>{s.text}</span>))
}

function Line({ text, kw, type }) {
  const p = type === 'access' ? parseAccess(text) : null
  if (!p) {
    const tone = /\[(error|crit|alert|emerg)\]/.test(text) ? 'text-red-300' : /\[warn\]/.test(text) ? 'text-amber-300' : 'text-slate-300'
    return <div className={cn('whitespace-pre-wrap break-all', tone)}><Hit text={text} kw={kw} /></div>
  }
  return (
    <div className="whitespace-pre-wrap break-all text-slate-300">
      {p.site && <span className="mr-1 rounded bg-sky-500/15 px-1 text-sky-300">{p.site}</span>}
      <span className="text-amber-400"><Hit text={p.ip} kw={kw} /></span> - - [<span className="text-emerald-400">{p.time}</span>]{' '}
      &quot;<span className="text-slate-100"><Hit text={p.request} kw={kw} /></span>&quot;{' '}
      <span className={statusTone(p.status)}>{p.status}</span> <span className="text-amber-400">{p.size}</span>{' '}
      <span className="text-slate-500">&quot;<Hit text={p.referer} kw={kw} />&quot; &quot;<Hit text={p.agent} kw={kw} />&quot;{p.rest}</span>
    </div>
  )
}

function Logs() {
  const qc = useQueryClient()
  const { canDelete } = usePermission()
  const [site, setSite] = useState('all')
  const [siteQ, setSiteQ] = useState('')
  const [type, setType] = useState('access')
  const [lines, setLines] = useState(100)
  const [kwInput, setKwInput] = useState('')
  const [kw, setKw] = useState('')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [auto, setAuto] = useState(false)
  const [confirm, setConfirm] = useState(false)

  useEffect(() => { const t = setTimeout(() => setKw(kwInput), 350); return () => clearTimeout(t) }, [kwInput])

  const sites = useQuery({ queryKey: ['log-sites'], queryFn: api.getLogSites, refetchInterval: 30000 })
  const list = sites.data?.data || []
  const shown = useMemo(() => list.filter((s) => s.domain.includes(siteQ.trim().toLowerCase())), [list, siteQ])
  const current = list.find((s) => s.domain === site)

  const params = { type, lines, q: kw || undefined, from: from || undefined, to: to || undefined }
  const log = useQuery({
    queryKey: ['web-log', site, params], queryFn: () => api.getLog(site, params), enabled: sites.isSuccess && (site === 'all' || !!current),
    refetchInterval: auto ? 5000 : false, placeholderData: (prev) => prev, retry: false,
  })
  const data = log.data?.data

  const clear = useMutation({
    mutationFn: () => api.clearLog(site, type),
    onSuccess: () => { toast.success('Log cleared'); setConfirm(false); qc.invalidateQueries({ queryKey: ['web-log'] }); qc.invalidateQueries({ queryKey: ['log-sites'] }) },
    onError: (e) => { setConfirm(false); toast.error(errMsg(e)) },
  })
  const exportLog = async () => {
    try {
      const blob = await api.downloadLog(site, type)
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url; a.download = `${site}.${type}.log`; a.click()
      URL.revokeObjectURL(url)
    } catch (e) { toast.error(errMsg(e)) }
  }

  const fileSize = site === 'all' ? list.reduce((n, s) => n + (type === 'access' ? s.access_size : s.error_size), 0) : (type === 'access' ? current?.access_size : current?.error_size)
  const filtering = !!(kw || from || to)

  return (
    <div className="grid gap-4 lg:grid-cols-[260px_1fr]">
      <aside className="rounded-xl border border-surface-border bg-surface-card p-3">
        <div className="relative mb-3">
          <Search size={14} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-text-tertiary" />
          <input value={siteQ} onChange={(e) => setSiteQ(e.target.value)} placeholder="Search site…" className="h-[38px] w-full rounded-md border-[1.5px] border-surface-border bg-field pl-8 pr-3 text-sm outline-none focus:border-primary" />
        </div>
        <ul className="max-h-[70vh] space-y-1 overflow-y-auto">
          <li>
            <button onClick={() => setSite('all')} className={cn('flex w-full items-center gap-2 rounded-md px-3 py-2 text-left text-sm transition-colors', site === 'all' ? 'bg-primary/10 font-medium text-primary' : 'text-text-secondary hover:bg-surface-hover')}>
              <Layers size={14} />All websites
            </button>
          </li>
          {sites.isLoading && <li><Skeleton className="h-9 w-full" /></li>}
          {shown.map((s) => (
            <li key={s.domain}>
              <button onClick={() => setSite(s.domain)} className={cn('flex w-full items-center gap-2 rounded-md px-3 py-2 text-left text-sm transition-colors', site === s.domain ? 'bg-primary/10 font-medium text-primary' : 'text-text-secondary hover:bg-surface-hover')}>
                <Globe size={14} className="shrink-0" /><span className="min-w-0 flex-1 truncate">{s.domain}</span>
                <span className="shrink-0 text-[10px] text-text-tertiary">{fmtSize(s.access_size)}</span>
              </button>
            </li>
          ))}
          {!sites.isLoading && shown.length === 0 && <li className="px-3 py-4 text-center text-xs text-text-tertiary">{list.length ? 'No site matches.' : 'No websites yet.'}</li>}
        </ul>
      </aside>

      <section className="min-w-0 space-y-3">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-sm text-text-secondary">{site === 'all' ? 'All websites' : site}</span>
          <span className="rounded-md bg-surface-hover px-2 py-0.5 font-mono text-xs text-text-secondary">{fmtSize(fileSize)}</span>
          {data?.truncated && <span className="text-xs text-text-tertiary">older lines not shown</span>}
          <div className="ml-auto flex flex-wrap items-center gap-2">
            <label className="mr-1 inline-flex cursor-pointer items-center gap-2 text-xs text-text-secondary">
              <input type="checkbox" className="h-3.5 w-3.5 accent-primary" checked={auto} onChange={(e) => setAuto(e.target.checked)} />Auto refresh
            </label>
            <Button variant="secondary" onClick={() => log.refetch()} disabled={log.isFetching}><RefreshCw size={14} className={log.isFetching ? 'animate-spin' : ''} />Refresh</Button>
            <Button variant="secondary" onClick={exportLog} disabled={site === 'all'} title={site === 'all' ? 'Choose a website to export its log' : 'Download the whole file'}><Download size={14} />Export</Button>
            {canDelete('logs') && <Button variant="secondary" className="!text-danger" onClick={() => setConfirm(true)} disabled={site === 'all'} title={site === 'all' ? 'Choose a website to clear its log' : undefined}><Trash2 size={14} />Clear</Button>}
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-3">
          <div className="inline-flex overflow-hidden rounded-md border border-surface-border">
            {[['access', 'Access logs'], ['error', 'Error logs']].map(([k, l]) => (
              <button key={k} onClick={() => setType(k)} aria-pressed={type === k} className={cn('h-[38px] px-4 text-[13px] font-medium transition-colors', type === k ? 'bg-primary text-white' : 'bg-surface-card text-text-secondary hover:bg-surface-hover')}>{l}</button>
            ))}
          </div>
          <div className="flex items-center gap-2 rounded-md border border-surface-border bg-surface-card px-2">
            <input type="date" value={from} max={to || undefined} onChange={(e) => setFrom(e.target.value)} aria-label="From date" className="h-[36px] bg-transparent text-xs text-text-primary outline-none" />
            <span className="text-text-tertiary">→</span>
            <input type="date" value={to} min={from || undefined} onChange={(e) => setTo(e.target.value)} aria-label="To date" className="h-[36px] bg-transparent text-xs text-text-primary outline-none" />
            {(from || to) && <button onClick={() => { setFrom(''); setTo('') }} aria-label="Clear dates" className="text-text-tertiary hover:text-text-primary"><X size={14} /></button>}
          </div>
          <div className="relative min-w-[200px] flex-1">
            <Search size={14} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-text-tertiary" />
            <input value={kwInput} onChange={(e) => setKwInput(e.target.value)} placeholder="Filter by keyword (IP, URL, status, user agent)…" className="h-[38px] w-full rounded-md border-[1.5px] border-surface-border bg-field pl-8 pr-8 text-sm outline-none focus:border-primary" />
            {kwInput && <button onClick={() => setKwInput('')} aria-label="Clear keyword" className="absolute right-2 top-1/2 -translate-y-1/2 text-text-tertiary hover:text-text-primary"><X size={14} /></button>}
          </div>
        </div>

        <div className="rounded-xl border border-ide-border bg-[#0b0f14]">
          {log.isLoading ? <Skeleton className="h-[52vh] w-full" /> : log.isError ? (
            <p className="p-6 text-sm text-danger">{errMsg(log.error)}</p>
          ) : (
            <div className="max-h-[60vh] min-h-[40vh] overflow-auto p-4 font-mono text-xs leading-5">
              {data?.lines?.length ? data.lines.map((l, i) => <Line key={i} text={l} kw={kw} type={type} />)
                : <p className="py-10 text-center text-slate-500">{filtering ? 'No line matches the filter.' : 'This log is empty.'}</p>}
            </div>
          )}
        </div>

        <div className="flex flex-wrap items-center justify-between gap-3 text-xs text-text-tertiary">
          <span>{data ? `${data.lines.length} line${data.lines.length === 1 ? '' : 's'} shown${data.matched > data.lines.length ? ` of ${data.matched}` : ''}` : ''} · newest at the bottom</span>
          <div className="flex items-center gap-2">
            <span>Last</span>
            <div className="inline-flex overflow-hidden rounded-md border border-surface-border">
              {limits.map(([l, v]) => (
                <button key={l} onClick={() => setLines(v)} aria-pressed={lines === v} className={cn('h-8 px-3 text-[13px] font-medium transition-colors', lines === v ? 'bg-primary text-white' : 'bg-surface-card text-text-secondary hover:bg-surface-hover')}>{l}</button>
              ))}
            </div>
          </div>
        </div>
      </section>

      <ConfirmDialog
        open={confirm} onClose={() => setConfirm(false)} onConfirm={() => clear.mutate()} loading={clear.isPending}
        title={`Clear the ${type} log of ${site}?`} message="The log file is emptied permanently. Export it first if you may need it."
        confirmLabel="Clear" loadingLabel="Clearing…"
      />
    </div>
  )
}

export default Logs
