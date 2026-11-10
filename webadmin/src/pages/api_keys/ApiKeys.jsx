import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { Plus, KeyRound, Copy, Ban, Trash2, TriangleAlert } from 'lucide-react'
import Button from '../../components/ui/Button'
import Input from '../../components/ui/Input'
import Badge from '../../components/ui/Badge'
import Modal from '../../components/ui/Modal'
import ConfirmDialog from '../../components/ui/ConfirmDialog'
import Skeleton from '../../components/ui/Skeleton'
import EmptyState from '../../components/ui/EmptyState'
import usePermission from '../../hooks/usePermission'
import { cn } from '../../utils/cn'
import { ago } from '../../utils/cron'
import * as api from '../../api/apiKeys'

const errMsg = (e) => e.response?.data?.message || 'Operation failed'
const SCOPES = {
  'deploy:upload': ['Upload files', 'Upload image archives and compose files into the deploy folders'],
  'deploy:docker': ['Docker deploy', 'docker load, edit compose files, compose up / down / logs'],
  'deploy:ssh': ['SSH access', 'Register a public key so the tool can open an SSH session'],
}
const emptyForm = { name: '', scopes: ['deploy:upload', 'deploy:docker'], allowed_ips: '', expires_days: 0 }

function ApiKeys() {
  const qc = useQueryClient()
  const { canCreate, canEdit, canDelete } = usePermission()
  const { data, isLoading } = useQuery({ queryKey: ['api-keys'], queryFn: api.getKeys })
  const keys = data?.data?.keys || []
  const [form, setForm] = useState(null)
  const [created, setCreated] = useState(null)
  const [del, setDel] = useState(null)
  const [rev, setRev] = useState(null)
  const refresh = () => qc.invalidateQueries({ queryKey: ['api-keys'] })

  const create = useMutation({
    mutationFn: (f) => api.createKey({ name: f.name, scopes: f.scopes, expires_days: Number(f.expires_days) || 0, allowed_ips: f.allowed_ips.split(/[\s,]+/).filter(Boolean) }),
    onSuccess: (r) => { setForm(null); setCreated(r.data.key); refresh() },
    onError: (e) => toast.error(errMsg(e)),
  })
  const revoke = useMutation({ mutationFn: (k) => api.revokeKey(k.uuid), onSuccess: () => { toast.success('Key revoked'); setRev(null); refresh() }, onError: (e) => toast.error(errMsg(e)) })
  const remove = useMutation({ mutationFn: (k) => api.deleteKey(k.uuid), onSuccess: () => { toast.success('Key deleted'); setDel(null); refresh() }, onError: (e) => toast.error(errMsg(e)) })

  const toggleScope = (s) => setForm({ ...form, scopes: form.scopes.includes(s) ? form.scopes.filter((x) => x !== s) : [...form.scopes, s] })
  const state = (k) => (k.revoked_at ? ['danger', 'Revoked'] : k.expires_at && new Date(k.expires_at) < new Date() ? ['warning', 'Expired'] : ['success', 'Active'])

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="max-w-2xl text-sm text-text-secondary">Keys let an external tool such as Vibecoder upload image archives and run Docker deploys without signing in to this panel. A key only has the scopes you give it, and only works inside the deploy folders.</p>
        {canCreate('api-keys') && <Button onClick={() => setForm(emptyForm)}><Plus size={14} />New key</Button>}
      </div>

      {isLoading ? <Skeleton className="h-40 w-full" /> : !keys.length ? (
        <div className="rounded-xl border border-dashed border-surface-border-hover bg-surface-card">
          <EmptyState icon={KeyRound} title="No API keys yet" message="Create one, then paste it into Vibecoder (Rpanel menu)." />
        </div>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 2xl:grid-cols-3">
          {keys.map((k) => {
            const [variant, label] = state(k)
            return (
              <article key={k.uuid} className={cn('flex flex-col gap-3 rounded-xl border border-surface-border bg-surface-card p-4', variant !== 'success' && 'opacity-70')}>
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <h3 className="truncate text-sm font-semibold text-text-primary">{k.name}</h3>
                    <p className="font-mono text-xs text-text-secondary">{k.prefix}…</p>
                  </div>
                  <Badge variant={variant}>{label}</Badge>
                </div>
                <div className="flex flex-wrap gap-1.5">
                  {k.scopes.map((s) => <span key={s} className="rounded-full bg-primary/10 px-2.5 py-0.5 text-xs font-medium text-primary">{SCOPES[s]?.[0] || s}</span>)}
                </div>
                <dl className="grid grid-cols-2 gap-x-3 gap-y-1 text-xs text-text-secondary">
                  <dt>Last used</dt><dd className="text-text-primary">{k.last_used_at ? `${ago(k.last_used_at)}${k.last_used_ip ? ` · ${k.last_used_ip}` : ''}` : 'Never'}</dd>
                  <dt>Allowed IPs</dt><dd className="break-all text-text-primary">{k.allowed_ips || 'Any'}</dd>
                  <dt>Expires</dt><dd className="text-text-primary">{k.expires_at ? new Date(k.expires_at).toLocaleDateString() : 'Never'}</dd>
                </dl>
                <div className="flex justify-end gap-0.5 border-t border-surface-border pt-3">
                  {canEdit('api-keys') && !k.revoked_at && <Button variant="ghost" className="h-8 px-2" title="Revoke" onClick={() => setRev(k)}><Ban size={14} /></Button>}
                  {canDelete('api-keys') && <Button variant="ghost" className="h-8 px-2" title="Delete" onClick={() => setDel(k)}><Trash2 size={14} className="text-danger" /></Button>}
                </div>
              </article>
            )
          })}
        </div>
      )}

      <Modal open={!!form} onClose={() => setForm(null)} title="New API key" size="lg">
        {form && (
          <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); create.mutate(form) }}>
            <Input label="Name" placeholder="Vibecoder on my Mac" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} autoFocus />
            <div className="space-y-2">
              <label className="text-xs font-medium uppercase tracking-[.05em] text-text-secondary">Scopes</label>
              {Object.entries(SCOPES).map(([s, [title, hint]]) => (
                <label key={s} className="flex cursor-pointer items-start gap-3 rounded-lg border border-surface-border p-3 hover:bg-surface-hover">
                  <input type="checkbox" className="mt-1" checked={form.scopes.includes(s)} onChange={() => toggleScope(s)} />
                  <span><span className="block text-sm font-medium text-text-primary">{title}</span><span className="text-xs text-text-secondary">{hint}</span></span>
                </label>
              ))}
            </div>
            <Input label="Allowed IPs (optional)" placeholder="203.0.113.7, 10.0.0.0/24" value={form.allowed_ips} onChange={(e) => setForm({ ...form, allowed_ips: e.target.value })} />
            <Input label="Expires after (days, 0 = never)" type="number" value={form.expires_days} onChange={(e) => setForm({ ...form, expires_days: e.target.value })} />
            <div className="flex justify-end gap-2">
              <Button type="button" variant="secondary" onClick={() => setForm(null)}>Cancel</Button>
              <Button type="submit" disabled={create.isPending || !form.name.trim() || !form.scopes.length}>{create.isPending ? 'Creating…' : 'Create key'}</Button>
            </div>
          </form>
        )}
      </Modal>

      <Modal open={!!created} onClose={() => setCreated(null)} title="Your new API key" size="lg">
        <div className="space-y-4">
          <p className="flex items-start gap-2 rounded-lg bg-warning-bg p-3 text-sm text-warning"><TriangleAlert size={16} className="mt-0.5 shrink-0" />Copy it now. It is shown only once and cannot be recovered; if you lose it, revoke it and create a new one.</p>
          <pre className="select-all break-all whitespace-pre-wrap rounded-md bg-[#14130f] p-3 font-mono text-xs text-gray-200">{created}</pre>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={() => { navigator.clipboard?.writeText(created).then(() => toast.success('Copied')) }}><Copy size={14} />Copy</Button>
            <Button onClick={() => setCreated(null)}>Done</Button>
          </div>
        </div>
      </Modal>

      <ConfirmDialog open={!!rev} onClose={() => setRev(null)} loading={revoke.isPending} title={`Revoke ${rev?.name}`}
        message="Tools using this key stop working immediately. This cannot be undone." onConfirm={() => revoke.mutate(rev)} />
      <ConfirmDialog open={!!del} onClose={() => setDel(null)} loading={remove.isPending} title={`Delete ${del?.name}`}
        message="The key is removed and stops working immediately." onConfirm={() => remove.mutate(del)} />
    </div>
  )
}

export default ApiKeys
