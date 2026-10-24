import { useMemo, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { Plus, Play, Pencil, Trash2, History, Terminal, Globe, Clock, CircleCheck, CircleX, PauseCircle, Timer } from 'lucide-react'
import Button from '../../components/ui/Button'
import Input from '../../components/ui/Input'
import Select from '../../components/ui/Select'
import Badge from '../../components/ui/Badge'
import Modal from '../../components/ui/Modal'
import ConfirmDialog from '../../components/ui/ConfirmDialog'
import Skeleton from '../../components/ui/Skeleton'
import EmptyState from '../../components/ui/EmptyState'
import usePermission from '../../hooks/usePermission'
import { cn } from '../../utils/cn'
import { describeSchedule, ago } from '../../utils/cron'
import * as api from '../../api/cron'

const errMsg = (e) => e.response?.data?.message || 'Operation failed'
const presets = [
  ['* * * * *', 'Every minute'], ['*/5 * * * *', 'Every 5 min'], ['0 * * * *', 'Hourly'],
  ['0 3 * * *', 'Daily 03:00'], ['0 3 * * 0', 'Weekly Sun'], ['0 3 1 * *', 'Monthly 1st'],
]
const emptyForm = { name: '', type: 'shell', schedule: '0 3 * * *', command: '', timeout: 3600 }
const statusVariant = { success: 'success', failed: 'danger', running: 'warning', skipped: 'neutral' }
const edge = { success: 'before:bg-success', failed: 'before:bg-danger', running: 'before:bg-warning' }

function Switch({ checked, onChange, disabled, label }) {
  return (
    <button role="switch" aria-checked={checked} aria-label={label} disabled={disabled} onClick={onChange}
      className={cn('relative h-5 w-9 shrink-0 rounded-full transition-colors disabled:opacity-50', checked ? 'bg-primary' : 'bg-surface-border-hover')}>
      <span className={cn('absolute top-0.5 h-4 w-4 rounded-full bg-white shadow transition-all', checked ? 'left-[18px]' : 'left-0.5')} />
    </button>
  )
}

function Stat({ icon: Icon, label, value, tone }) {
  return (
    <div className="flex items-center gap-3 rounded-xl border border-surface-border bg-surface-card p-4">
      <span className={cn('flex h-10 w-10 items-center justify-center rounded-lg', tone)}><Icon size={18} /></span>
      <div>
        <p className="text-2xl font-semibold leading-none text-text-primary">{value}</p>
        <p className="mt-1 text-xs text-text-secondary">{label}</p>
      </div>
    </div>
  )
}

function Cron() {
  const qc = useQueryClient()
  const { canCreate, canEdit, canDelete } = usePermission()
  const { data, isLoading } = useQuery({ queryKey: ['cron-jobs'], queryFn: api.getJobs, refetchInterval: 10000 })
  const jobs = data?.data || []
  const [form, setForm] = useState(null)
  const [del, setDel] = useState(null)
  const [hist, setHist] = useState(null)
  const histQ = useQuery({ queryKey: ['cron-runs', hist?.uuid], queryFn: () => api.getRuns(hist.uuid), enabled: !!hist, refetchInterval: hist ? 3000 : false })
  const [openRun, setOpenRun] = useState(null)
  const [filter, setFilter] = useState('all')

  const refresh = () => qc.invalidateQueries({ queryKey: ['cron-jobs'] })
  const m = (fn, ok, after) => ({ mutationFn: fn, onSuccess: () => { toast.success(ok); refresh(); after?.() }, onError: (e) => toast.error(errMsg(e)) })
  const save = useMutation(m((f) => {
    const body = { name: f.name, type: f.type, schedule: f.schedule, command: f.command, timeout: Number(f.timeout) || 3600 }
    return f.uuid ? api.updateJob(f.uuid, body) : api.createJob(body)
  }, 'Saved', () => setForm(null)))
  const toggle = useMutation(m((j) => (j.enabled ? api.disableJob(j.uuid) : api.enableJob(j.uuid)), 'Updated'))
  const run = useMutation(m((j) => api.runJob(j.uuid), 'Job started — see history for the result'))
  const remove = useMutation(m((j) => api.deleteJob(j.uuid), 'Job deleted', () => setDel(null)))

  const stats = useMemo(() => ({
    total: jobs.length,
    active: jobs.filter((j) => j.enabled).length,
    failed: jobs.filter((j) => j.last_status === 'failed').length,
    running: jobs.filter((j) => j.last_status === 'running').length,
  }), [jobs])
  const shown = jobs.filter((j) => filter === 'all' || (filter === 'active' && j.enabled) || (filter === 'paused' && !j.enabled) || (filter === 'failed' && j.last_status === 'failed'))

  const set = (k) => (e) => setForm({ ...form, [k]: e.target.value })

  return (
    <div className="space-y-5">
      <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
        <Stat icon={Clock} label="Total jobs" value={stats.total} tone="bg-info-bg text-info" />
        <Stat icon={CircleCheck} label="Active" value={stats.active} tone="bg-success-bg text-success" />
        <Stat icon={CircleX} label="Last run failed" value={stats.failed} tone={stats.failed ? 'bg-danger-bg text-danger' : 'bg-surface-hover text-text-tertiary'} />
        <Stat icon={Timer} label="Running now" value={stats.running} tone="bg-warning-bg text-warning" />
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex gap-1 rounded-lg bg-surface-hover p-1">
          {[['all', 'All'], ['active', 'Active'], ['paused', 'Paused'], ['failed', 'Failed']].map(([k, l]) => (
            <button key={k} onClick={() => setFilter(k)} className={cn('rounded-md px-3.5 py-1.5 text-sm font-medium transition-colors', filter === k ? 'bg-surface-card text-primary shadow-sm' : 'text-text-secondary hover:text-text-primary')}>{l}</button>
          ))}
        </div>
        <div className="flex items-center gap-3">
          <span className="hidden text-xs text-text-tertiary md:inline">Runs through system cron as root · last 30 runs kept per job</span>
          {canCreate('cron') && <Button onClick={() => setForm(emptyForm)}><Plus size={14} />New job</Button>}
        </div>
      </div>

      {isLoading ? <Skeleton className="h-48 w-full" /> : !shown.length ? (
        <div className="rounded-xl border border-dashed border-surface-border-hover bg-surface-card">
          <EmptyState icon={Clock} title={jobs.length ? 'No jobs match this filter' : 'No cron jobs yet'} message={jobs.length ? undefined : 'Schedule backups, cleanups or URL pings.'} />
        </div>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 2xl:grid-cols-3">
          {shown.map((j) => (
            <article key={j.uuid} className={cn('relative flex flex-col gap-3 overflow-hidden rounded-xl border border-surface-border bg-surface-card p-4 pl-5 before:absolute before:inset-y-0 before:left-0 before:w-1', edge[j.last_status] || 'before:bg-surface-border', !j.enabled && 'opacity-70')}>
              <div className="flex items-start justify-between gap-3">
                <div className="flex min-w-0 items-center gap-3">
                  <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">{j.type === 'url' ? <Globe size={17} /> : <Terminal size={17} />}</span>
                  <div className="min-w-0">
                    <h3 className="truncate text-sm font-semibold text-text-primary">{j.name}</h3>
                    <p className="truncate text-xs text-text-secondary">{describeSchedule(j.schedule)} · <span className="font-mono">{j.schedule}</span></p>
                  </div>
                </div>
                {canEdit('cron') && <Switch checked={j.enabled} label={j.enabled ? 'Disable job' : 'Enable job'} disabled={toggle.isPending} onChange={() => toggle.mutate(j)} />}
              </div>

              <pre className="max-h-16 overflow-hidden whitespace-pre-wrap break-all rounded-md bg-[#14130f] px-3 py-2 font-mono text-[11px] leading-snug text-gray-300">{j.command}</pre>

              <div className="flex flex-wrap items-center justify-between gap-2 border-t border-surface-border pt-3">
                <div className="flex items-center gap-2 text-xs text-text-secondary">
                  {j.last_run_at ? <><Badge variant={statusVariant[j.last_status] || 'neutral'}>{j.last_status}</Badge><span title={new Date(j.last_run_at).toLocaleString()}>{ago(j.last_run_at)}</span></> : <span className="text-text-tertiary">Never run</span>}
                  {!j.enabled && <span className="inline-flex items-center gap-1 text-text-tertiary"><PauseCircle size={12} />paused</span>}
                </div>
                <div className="flex gap-0.5">
                  {canEdit('cron') && <Button variant="ghost" className="h-8 px-2" title="Run now" disabled={run.isPending} onClick={() => run.mutate(j)}><Play size={14} /></Button>}
                  <Button variant="ghost" className="h-8 px-2" title="History" onClick={() => { setOpenRun(null); setHist(j) }}><History size={14} /></Button>
                  {canEdit('cron') && <Button variant="ghost" className="h-8 px-2" title="Edit" onClick={() => setForm({ ...emptyForm, ...j })}><Pencil size={14} /></Button>}
                  {canDelete('cron') && <Button variant="ghost" className="h-8 px-2" title="Delete" onClick={() => setDel(j)}><Trash2 size={14} className="text-danger" /></Button>}
                </div>
              </div>
            </article>
          ))}
        </div>
      )}

      <Modal open={!!form} onClose={() => setForm(null)} title={form?.uuid ? 'Edit job' : 'New job'} size="lg">
        {form && (
          <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); save.mutate(form) }}>
            <div className="grid grid-cols-2 gap-3">
              <Input label="Name" value={form.name} onChange={set('name')} autoFocus />
              <Select label="Type" value={form.type} onChange={set('type')}>
                <option value="shell">Shell script</option><option value="url">Fetch URL (GET)</option>
              </Select>
            </div>
            <div>
              <label className="text-xs font-medium uppercase tracking-[.05em] text-text-secondary">Schedule</label>
              <div className="mt-1.5 flex flex-wrap gap-1.5">
                {presets.map(([v, l]) => (
                  <button type="button" key={v} onClick={() => setForm({ ...form, schedule: v })}
                    className={cn('rounded-full border px-3 py-1 text-xs font-medium transition-colors', form.schedule === v ? 'border-primary bg-primary/10 text-primary' : 'border-surface-border text-text-secondary hover:bg-surface-hover')}>{l}</button>
                ))}
              </div>
              <div className="mt-2 flex items-center gap-3">
                <div className="flex-1"><Input className="font-mono" placeholder="min hour day month weekday" value={form.schedule} onChange={set('schedule')} /></div>
                <span className="w-44 text-xs text-text-secondary">{describeSchedule(form.schedule)}</span>
              </div>
            </div>
            {form.type === 'shell' ? (
              <div className="flex flex-col gap-1">
                <label className="text-xs font-medium uppercase tracking-[.05em] text-text-secondary">Script (bash)</label>
                <textarea rows={8} spellCheck={false} className="w-full rounded-md border-[1.5px] border-surface-border bg-[#14130f] p-3 font-mono text-xs text-gray-200 outline-none focus:border-primary" placeholder={'#!/bin/bash\ntar czf /www/backup/site.tgz /www/wwwroot/example.com'} value={form.command} onChange={set('command')} />
              </div>
            ) : (
              <Input label="URL" placeholder="https://example.com/cron.php?key=…" value={form.command} onChange={set('command')} />
            )}
            <Input label="Timeout (seconds)" type="number" value={form.timeout} onChange={set('timeout')} />
            <div className="flex justify-end gap-2">
              <Button type="button" variant="secondary" onClick={() => setForm(null)}>Cancel</Button>
              <Button type="submit" disabled={save.isPending || !form.name || !form.command}>{save.isPending ? 'Saving…' : 'Save'}</Button>
            </div>
          </form>
        )}
      </Modal>

      <Modal open={!!hist} onClose={() => setHist(null)} title={`History: ${hist?.name}`} size="xl">
        <div className="space-y-2">
          {(histQ.data?.data || []).map((r) => (
            <div key={r.id} className="overflow-hidden rounded-lg border border-surface-border">
              <button className="flex w-full items-center justify-between p-3 text-left text-sm hover:bg-surface-hover" onClick={() => setOpenRun(openRun === r.id ? null : r.id)}>
                <span>{new Date(r.started_at).toLocaleString()} <span className="text-text-tertiary">· {r.trigger}</span></span>
                <span className="flex items-center gap-2 text-xs text-text-secondary">
                  {r.finished_at ? `${Math.max(0, Math.round((new Date(r.finished_at) - new Date(r.started_at)) / 1000))}s` : 'running…'} · exit {r.exit_code}
                  <Badge variant={statusVariant[r.status] || 'neutral'}>{r.status}</Badge>
                </span>
              </button>
              {openRun === r.id && <pre className="max-h-72 overflow-auto whitespace-pre-wrap break-all bg-[#14130f] p-3 font-mono text-xs text-gray-200">{r.output || '(no output)'}</pre>}
            </div>
          ))}
          {!histQ.isLoading && !(histQ.data?.data || []).length && <p className="p-6 text-center text-sm text-text-secondary">No runs yet</p>}
        </div>
      </Modal>

      <ConfirmDialog open={!!del} onClose={() => setDel(null)} loading={remove.isPending} title={`Delete ${del?.name}`}
        message="The job, its cron file, script and run history are removed." onConfirm={() => remove.mutate(del)} />
    </div>
  )
}

export default Cron
