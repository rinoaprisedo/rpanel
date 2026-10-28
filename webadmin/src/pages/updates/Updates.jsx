import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import {
  ShieldCheck, ShieldAlert, RefreshCw, Download, Power, Search, Check, LoaderCircle, X, Circle, TriangleAlert, ArrowRight, Clock,
} from 'lucide-react'
import Button from '../../components/ui/Button'
import Input from '../../components/ui/Input'
import Badge from '../../components/ui/Badge'
import Modal from '../../components/ui/Modal'
import Switch from '../../components/ui/Switch'
import ConfirmDialog from '../../components/ui/ConfirmDialog'
import Skeleton from '../../components/ui/Skeleton'
import usePermission from '../../hooks/usePermission'
import { cn } from '../../utils/cn'
import * as api from '../../api/updates'
import { getVersion } from '../../api/system'

const errMsg = (e) => e.response?.data?.message || 'Operation failed'
const kindTitle = { check: 'Checking for updates', security: 'Installing security updates', all: 'Installing all updates' }

function StepIcon({ status }) {
  if (status === 'done') return <span className="flex h-6 w-6 items-center justify-center rounded-full bg-success text-white"><Check size={14} strokeWidth={3} /></span>
  if (status === 'failed') return <span className="flex h-6 w-6 items-center justify-center rounded-full bg-danger text-white"><X size={14} strokeWidth={3} /></span>
  if (status === 'running') return <span className="flex h-6 w-6 items-center justify-center rounded-full bg-primary/15 text-primary"><LoaderCircle size={15} className="animate-spin" /></span>
  return <span className="flex h-6 w-6 items-center justify-center text-text-tertiary"><Circle size={14} /></span>
}

// Live progress of an update job; stays open until the job ends.
function JobDialog({ open, onClose, onFinished }) {
  const { data } = useQuery({
    queryKey: ['updates', 'progress'], queryFn: api.getUpdateProgress, enabled: open, retry: 3, gcTime: 0,
    refetchInterval: (q) => (q.state.data?.data?.state === 'running' || !q.state.data ? 1000 : false),
  })
  const p = data?.data
  const running = !p || p.state === 'running'
  const logRef = useRef(null)
  const notified = useRef(false)
  useEffect(() => { if (logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight }, [p?.log?.length])
  useEffect(() => {
    if (!open) { notified.current = false; return }
    if (p && p.state !== 'running' && !notified.current) { notified.current = true; onFinished?.(p) }
  }, [p, open, onFinished])

  const failed = p?.state === 'failed'
  const done = p?.state === 'done'
  const percent = p?.percent ?? 0

  return (
    <Modal open={open} onClose={running ? () => {} : onClose} title={kindTitle[p?.kind] || 'System updates'} size="lg">
      <div className="space-y-5">
        <div>
          <div className="mb-1.5 flex items-center justify-between text-xs">
            <span className={cn('font-medium', failed ? 'text-danger' : done ? 'text-success' : 'text-text-secondary')}>
              {failed ? 'Failed' : done ? 'Finished' : p?.total ? `Installed ${p.installed} of ${p.total} packages…` : 'Working…'}
            </span>
            <span className="font-mono text-text-tertiary">{percent}%{p ? ` · ${p.elapsed_seconds}s` : ''}</span>
          </div>
          <div className="h-2.5 overflow-hidden rounded-full bg-surface-hover" role="progressbar" aria-valuenow={percent} aria-valuemin={0} aria-valuemax={100}>
            <div className={cn('h-full rounded-full transition-[width] duration-500', failed ? 'bg-danger' : done ? 'bg-success' : 'bg-primary', running && 'animate-pulse')} style={{ width: `${Math.max(percent, 2)}%` }} />
          </div>
        </div>
        <ol className="space-y-2.5">
          {(p?.steps || []).map((s) => (
            <li key={s.key} className="flex items-start gap-3">
              <StepIcon status={s.status} />
              <div className="min-w-0 pt-0.5">
                <p className={cn('text-sm', s.status === 'pending' ? 'text-text-tertiary' : 'font-medium text-text-primary')}>{s.label}</p>
                {s.detail && <p className={cn('text-xs', s.status === 'failed' ? 'text-danger' : 'text-text-tertiary')}>{s.detail}</p>}
              </div>
            </li>
          ))}
        </ol>
        {failed && (
          <div className="rounded-lg border border-danger/30 bg-danger-bg p-3 text-sm text-danger">
            <p className="mb-1 font-medium">The job stopped with an error.</p>
            <pre className="whitespace-pre-wrap break-words font-mono text-xs">{p.error}</pre>
          </div>
        )}
        <details open={failed}>
          <summary className="cursor-pointer text-xs font-medium text-text-secondary hover:text-text-primary">Output log</summary>
          <pre ref={logRef} className="mt-2 max-h-56 overflow-auto rounded-md bg-[#0b0f14] p-3 font-mono text-[11px] leading-5 text-slate-300">
            {(p?.log || []).length ? p.log.map((l, i) => <div key={i} className={/^(E:|W:)|error|failed/i.test(l) ? 'text-warning' : undefined}>{l}</div>) : 'Waiting…'}
          </pre>
        </details>
        <p className="text-xs text-text-tertiary">You can close this page; the update keeps running on the server.</p>
        <div className="flex justify-end">
          <Button onClick={onClose} disabled={running}>{running ? 'Working…' : 'Close'}</Button>
        </div>
      </div>
    </Modal>
  )
}

// After a reboot request: wait until the panel goes away and comes back, then reload.
function RebootDialog({ open }) {
  const [phase, setPhase] = useState('down') // down -> waiting for it to stop; up -> waiting for it to answer
  const [secs, setSecs] = useState(0)
  useEffect(() => {
    if (!open) return undefined
    let stopped = false
    let wentDown = false
    const t0 = Date.now()
    const tick = async () => {
      if (stopped) return
      setSecs(Math.floor((Date.now() - t0) / 1000))
      try {
        await getVersion()
        if (wentDown) { window.location.reload(); return }
      } catch {
        wentDown = true
        setPhase('up')
      }
      setTimeout(tick, 3000)
    }
    const first = setTimeout(tick, 4000)
    return () => { stopped = true; clearTimeout(first) }
  }, [open])
  return (
    <Modal open={open} onClose={() => {}} title="Restarting the server" size="sm">
      <div className="flex flex-col items-center gap-3 py-4 text-center">
        <LoaderCircle size={34} className="animate-spin text-primary" />
        <p className="text-sm text-text-primary">{phase === 'down' ? 'Shutting down…' : 'Waiting for the server to come back…'}</p>
        <p className="text-xs text-text-tertiary">{secs}s · this page reloads automatically (usually 1–2 minutes).</p>
        {secs > 300 && <p className="text-xs text-warning">It is taking longer than usual. Check the server console of your hosting provider.</p>}
      </div>
    </Modal>
  )
}

function fmtTime(ts) {
  if (!ts) return 'never'
  const m = Math.round((Date.now() / 1000 - ts) / 60)
  if (m < 1) return 'just now'
  if (m < 60) return `${m} min ago`
  if (m < 1440) return `${Math.round(m / 60)} h ago`
  return new Date(ts * 1000).toLocaleDateString()
}

function Updates() {
  const qc = useQueryClient()
  const { canEdit } = usePermission()
  const editable = canEdit('updates')
  const { data, isLoading } = useQuery({ queryKey: ['updates'], queryFn: api.getUpdates, refetchInterval: 30000 })
  const st = data?.data
  const [jobOpen, setJobOpen] = useState(false)
  const [confirm, setConfirm] = useState(null) // 'all' | 'security'
  const [rebootAsk, setRebootAsk] = useState(false)
  const [rebooting, setRebooting] = useState(false)
  const [filter, setFilter] = useState('all')
  const [q, setQ] = useState('')

  // A job started earlier (or from another tab) reopens its dialog.
  useEffect(() => { if (st?.job?.state === 'running') setJobOpen(true) }, [st?.job?.state])

  const refresh = () => qc.invalidateQueries({ queryKey: ['updates'] })
  const run = useMutation({
    mutationFn: api.runUpdates,
    onSuccess: () => { setConfirm(null); setJobOpen(true); qc.invalidateQueries({ queryKey: ['updates', 'progress'] }) },
    onError: (e) => { setConfirm(null); toast.error(errMsg(e)) },
  })
  const auto = useMutation({
    mutationFn: api.setAutoSecurity,
    onSuccess: (_, v) => { toast.success(v ? 'Automatic security updates enabled' : 'Automatic security updates disabled'); refresh() },
    onError: (e) => toast.error(errMsg(e), { duration: 8000 }),
  })
  const boot = useMutation({
    mutationFn: api.reboot,
    onSuccess: () => { setRebootAsk(false); setRebooting(true) },
    onError: (e) => { setRebootAsk(false); toast.error(errMsg(e)) },
  })

  const pkgs = useMemo(() => (st?.packages || []).filter((p) => (filter === 'all' || p.security) && p.name.includes(q.trim().toLowerCase())), [st, filter, q])

  if (isLoading) return <Skeleton className="h-64 w-full" />
  if (!st) return <p className="text-sm text-danger">Could not read the update status.</p>
  const upToDate = st.known && st.total === 0
  const busy = st.job?.state === 'running'

  return (
    <div className="space-y-5">
      <section className={cn('flex flex-wrap items-center justify-between gap-5 rounded-xl border p-5', upToDate ? 'border-success/30 bg-success-bg' : st.security ? 'border-warning/40 bg-warning-bg' : 'border-surface-border bg-surface-card')}>
        <div className="flex items-center gap-4">
          <span className={cn('flex h-14 w-14 items-center justify-center rounded-2xl text-white shadow-md', upToDate ? 'bg-success' : st.security ? 'bg-warning' : 'bg-primary')}>
            {upToDate ? <ShieldCheck size={28} /> : <ShieldAlert size={28} />}
          </span>
          <div>
            <h2 className="text-lg font-semibold text-text-primary">
              {!st.known ? 'Update status unknown' : upToDate ? 'Your server is up to date' : `${st.total} update${st.total === 1 ? '' : 's'} available`}
            </h2>
            <p className="text-sm text-text-secondary">
              {st.known && !upToDate && <>{st.security} security fix{st.security === 1 ? '' : 'es'} · </>}
              <span className="inline-flex items-center gap-1"><Clock size={12} />checked {fmtTime(st.checked_at)}</span>
            </p>
          </div>
        </div>
        {editable && (
          <div className="flex flex-wrap gap-2">
            <Button variant="secondary" disabled={busy || run.isPending} onClick={() => run.mutate('check')}><RefreshCw size={14} />Check now</Button>
            {st.security > 0 && <Button disabled={busy || run.isPending} onClick={() => setConfirm('security')}><Download size={14} />Install security updates</Button>}
            {st.total > 0 && <Button variant={st.security > 0 ? 'secondary' : 'primary'} disabled={busy || run.isPending} onClick={() => setConfirm('all')}><Download size={14} />Install all</Button>}
          </div>
        )}
      </section>

      {st.reboot_required && (
        <section className="flex flex-wrap items-center justify-between gap-4 rounded-xl border border-warning/40 bg-warning-bg p-4">
          <div className="flex items-start gap-3">
            <TriangleAlert size={20} className="mt-0.5 shrink-0 text-warning" />
            <div>
              <p className="text-sm font-semibold text-text-primary">A restart is required</p>
              <p className="text-xs text-text-secondary">
                Installed updates{st.reboot_packages?.length ? ` (${st.reboot_packages.slice(0, 3).join(', ')}${st.reboot_packages.length > 3 ? '…' : ''})` : ''} only take effect after the server restarts. Websites and the panel are offline for about a minute.
              </p>
            </div>
          </div>
          {editable && <Button variant="secondary" disabled={busy} onClick={() => setRebootAsk(true)}><Power size={14} />Restart server</Button>}
        </section>
      )}

      <section className="flex items-center justify-between gap-4 rounded-xl border border-surface-border bg-surface-card p-5">
        <div>
          <h3 className="font-semibold text-text-primary">Automatic security updates</h3>
          <p className="mt-0.5 text-sm text-text-secondary">
            Install security fixes daily in the background (Ubuntu&apos;s unattended-upgrades). The server is never restarted automatically.
            {!st.auto_installed && !st.auto_security && ' It will be installed when you switch this on.'}
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-3">
          {auto.isPending && <LoaderCircle size={16} className="animate-spin text-text-tertiary" />}
          <Badge variant={st.auto_security ? 'success' : 'neutral'}>{st.auto_security ? 'On' : 'Off'}</Badge>
          <Switch checked={st.auto_security} disabled={!editable || auto.isPending || busy} onChange={(v) => auto.mutate(v)} label="Automatic security updates" />
        </div>
      </section>

      {st.total > 0 && (
        <section className="space-y-3">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <h3 className="font-semibold text-text-primary">Available updates</h3>
            <div className="flex flex-wrap items-center gap-3">
              <div className="inline-flex rounded-md border border-surface-border bg-surface-card p-0.5">
                {[['all', `All (${st.total})`], ['security', `Security (${st.security})`]].map(([k, l]) => (
                  <button key={k} onClick={() => setFilter(k)} aria-pressed={filter === k} className={cn('h-8 rounded-[8px] px-3 text-[13px] font-medium transition-colors', filter === k ? 'bg-primary text-white' : 'text-text-secondary hover:bg-surface-hover')}>{l}</button>
                ))}
              </div>
              <div className="relative w-56">
                <Search size={14} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-text-tertiary" />
                <Input className="pl-8" placeholder="Search package…" value={q} onChange={(e) => setQ(e.target.value)} />
              </div>
            </div>
          </div>
          <div className="overflow-hidden rounded-xl border border-surface-border bg-surface-card">
            <div className="max-h-[480px] overflow-auto">
              <table className="w-full text-sm">
                <thead className="sticky top-0 bg-surface-hover text-left text-xs uppercase tracking-wide text-text-secondary">
                  <tr><th className="px-4 py-2.5 font-medium">Package</th><th className="px-4 py-2.5 font-medium">Version</th><th className="px-4 py-2.5 font-medium" /></tr>
                </thead>
                <tbody className="divide-y divide-surface-border">
                  {pkgs.map((p) => (
                    <tr key={p.name}>
                      <td className="px-4 py-2 font-mono text-[13px] text-text-primary">{p.name}</td>
                      <td className="px-4 py-2 text-xs text-text-secondary">
                        <span className="font-mono">{p.current || 'new'}</span><ArrowRight size={11} className="mx-1.5 inline" /><span className="font-mono text-text-primary">{p.new}</span>
                      </td>
                      <td className="px-4 py-2 text-right">{p.security && <Badge variant="warning">security</Badge>}</td>
                    </tr>
                  ))}
                  {pkgs.length === 0 && <tr><td colSpan={3} className="px-4 py-8 text-center text-text-tertiary">No package matches.</td></tr>}
                </tbody>
              </table>
            </div>
          </div>
          {st.total > 500 && <p className="text-xs text-text-tertiary">Showing the first 500 of {st.total} packages.</p>}
        </section>
      )}

      <JobDialog open={jobOpen} onClose={() => { setJobOpen(false); refresh() }} onFinished={refresh} />
      <ConfirmDialog
        open={!!confirm} onClose={() => setConfirm(null)} onConfirm={() => run.mutate(confirm)} loading={run.isPending}
        title={confirm === 'security' ? 'Install security updates?' : 'Install all updates?'}
        message={confirm === 'security'
          ? `${st.security} security package${st.security === 1 ? '' : 's'} will be installed. Websites keep running; some services restart briefly.`
          : `${st.total} package${st.total === 1 ? '' : 's'} will be installed. Services such as nginx or the database may restart briefly, and a reboot may be needed afterwards.`}
        confirmLabel="Install" loadingLabel="Starting…" variant="primary"
      />
      <ConfirmDialog
        open={rebootAsk} onClose={() => setRebootAsk(false)} onConfirm={() => boot.mutate()} loading={boot.isPending}
        title="Restart the server?" message="The panel and every website go offline for about a minute. Running terminal sessions and uploads are cut."
        confirmLabel="Restart now" loadingLabel="Restarting…"
      />
      <RebootDialog open={rebooting} />
    </div>
  )
}

export default Updates
