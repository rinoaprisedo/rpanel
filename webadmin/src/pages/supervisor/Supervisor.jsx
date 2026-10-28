import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { Plus, Play, Square, RotateCw, Trash2, ScrollText, Pencil, Search, Layers, ShieldOff, RefreshCw, Info } from 'lucide-react'
import Button from '../../components/ui/Button'
import Input from '../../components/ui/Input'
import Select from '../../components/ui/Select'
import Badge from '../../components/ui/Badge'
import Modal from '../../components/ui/Modal'
import Switch from '../../components/ui/Switch'
import ConfirmDialog from '../../components/ui/ConfirmDialog'
import Skeleton from '../../components/ui/Skeleton'
import usePermission from '../../hooks/usePermission'
import { cn } from '../../utils/cn'
import * as api from '../../api/supervisor'

const errMsg = (e) => e.response?.data?.message || 'Operation failed'
const empty = { name: '', command: '', directory: '', user: 'root', numprocs: 1, autostart: true, autorestart: 'true', env: '' }
const stateVariant = { RUNNING: 'success', STARTING: 'warning', BACKOFF: 'warning', STOPPING: 'warning', STOPPED: 'neutral', EXITED: 'neutral', FATAL: 'danger', 'NOT LOADED': 'neutral', UNKNOWN: 'neutral' }
const toForm = (c) => ({ name: c.name, command: c.command, directory: c.directory || '', user: c.user || '', numprocs: c.numprocs, autostart: c.autostart, autorestart: c.autorestart, env: (c.environment || []).join('\n') })
const toBody = (f) => ({
  name: f.name.trim(), command: f.command.trim(), directory: f.directory.trim(), user: f.user.trim(), numprocs: Number(f.numprocs) || 1,
  autostart: f.autostart, autorestart: f.autorestart, environment: f.env.split('\n').map((x) => x.trim()).filter(Boolean),
})

function Supervisor() {
  const qc = useQueryClient()
  const { canEdit, canCreate, canDelete } = usePermission()
  const { data: inst } = useQuery({ queryKey: ['supervisor', 'installed'], queryFn: api.getInstalled })
  const installed = inst?.data?.installed
  const statusQ = useQuery({ queryKey: ['supervisor', 'status'], queryFn: api.getStatus, enabled: installed === true, refetchInterval: 5000 })
  const st = statusQ.data?.data

  const [q, setQ] = useState('')
  const [form, setForm] = useState(null) // { isNew, ...fields }
  const [logs, setLogs] = useState(null) // { name, stream }
  const [del, setDel] = useState(null)

  const refresh = () => qc.invalidateQueries({ queryKey: ['supervisor'] })
  const onError = (e) => toast.error(errMsg(e), { duration: 8000 })
  const act = useMutation({ mutationFn: ({ name, action }) => api.programAction(name, action), onSuccess: (_, v) => { toast.success(`${v.name}: ${v.action} done`); refresh() }, onError: (e) => { onError(e); refresh() } })
  const daemon = useMutation({ mutationFn: api.daemon, onSuccess: (_, a) => { toast.success(`Supervisor ${{ start: 'started', stop: 'stopped', restart: 'restarted' }[a]}`); refresh() }, onError })
  const save = useMutation({
    mutationFn: (f) => (f.isNew ? api.createProgram(toBody(f)) : api.updateProgram(f.name, toBody(f))),
    onSuccess: (_, f) => { toast.success(f.isNew ? 'Program added' : 'Program updated'); setForm(null); refresh() },
    onError,
  })
  const remove = useMutation({ mutationFn: (name) => api.deleteProgram(name), onSuccess: () => { toast.success('Program removed'); setDel(null); refresh() }, onError: (e) => { setDel(null); onError(e) } })

  const logQ = useQuery({ queryKey: ['supervisor', 'log', logs?.name, logs?.stream], queryFn: () => api.getLog(logs.name, logs.stream), enabled: !!logs, refetchInterval: 4000, retry: false })

  const procs = useMemo(() => (st?.processes || []).filter((p) => p.name.toLowerCase().includes(q.trim().toLowerCase())), [st, q])
  const counts = useMemo(() => (st?.processes || []).reduce((a, p) => ({ ...a, [p.state]: (a[p.state] || 0) + 1 }), {}), [st])

  if (installed === undefined) return <Skeleton className="h-64 w-full" />
  if (!installed) {
    return (
      <div className="rounded-xl border border-surface-border bg-surface-card p-10 text-center">
        <span className="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-surface-hover text-text-secondary"><Layers size={28} /></span>
        <h2 className="text-lg font-semibold text-text-primary">Supervisor is not installed</h2>
        <p className="mx-auto mt-1 max-w-md text-sm text-text-secondary">Install it from the App Store to keep queue workers, bots and other long-running processes alive.</p>
        <Link to="/app-store" className="mt-5 inline-flex h-[38px] items-center rounded-md bg-primary px-4 text-[13px] font-medium text-white hover:opacity-90">Open App Store</Link>
      </div>
    )
  }

  const running = !!st?.running
  const total = st?.processes?.length || 0
  const set = (k) => (e) => setForm({ ...form, [k]: e.target.value })

  return (
    <div className="space-y-5">
      <section className={cn('flex flex-wrap items-center justify-between gap-5 rounded-xl border p-5', !st ? 'border-surface-border bg-surface-card' : running ? 'border-success/30 bg-success-bg' : 'border-danger/30 bg-danger-bg')}>
        <div className="flex items-center gap-4">
          <span className={cn('flex h-14 w-14 items-center justify-center rounded-2xl text-white shadow-md', running ? 'bg-success' : 'bg-danger')}>{running ? <Layers size={28} /> : <ShieldOff size={28} />}</span>
          <div>
            <h2 className="text-lg font-semibold text-text-primary">Supervisor is {!st ? '…' : running ? 'running' : 'stopped'}</h2>
            <p className="text-sm text-text-secondary">
              {running ? `${total} process${total === 1 ? '' : 'es'} · ${counts.RUNNING || 0} running${counts.FATAL ? ` · ${counts.FATAL} failed` : ''}` : 'Programs are not supervised while the service is stopped.'}
              {st?.version && <span className="ml-2 font-mono text-xs text-text-tertiary">v{st.version}</span>}
            </p>
          </div>
        </div>
        {canEdit('supervisor') && (
          <div className="flex flex-wrap gap-2">
            {running ? (
              <>
                <Button variant="secondary" disabled={daemon.isPending} onClick={() => daemon.mutate('restart')}><RotateCw size={14} />Restart service</Button>
                <Button variant="secondary" disabled={daemon.isPending} onClick={() => daemon.mutate('stop')}><Square size={14} />Stop</Button>
              </>
            ) : <Button disabled={daemon.isPending} onClick={() => daemon.mutate('start')}><Play size={14} />Start</Button>}
          </div>
        )}
      </section>
      {st?.error && <p className="text-sm text-danger">{st.error}</p>}

      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="relative w-full max-w-xs">
          <Search size={14} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-text-tertiary" />
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Search program…" className="h-[38px] w-full rounded-md border-[1.5px] border-surface-border bg-field pl-8 pr-3 text-sm outline-none focus:border-primary" />
        </div>
        <div className="flex gap-2">
          <Button variant="secondary" onClick={() => statusQ.refetch()} aria-label="Refresh"><RefreshCw size={14} className={statusQ.isFetching ? 'animate-spin' : ''} /></Button>
          {canCreate('supervisor') && <Button disabled={!running} onClick={() => setForm({ ...empty, isNew: true })}><Plus size={14} />Add program</Button>}
        </div>
      </div>

      <div className="overflow-x-auto rounded-xl border border-surface-border bg-surface-card">
        {statusQ.isLoading ? <Skeleton className="h-40 w-full" /> : (
          <table className="w-full text-sm">
            <thead className="border-b border-surface-border text-left text-xs uppercase text-text-secondary">
              <tr><th className="p-3">Program</th><th className="p-3">State</th><th className="p-3">Details</th><th className="p-3 text-right">Actions</th></tr>
            </thead>
            <tbody>
              {procs.map((p) => {
                const live = p.state === 'RUNNING' || p.state === 'STARTING' || p.state === 'BACKOFF'
                return (
                  <tr key={p.name} className="border-b border-surface-border last:border-0">
                    <td className="p-3">
                      <span className="font-medium text-text-primary">{p.name}</span>
                      {p.managed ? <span className="ml-2 rounded bg-primary/10 px-1.5 py-0.5 text-[10px] font-medium text-primary">panel</span> : <span className="ml-2 rounded bg-surface-hover px-1.5 py-0.5 text-[10px] text-text-tertiary" title="Defined outside the panel: it can be controlled here but not edited">external</span>}
                      {p.config?.command && <p className="mt-0.5 max-w-md truncate font-mono text-[11px] text-text-tertiary" title={p.config.command}>{p.config.command}</p>}
                    </td>
                    <td className="p-3"><Badge variant={stateVariant[p.state] || 'neutral'}>{p.state}</Badge></td>
                    <td className="max-w-xs truncate p-3 text-xs text-text-secondary" title={p.detail}>{p.detail}</td>
                    <td className="p-3">
                      <div className="flex justify-end gap-0.5">
                        {canEdit('supervisor') && p.state !== 'NOT LOADED' && (live
                          ? <>
                            <Button variant="ghost" className="h-8 w-8 !px-0" title="Restart" disabled={act.isPending} onClick={() => act.mutate({ name: p.name, action: 'restart' })}><RotateCw size={14} /></Button>
                            <Button variant="ghost" className="h-8 w-8 !px-0" title="Stop" disabled={act.isPending} onClick={() => act.mutate({ name: p.name, action: 'stop' })}><Square size={14} /></Button>
                          </>
                          : <Button variant="ghost" className="h-8 w-8 !px-0" title="Start" disabled={act.isPending} onClick={() => act.mutate({ name: p.name, action: 'start' })}><Play size={14} /></Button>)}
                        {p.state !== 'NOT LOADED' && <Button variant="ghost" className="h-8 w-8 !px-0" title="Logs" onClick={() => setLogs({ name: p.name, stream: p.state === 'FATAL' ? 'stderr' : 'stdout' })}><ScrollText size={14} /></Button>}
                        {p.managed && canEdit('supervisor') && <Button variant="ghost" className="h-8 w-8 !px-0" title="Edit" onClick={() => setForm({ ...toForm(p.config), isNew: false })}><Pencil size={14} /></Button>}
                        {p.managed && canDelete('supervisor') && <Button variant="ghost" className="h-8 w-8 !px-0" title="Remove" onClick={() => setDel(p.program)}><Trash2 size={14} className="text-danger" /></Button>}
                      </div>
                    </td>
                  </tr>
                )
              })}
              {!procs.length && <tr><td colSpan={4} className="p-10 text-center text-text-secondary">{q.trim() ? 'No program matches.' : running ? 'No program yet. Add one to keep a worker or bot running.' : 'Start the service to see programs.'}</td></tr>}
            </tbody>
          </table>
        )}
      </div>

      <Modal open={!!form} onClose={() => setForm(null)} title={form?.isNew ? 'Add program' : `Edit ${form?.name}`} size="lg">
        {form && (
          <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); save.mutate(form) }}>
            <div className="grid gap-3 sm:grid-cols-2">
              <Input label="Name" placeholder="queue-worker" value={form.name} onChange={set('name')} disabled={!form.isNew} autoFocus={form.isNew} />
              <Input label="Run as user" placeholder="www-data" value={form.user} onChange={set('user')} />
            </div>
            <Input label="Command" placeholder="php /www/wwwroot/app/artisan queue:work --sleep=3" value={form.command} onChange={set('command')} className="font-mono text-xs" />
            <Input label="Working directory (optional)" placeholder="/www/wwwroot/app" value={form.directory} onChange={set('directory')} className="font-mono text-xs" />
            <div className="grid gap-3 sm:grid-cols-3">
              <Input label="Processes" type="number" min={1} max={32} value={form.numprocs} onChange={set('numprocs')} />
              <Select label="Restart when it exits" value={form.autorestart} onChange={set('autorestart')}>
                <option value="true">Always</option><option value="unexpected">Only on failure</option><option value="false">Never</option>
              </Select>
              <label className="flex items-end gap-3 pb-2"><Switch checked={form.autostart} onChange={(v) => setForm({ ...form, autostart: v })} label="Start automatically" /><span className="text-sm text-text-primary">Start with the server</span></label>
            </div>
            <div className="flex flex-col gap-1">
              <label className="text-xs font-medium uppercase tracking-[.05em] text-text-secondary">Environment (one KEY=value per line)</label>
              <textarea rows={3} spellCheck={false} value={form.env} onChange={set('env')} placeholder="APP_ENV=production" className="w-full rounded-md border-[1.5px] border-surface-border bg-field p-3 font-mono text-xs outline-none focus:border-primary" />
            </div>
            <p className="flex items-start gap-2 rounded-md bg-surface-hover p-3 text-xs text-text-secondary"><Info size={14} className="mt-0.5 shrink-0" />Supervisor cannot check the command in advance: if it fails to start the program shows FATAL, and its Logs tell you why. Output is written to /var/log/supervisor/{form.name || '<name>'}.out.log and .err.log.</p>
            <div className="flex justify-end gap-2">
              <Button type="button" variant="secondary" onClick={() => setForm(null)}>Cancel</Button>
              <Button type="submit" disabled={save.isPending || !form.name || !form.command}>{save.isPending ? 'Applying…' : form.isNew ? 'Add & start' : 'Save & apply'}</Button>
            </div>
          </form>
        )}
      </Modal>

      <Modal open={!!logs} onClose={() => setLogs(null)} title={`Logs: ${logs?.name || ''}`} size="xl">
        {logs && (
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <div className="inline-flex overflow-hidden rounded-md border border-surface-border">
                {['stdout', 'stderr'].map((s) => <button key={s} onClick={() => setLogs({ ...logs, stream: s })} aria-pressed={logs.stream === s} className={cn('h-[34px] px-4 text-[13px] font-medium', logs.stream === s ? 'bg-primary text-white' : 'bg-surface-card text-text-secondary hover:bg-surface-hover')}>{s}</button>)}
              </div>
              <Button variant="secondary" onClick={() => logQ.refetch()}><RefreshCw size={14} className={logQ.isFetching ? 'animate-spin' : ''} />Refresh</Button>
            </div>
            <pre className="h-[55vh] overflow-auto whitespace-pre-wrap break-all rounded-md bg-[#0b0f14] p-3 font-mono text-xs leading-5 text-slate-300">
              {logQ.isError ? errMsg(logQ.error) : (logQ.data?.data?.log || '(empty)')}
            </pre>
            <p className="text-[11px] text-text-tertiary">The last 20 KB · refreshes every 4 s.</p>
          </div>
        )}
      </Modal>

      <ConfirmDialog open={!!del} onClose={() => setDel(null)} onConfirm={() => remove.mutate(del)} loading={remove.isPending}
        title={`Remove ${del}?`} message="The program is stopped and its configuration is deleted. Its log files are kept." confirmLabel="Remove" loadingLabel="Removing…" />
    </div>
  )
}

export default Supervisor
