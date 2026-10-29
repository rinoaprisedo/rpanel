import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { Plus, Trash2, ShieldCheck, ShieldOff, Lock, ArrowDownToLine, ArrowUpFromLine, Search } from 'lucide-react'
import Button from '../../components/ui/Button'
import Input from '../../components/ui/Input'
import Select from '../../components/ui/Select'
import Badge from '../../components/ui/Badge'
import Modal from '../../components/ui/Modal'
import ConfirmDialog from '../../components/ui/ConfirmDialog'
import Skeleton from '../../components/ui/Skeleton'
import usePermission from '../../hooks/usePermission'
import { cn } from '../../utils/cn'
import * as api from '../../api/firewall'

const errMsg = (e) => e.response?.data?.message || 'Operation failed'
const isLockout = (e) => e.response?.data?.code === 'lockout'
const emptyRule = { action: 'allow', port: '', proto: 'tcp', source: 'any', comment: '' }
const quick = [['SSH', '22', 'tcp'], ['HTTP', '80', 'tcp'], ['HTTPS', '443', 'tcp'], ['MySQL', '3306', 'tcp'], ['PostgreSQL', '5432', 'tcp'], ['Redis', '6379', 'tcp']]
const actionVariant = { ALLOW: 'success', DENY: 'danger', REJECT: 'danger', LIMIT: 'warning' }

function Firewall() {
  const qc = useQueryClient()
  const { data, isLoading } = useQuery({ queryKey: ['firewall'], queryFn: api.getFirewall })
  const st = data?.data
  const { canCreate, canEdit, canDelete } = usePermission()
  const [q, setQ] = useState('')
  const [fa, setFa] = useState('all')
  const [form, setForm] = useState(null)
  const [del, setDel] = useState(null)
  const [warn, setWarn] = useState(null) // { message, retry }

  const refresh = () => qc.invalidateQueries({ queryKey: ['firewall'] })
  const m = (fn, ok, after) => ({
    mutationFn: fn,
    onSuccess: () => { if (ok) toast.success(ok); refresh(); after?.() },
    onError: (e, vars) => {
      if (isLockout(e)) {
        // Ask once; the retry carries confirm=true.
        setWarn({ message: errMsg(e), retry: () => vars?.__retry?.() })
      } else toast.error(errMsg(e))
    },
  })
  const install = useMutation(m(api.installFirewall, 'ufw installed'))
  const enable = useMutation(m(api.enableFirewall, 'Firewall enabled'))
  const disable = useMutation(m(api.disableFirewall, 'Firewall disabled'))
  const policy = useMutation(m((p) => api.setDefaultIncoming(p), 'Default policy updated'))
  const add = useMutation(m((v) => api.addRule(v.rule), 'Rule added', () => setForm(null)))
  const remove = useMutation(m((v) => api.deleteRule(v.rule, v.confirm), 'Rule deleted', () => setDel(null)))

  const submitRule = (confirm = false) => add.mutate({ rule: { ...form, confirm }, __retry: () => submitRule(true) })
  const doDelete = (confirm = false) => remove.mutate({ rule: del, confirm, __retry: () => doDelete(true) })

  if (isLoading) return <Skeleton className="h-64 w-full" />

  if (!st?.installed) {
    return (
      <div className="rounded-lg border border-surface-border bg-surface-card p-8 text-center text-sm">
        <p className="mb-4 text-text-secondary">UFW is not installed on this server.</p>
        <Button disabled={install.isPending} onClick={() => install.mutate()}>{install.isPending ? 'Installing…' : 'Install UFW'}</Button>
      </div>
    )
  }

  const rules = st.rules.filter((r) => (fa === 'all' || r.action.toLowerCase().startsWith(fa)) && `${r.to} ${r.from} ${r.action}`.toLowerCase().includes(q.toLowerCase()))
  const counts = st.rules.reduce((m, r) => ({ ...m, [r.action]: (m[r.action] || 0) + 1 }), {})

  return (
    <div className="space-y-5">
      <section className={cn('flex flex-wrap items-center justify-between gap-5 rounded-xl border p-5', st.active ? 'border-success/30 bg-success-bg' : 'border-danger/30 bg-danger-bg')}>
        <div className="flex items-center gap-4">
          <span className={cn('flex h-14 w-14 items-center justify-center rounded-2xl text-white shadow-md', st.active ? 'bg-success' : 'bg-danger')}>
            {st.active ? <ShieldCheck size={28} /> : <ShieldOff size={28} />}
          </span>
          <div>
            <h2 className="text-lg font-semibold text-text-primary">Firewall is {st.active ? 'active' : 'inactive'}</h2>
            <p className="text-sm text-text-secondary">
              {st.active ? `${st.rules.length} rule${st.rules.length === 1 ? '' : 's'} protecting this server` : 'All ports are reachable. Enabling first allows SSH and this panel so you cannot lock yourself out.'}
            </p>
          </div>
        </div>
        {canEdit('firewall') && (st.active
          ? <Button variant="secondary" disabled={disable.isPending} onClick={() => disable.mutate()}>Disable firewall</Button>
          : <Button disabled={enable.isPending} onClick={() => enable.mutate()}>Enable firewall</Button>)}
      </section>
      {st.error && <p className="text-sm text-danger">{st.error}</p>}

      <div className="grid gap-3 md:grid-cols-3">
        <div className="rounded-xl border border-surface-border bg-surface-card p-4">
          <p className="mb-2 flex items-center gap-2 text-xs font-medium text-text-secondary"><ArrowDownToLine size={14} />Incoming traffic</p>
          <div className="flex gap-1 rounded-lg bg-surface-hover p-1">
            {['deny', 'reject', 'allow'].map((p) => (
              <button key={p} disabled={!canEdit('firewall') || policy.isPending} onClick={() => st.default_incoming !== p && policy.mutate(p)}
                className={cn('flex-1 rounded-md py-1.5 text-xs font-medium capitalize transition-colors disabled:cursor-not-allowed', st.default_incoming === p ? (p === 'allow' ? 'bg-warning text-white' : 'bg-success text-white') : 'text-text-secondary hover:text-text-primary')}>{p}</button>
            ))}
          </div>
          <p className="mt-2 text-[11px] text-text-tertiary">Default for ports without a rule. “deny” is the safest.</p>
        </div>
        <div className="rounded-xl border border-surface-border bg-surface-card p-4">
          <p className="mb-2 flex items-center gap-2 text-xs font-medium text-text-secondary"><ArrowUpFromLine size={14} />Outgoing traffic</p>
          <p className="text-xl font-semibold capitalize text-text-primary">{st.default_outgoing || '—'}</p>
          <p className="mt-2 text-[11px] text-text-tertiary">What this server may connect to.</p>
        </div>
        <div className="rounded-xl border border-surface-border bg-surface-card p-4">
          <p className="mb-2 flex items-center gap-2 text-xs font-medium text-text-secondary"><Lock size={14} />Protected ports</p>
          <div className="flex flex-wrap gap-1.5">
            {st.protected_ports.map((p) => <span key={p} className="rounded-md bg-surface-hover px-2 py-1 font-mono text-xs text-text-primary">{p}</span>)}
          </div>
          <p className="mt-2 text-[11px] text-text-tertiary">Cannot be blocked, to keep you connected.</p>
        </div>
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-3">
          <div className="flex gap-1 rounded-lg bg-surface-hover p-1">
            {[['all', `All (${st.rules.length})`], ['allow', `Allow (${counts.ALLOW || 0})`], ['deny', `Deny (${(counts.DENY || 0) + (counts.REJECT || 0)})`], ['limit', `Limit (${counts.LIMIT || 0})`]].map(([k, l]) => (
              <button key={k} onClick={() => setFa(k)} className={cn('rounded-md px-3 py-1.5 text-xs font-medium transition-colors', fa === k ? 'bg-surface-card text-primary shadow-sm' : 'text-text-secondary hover:text-text-primary')}>{l}</button>
            ))}
          </div>
          <div className="relative">
            <Search size={14} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-text-tertiary" />
            <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Search port or source…" className="h-9 w-56 rounded-md border-[1.5px] border-surface-border bg-field pl-8 pr-3 text-sm outline-none focus:border-primary" />
          </div>
        </div>
        {canCreate('firewall') && <Button onClick={() => setForm(emptyRule)}><Plus size={14} />Add rule</Button>}
      </div>

      <div className="overflow-x-auto rounded-xl border border-surface-border bg-surface-card">
        <table className="w-full text-sm">
          <thead className="bg-surface-hover text-left text-xs uppercase text-text-secondary">
            <tr><th className="p-3 pl-4">#</th><th className="p-3">Port / service</th><th className="p-3">Action</th><th className="p-3">Direction</th><th className="p-3">From</th><th className="p-3" /></tr>
          </thead>
          <tbody>
            {rules.map((r) => (
              <tr key={r.number} className="group border-t border-surface-border hover:bg-surface-hover/60">
                <td className="p-3 pl-4 text-text-tertiary">{r.number}</td>
                <td className="p-3"><span className="rounded-md bg-surface-hover px-2 py-1 font-mono text-xs font-medium text-text-primary">{r.to}</span></td>
                <td className="p-3"><Badge variant={actionVariant[r.action] || 'neutral'}>{r.action}</Badge></td>
                <td className="p-3 text-text-secondary">{r.direction}</td>
                <td className="p-3 font-mono text-xs text-text-secondary">{r.from}</td>
                <td className="p-3 text-right">{r.locked ? <span className="inline-flex h-8 items-center gap-1 px-2 text-xs text-text-tertiary" title="This rule keeps the panel reachable and cannot be deleted"><Lock size={13} />Panel</span> : canDelete('firewall') && <Button variant="ghost" className="h-8 px-2 opacity-50 group-hover:opacity-100" title="Delete rule" onClick={() => setDel(r)}><Trash2 size={14} className="text-danger" /></Button>}</td>
              </tr>
            ))}
            {!rules.length && <tr><td colSpan={6} className="p-10 text-center text-text-secondary">{st.rules.length ? 'No rule matches' : 'No rules yet'}</td></tr>}
          </tbody>
        </table>
      </div>

      <Modal open={!!form} onClose={() => setForm(null)} title="Add rule">
        {form && (
          <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); submitRule(false) }}>
            <div>
              <p className="mb-1.5 text-xs font-medium uppercase tracking-[.05em] text-text-secondary">Quick fill</p>
              <div className="flex flex-wrap gap-1.5">
                {quick.map(([n, port, proto]) => (
                  <button type="button" key={n} onClick={() => setForm({ ...form, port, proto, comment: form.comment || n })}
                    className={cn('rounded-full border px-3 py-1 text-xs font-medium transition-colors', form.port === port ? 'border-primary bg-primary/10 text-primary' : 'border-surface-border text-text-secondary hover:bg-surface-hover')}>{n} <span className="font-mono opacity-70">{port}</span></button>
                ))}
              </div>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <Select label="Action" value={form.action} onChange={(e) => setForm({ ...form, action: e.target.value })}>
                <option value="allow">allow</option><option value="deny">deny</option><option value="reject">reject</option><option value="limit">limit (rate limit)</option>
              </Select>
              <Select label="Protocol" value={form.proto} onChange={(e) => setForm({ ...form, proto: e.target.value })}>
                <option value="tcp">tcp</option><option value="udp">udp</option><option value="any">any</option>
              </Select>
            </div>
            <Input label="Port or range" placeholder="80 or 8000:8100" value={form.port} onChange={(e) => setForm({ ...form, port: e.target.value })} autoFocus />
            <Input label="Source (any, IP or CIDR)" value={form.source} onChange={(e) => setForm({ ...form, source: e.target.value })} />
            <Input label="Comment (optional)" value={form.comment} onChange={(e) => setForm({ ...form, comment: e.target.value })} />
            <div className="flex justify-end gap-2">
              <Button type="button" variant="secondary" onClick={() => setForm(null)}>Cancel</Button>
              <Button type="submit" disabled={add.isPending || !form.port}>{add.isPending ? 'Adding…' : 'Add'}</Button>
            </div>
          </form>
        )}
      </Modal>

      <ConfirmDialog open={!!del} onClose={() => setDel(null)} loading={remove.isPending} title={`Delete rule ${del?.number}`}
        message={`${del?.action} ${del?.to} from ${del?.from}. IPv4 and IPv6 variants are separate rules.`} onConfirm={() => doDelete(false)} />

      <ConfirmDialog open={!!warn} onClose={() => setWarn(null)} title="This could lock you out" variant="danger" confirmLabel="Do it anyway" loadingLabel="Working…"
        message={`${warn?.message}. Make sure you have another way into the server (console access) before continuing.`}
        onConfirm={() => { const w = warn; setWarn(null); w.retry() }} />
    </div>
  )
}

export default Firewall
