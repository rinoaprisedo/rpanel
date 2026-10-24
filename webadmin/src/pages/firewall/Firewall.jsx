import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { Plus, Trash2, ShieldCheck, ShieldOff } from 'lucide-react'
import Button from '../../components/ui/Button'
import Input from '../../components/ui/Input'
import Select from '../../components/ui/Select'
import Badge from '../../components/ui/Badge'
import Modal from '../../components/ui/Modal'
import ConfirmDialog from '../../components/ui/ConfirmDialog'
import Skeleton from '../../components/ui/Skeleton'
import * as api from '../../api/firewall'

const errMsg = (e) => e.response?.data?.message || 'Operation failed'
const isLockout = (e) => e.response?.data?.code === 'lockout'
const emptyRule = { action: 'allow', port: '', proto: 'tcp', source: 'any', comment: '' }
const actionVariant = { ALLOW: 'success', DENY: 'danger', REJECT: 'danger', LIMIT: 'warning' }

function Firewall() {
  const qc = useQueryClient()
  const { data, isLoading } = useQuery({ queryKey: ['firewall'], queryFn: api.getFirewall })
  const st = data?.data
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

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-surface-border bg-surface-card p-4">
        <div className="flex flex-wrap items-center gap-4 text-sm text-text-secondary">
          <span className="flex items-center gap-2">{st.active ? <ShieldCheck size={16} className="text-success" /> : <ShieldOff size={16} className="text-danger" />}
            Firewall <Badge variant={st.active ? 'success' : 'danger'}>{st.active ? 'active' : 'inactive'}</Badge></span>
          <span>Default incoming: <b className="text-text-primary">{st.default_incoming || '—'}</b>, outgoing: <b className="text-text-primary">{st.default_outgoing || '—'}</b></span>
          <span>Protected ports: <span className="font-mono text-text-primary">{st.protected_ports.join(', ')}</span></span>
        </div>
        <div className="flex flex-wrap gap-2">
          <Select value={st.default_incoming || ''} onChange={(e) => e.target.value && policy.mutate(e.target.value)} aria-label="Default incoming policy">
            <option value="" disabled>Default incoming…</option>
            <option value="deny">deny</option><option value="reject">reject</option><option value="allow">allow</option>
          </Select>
          {st.active
            ? <Button variant="secondary" disabled={disable.isPending} onClick={() => disable.mutate()}>Disable</Button>
            : <Button disabled={enable.isPending} onClick={() => enable.mutate()}>Enable</Button>}
          <Button onClick={() => setForm(emptyRule)}><Plus size={14} />Add rule</Button>
        </div>
      </div>
      {st.error && <p className="text-sm text-danger">{st.error}</p>}
      {!st.active && <p className="text-sm text-text-secondary">Enabling first allows the protected ports above (SSH and this panel) so you cannot lock yourself out.</p>}

      <div className="overflow-x-auto rounded-lg border border-surface-border bg-surface-card">
        <table className="w-full text-sm">
          <thead className="border-b border-surface-border text-left text-xs uppercase text-text-secondary">
            <tr><th className="p-3">#</th><th className="p-3">To (port)</th><th className="p-3">Action</th><th className="p-3">Direction</th><th className="p-3">From</th><th className="p-3" /></tr>
          </thead>
          <tbody>
            {st.rules.map((r) => (
              <tr key={r.number} className="border-b border-surface-border last:border-0">
                <td className="p-3 text-text-secondary">{r.number}</td>
                <td className="p-3 font-mono text-xs">{r.to}</td>
                <td className="p-3"><Badge variant={actionVariant[r.action] || 'neutral'}>{r.action}</Badge></td>
                <td className="p-3 text-text-secondary">{r.direction}</td>
                <td className="p-3 font-mono text-xs">{r.from}</td>
                <td className="p-3 text-right"><Button variant="ghost" title="Delete rule" onClick={() => setDel(r)}><Trash2 size={14} className="text-danger" /></Button></td>
              </tr>
            ))}
            {!st.rules.length && <tr><td colSpan={6} className="p-8 text-center text-text-secondary">No rules</td></tr>}
          </tbody>
        </table>
      </div>

      <Modal open={!!form} onClose={() => setForm(null)} title="Add rule">
        {form && (
          <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); submitRule(false) }}>
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
