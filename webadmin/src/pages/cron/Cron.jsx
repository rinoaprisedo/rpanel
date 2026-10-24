import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { Plus, Play, Pencil, Trash2, History, Power } from 'lucide-react'
import Button from '../../components/ui/Button'
import Input from '../../components/ui/Input'
import Select from '../../components/ui/Select'
import Badge from '../../components/ui/Badge'
import Modal from '../../components/ui/Modal'
import ConfirmDialog from '../../components/ui/ConfirmDialog'
import Skeleton from '../../components/ui/Skeleton'
import * as api from '../../api/cron'

const errMsg = (e) => e.response?.data?.message || 'Operation failed'
const presets = [
  ['* * * * *', 'Every minute'], ['*/5 * * * *', 'Every 5 minutes'], ['0 * * * *', 'Hourly'],
  ['0 3 * * *', 'Daily at 03:00'], ['0 3 * * 0', 'Weekly (Sunday 03:00)'], ['0 3 1 * *', 'Monthly (1st, 03:00)'],
]
const emptyForm = { name: '', type: 'shell', schedule: '0 3 * * *', command: '', timeout: 3600 }
const statusVariant = { success: 'success', failed: 'danger', running: 'warning', skipped: 'neutral' }

function Cron() {
  const qc = useQueryClient()
  const { data, isLoading } = useQuery({ queryKey: ['cron-jobs'], queryFn: api.getJobs, refetchInterval: 10000 })
  const jobs = data?.data || []
  const [form, setForm] = useState(null) // { uuid?, ...fields }
  const [del, setDel] = useState(null)
  const [hist, setHist] = useState(null) // job
  const histQ = useQuery({ queryKey: ['cron-runs', hist?.uuid], queryFn: () => api.getRuns(hist.uuid), enabled: !!hist, refetchInterval: hist ? 3000 : false })
  const [openRun, setOpenRun] = useState(null)

  const refresh = () => qc.invalidateQueries({ queryKey: ['cron-jobs'] })
  const m = (fn, ok, after) => ({ mutationFn: fn, onSuccess: () => { toast.success(ok); refresh(); after?.() }, onError: (e) => toast.error(errMsg(e)) })
  const save = useMutation(m((f) => {
    const body = { name: f.name, type: f.type, schedule: f.schedule, command: f.command, timeout: Number(f.timeout) || 3600 }
    return f.uuid ? api.updateJob(f.uuid, body) : api.createJob(body)
  }, 'Saved', () => setForm(null)))
  const toggle = useMutation(m((j) => (j.enabled ? api.disableJob(j.uuid) : api.enableJob(j.uuid)), 'Updated'))
  const run = useMutation(m((j) => api.runJob(j.uuid), 'Job started — see history for the result', () => {}))
  const remove = useMutation(m((j) => api.deleteJob(j.uuid), 'Job deleted', () => setDel(null)))

  const set = (k) => (e) => setForm({ ...form, [k]: e.target.value })
  const isPreset = form && presets.some(([v]) => v === form.schedule)

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between rounded-lg border border-surface-border bg-surface-card p-4 text-sm text-text-secondary">
        <span>Jobs are scheduled through the system cron daemon and run as root. Output of the last 30 runs is kept per job.</span>
        <Button onClick={() => setForm(emptyForm)}><Plus size={14} />New job</Button>
      </div>

      <div className="overflow-x-auto rounded-lg border border-surface-border bg-surface-card">
        {isLoading ? <Skeleton className="h-48 w-full" /> : (
          <table className="w-full text-sm">
            <thead className="border-b border-surface-border text-left text-xs uppercase text-text-secondary">
              <tr><th className="p-3">Name</th><th className="p-3">Type</th><th className="p-3">Schedule</th><th className="p-3">Enabled</th><th className="p-3">Last run</th><th className="p-3 text-right">Actions</th></tr>
            </thead>
            <tbody>
              {jobs.map((j) => (
                <tr key={j.uuid} className="border-b border-surface-border last:border-0">
                  <td className="p-3 font-medium text-text-primary">{j.name}</td>
                  <td className="p-3 text-text-secondary">{j.type}</td>
                  <td className="p-3 font-mono text-xs">{j.schedule}</td>
                  <td className="p-3"><Badge variant={j.enabled ? 'success' : 'neutral'}>{j.enabled ? 'on' : 'off'}</Badge></td>
                  <td className="p-3 text-xs text-text-secondary">
                    {j.last_run_at ? <>{new Date(j.last_run_at).toLocaleString()} <Badge variant={statusVariant[j.last_status] || 'neutral'}>{j.last_status}</Badge></> : 'never'}
                  </td>
                  <td className="p-3">
                    <div className="flex justify-end gap-1">
                      <Button variant="ghost" title="Run now" disabled={run.isPending} onClick={() => run.mutate(j)}><Play size={14} /></Button>
                      <Button variant="ghost" title="History" onClick={() => { setOpenRun(null); setHist(j) }}><History size={14} /></Button>
                      <Button variant="ghost" title={j.enabled ? 'Disable' : 'Enable'} onClick={() => toggle.mutate(j)}><Power size={14} /></Button>
                      <Button variant="ghost" title="Edit" onClick={() => setForm({ ...j })}><Pencil size={14} /></Button>
                      <Button variant="ghost" title="Delete" onClick={() => setDel(j)}><Trash2 size={14} className="text-danger" /></Button>
                    </div>
                  </td>
                </tr>
              ))}
              {!jobs.length && <tr><td colSpan={6} className="p-8 text-center text-text-secondary">No cron jobs yet</td></tr>}
            </tbody>
          </table>
        )}
      </div>

      <Modal open={!!form} onClose={() => setForm(null)} title={form?.uuid ? 'Edit job' : 'New job'} size="lg">
        {form && (
          <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); save.mutate(form) }}>
            <div className="grid grid-cols-2 gap-3">
              <Input label="Name" value={form.name} onChange={set('name')} autoFocus />
              <Select label="Type" value={form.type} onChange={set('type')}>
                <option value="shell">Shell script</option><option value="url">Fetch URL (GET)</option>
              </Select>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <Select label="Preset" value={isPreset ? form.schedule : ''} onChange={(e) => e.target.value && setForm({ ...form, schedule: e.target.value })}>
                <option value="">Custom…</option>
                {presets.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
              </Select>
              <Input label="Schedule (min hour day month weekday)" className="font-mono" value={form.schedule} onChange={set('schedule')} />
            </div>
            {form.type === 'shell' ? (
              <div className="flex flex-col gap-1">
                <label className="text-xs font-medium uppercase tracking-[.05em] text-text-secondary">Script (bash)</label>
                <textarea rows={8} spellCheck={false} className="w-full rounded-md border-[1.5px] border-surface-border bg-white p-3 font-mono text-xs outline-none focus:border-primary" placeholder={'#!/bin/bash\ntar czf /www/backup/site.tgz /www/wwwroot/example.com'} value={form.command} onChange={set('command')} />
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
            <div key={r.id} className="rounded-md border border-surface-border">
              <button className="flex w-full items-center justify-between p-3 text-left text-sm" onClick={() => setOpenRun(openRun === r.id ? null : r.id)}>
                <span>{new Date(r.started_at).toLocaleString()} · {r.trigger}</span>
                <span className="flex items-center gap-2 text-xs text-text-secondary">
                  {r.finished_at ? `${Math.max(0, Math.round((new Date(r.finished_at) - new Date(r.started_at)) / 1000))}s` : 'running…'} · exit {r.exit_code}
                  <Badge variant={statusVariant[r.status] || 'neutral'}>{r.status}</Badge>
                </span>
              </button>
              {openRun === r.id && <pre className="max-h-72 overflow-auto whitespace-pre-wrap break-all rounded-b-md bg-[#1a1917] p-3 font-mono text-xs text-gray-200">{r.output || '(no output)'}</pre>}
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
