import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { Plus, KeyRound, Copy, Ban, Trash2, TriangleAlert, Pencil, Folder, FolderSearch, X, ScrollText, Upload, Container, Terminal, Clock, Globe, ShieldCheck } from 'lucide-react'
import Button from '../../components/ui/Button'
import Input from '../../components/ui/Input'
import Badge from '../../components/ui/Badge'
import Modal from '../../components/ui/Modal'
import ConfirmDialog from '../../components/ui/ConfirmDialog'
import FolderBrowser from './FolderBrowser'
import Skeleton from '../../components/ui/Skeleton'
import EmptyState from '../../components/ui/EmptyState'
import usePermission from '../../hooks/usePermission'
import KeyLogs from './KeyLogs'
import { cn } from '../../utils/cn'
import { ago } from '../../utils/cron'
import * as api from '../../api/apiKeys'

const errMsg = (e) => e.response?.data?.message || 'Operation failed'
const SCOPES = {
  'deploy:upload': ['Upload files', 'Upload image archives and compose files into the deploy folders'],
  'deploy:docker': ['Docker deploy', 'docker load, edit compose files, compose up / down / logs'],
  'deploy:ssh': ['SSH access', 'Register a public key so the tool can open an SSH session'],
}
const SCOPE_ICON = { 'deploy:upload': Upload, 'deploy:docker': Container, 'deploy:ssh': Terminal }
const emptyForm = { name: '', scopes: ['deploy:upload', 'deploy:docker'], allowed_ips: '', allowed_dirs: [], expires_days: 0 }

function ApiKeys() {
  const qc = useQueryClient()
  const { canCreate, canEdit, canDelete } = usePermission()
  const { data, isLoading } = useQuery({ queryKey: ['api-keys'], queryFn: api.getKeys })
  const keys = data?.data?.keys || []
  const [form, setForm] = useState(null)
  const [picking, setPicking] = useState(false)
  const [created, setCreated] = useState(null)
  const [del, setDel] = useState(null)
  const [rev, setRev] = useState(null)
  const [logKey, setLogKey] = useState(null)
  const refresh = () => qc.invalidateQueries({ queryKey: ['api-keys'] })

  const update = useMutation({
    mutationFn: (f) => api.updateKey(f.uuid, { name: f.name, scopes: f.scopes, expires_at: f.expires_at, allowed_dirs: f.allowed_dirs, allowed_ips: f.allowed_ips.split(/[\s,]+/).filter(Boolean) }),
    onSuccess: () => { toast.success('Key updated'); setForm(null); refresh() },
    onError: (e) => toast.error(errMsg(e)),
  })
  const create = useMutation({
    mutationFn: (f) => api.createKey({ name: f.name, scopes: f.scopes, expires_days: Number(f.expires_days) || 0, allowed_dirs: f.allowed_dirs, allowed_ips: f.allowed_ips.split(/[\s,]+/).filter(Boolean) }),
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
        <p className="max-w-2xl text-sm text-text-secondary">Keys let an external tool such as Vibecoder upload image archives and run Docker deploys without signing in to this panel. A key only has the scopes you give it, and only works inside the deploy folders and the folders you choose for it.</p>
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
            const dead = variant !== 'success'
            return (
              <article key={k.uuid} className={cn('flex flex-col overflow-hidden rounded-xl border border-surface-border bg-surface-card', dead && 'opacity-75')}>
                <header className="flex items-start gap-3 p-4 pb-3">
                  <span className={cn('flex size-10 shrink-0 items-center justify-center rounded-lg', dead ? 'bg-surface-hover text-text-secondary' : 'bg-primary/10 text-primary')}><KeyRound size={18} /></span>
                  <div className="min-w-0 flex-1">
                    <h3 className="truncate text-sm font-semibold text-text-primary">{k.name}</h3>
                    <p className="font-mono text-xs text-text-secondary">{k.prefix}…</p>
                  </div>
                  <Badge variant={variant}>{label}</Badge>
                </header>

                <div className="flex flex-wrap gap-1.5 px-4">
                  {k.scopes.map((s) => {
                    const Icon = SCOPE_ICON[s] || ShieldCheck
                    return <span key={s} className="inline-flex items-center gap-1 rounded-md border border-surface-border bg-surface-hover/50 px-2 py-0.5 text-xs text-text-primary"><Icon size={12} className="text-primary" />{SCOPES[s]?.[0] || s}</span>
                  })}
                </div>

                <dl className="mt-3 space-y-2 px-4 pb-4 text-xs">
                  <Row icon={Clock} label="Last used">{k.last_used_at ? <><span className="text-text-primary">{ago(k.last_used_at)}</span>{k.last_used_ip && <span className="text-text-secondary"> · {k.last_used_ip}</span>}</> : <span className="text-text-secondary">Never</span>}</Row>
                  <Row icon={Folder} label="Folders">{k.allowed_dirs?.length ? <span className="break-all font-mono text-text-primary">{k.allowed_dirs.join(', ')}</span> : <span className="text-text-secondary">Deploy folders only</span>}</Row>
                  <Row icon={Globe} label="Allowed IPs"><span className={cn('break-all', k.allowed_ips ? 'text-text-primary' : 'text-text-secondary')}>{k.allowed_ips ? k.allowed_ips.split(',').join(', ') : 'Any'}</span></Row>
                  <Row icon={ShieldCheck} label="Expires"><span className={k.expires_at ? 'text-text-primary' : 'text-text-secondary'}>{k.expires_at ? new Date(k.expires_at).toLocaleDateString() : 'Never'}</span></Row>
                </dl>

                <footer className="mt-auto flex items-center justify-between gap-2 border-t border-surface-border bg-surface-hover/30 px-3 py-2">
                  <Button variant="ghost" className="h-8 gap-1.5 px-2 text-xs" onClick={() => setLogKey(k)}><ScrollText size={14} />Activity</Button>
                  <div className="flex gap-0.5">
                    {canEdit('api-keys') && !k.revoked_at && <Button variant="ghost" className="h-8 px-2" title="Edit" onClick={() => setForm({ uuid: k.uuid, name: k.name, scopes: k.scopes, allowed_ips: k.allowed_ips || '', allowed_dirs: k.allowed_dirs || [], expires_at: k.expires_at ? k.expires_at.slice(0, 10) : '' })}><Pencil size={14} /></Button>}
                    {canEdit('api-keys') && !k.revoked_at && <Button variant="ghost" className="h-8 px-2" title="Revoke" onClick={() => setRev(k)}><Ban size={14} /></Button>}
                    {canDelete('api-keys') && <Button variant="ghost" className="h-8 px-2" title="Delete" onClick={() => setDel(k)}><Trash2 size={14} className="text-danger" /></Button>}
                  </div>
                </footer>
              </article>
            )
          })}
        </div>
      )}

      <Modal open={!!form} onClose={() => setForm(null)} title={form?.uuid ? 'Edit API key' : 'New API key'} size="lg">
        {form && (
          <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); (form.uuid ? update : create).mutate(form) }}>
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
            <div className="space-y-2">
              <label className="text-xs font-medium uppercase tracking-[.05em] text-text-secondary">Folders this key can use</label>
              <p className="text-xs text-text-secondary">Besides the deploy folders, the key can upload to, load from and run compose in these folders only.</p>
              {form.allowed_dirs.length > 0 && (
                <ul className="space-y-1.5">
                  {form.allowed_dirs.map((d) => (
                    <li key={d} className="flex items-center gap-2 rounded-md border border-surface-border px-3 py-1.5 text-sm">
                      <Folder size={14} className="shrink-0 text-amber-500" />
                      <span className="min-w-0 flex-1 truncate font-mono text-xs text-text-primary" title={d}>{d}</span>
                      <button type="button" aria-label={`Remove ${d}`} className="text-text-tertiary hover:text-danger" onClick={() => setForm({ ...form, allowed_dirs: form.allowed_dirs.filter((x) => x !== d) })}><X size={14} /></button>
                    </li>
                  ))}
                </ul>
              )}
              <Button type="button" variant="secondary" onClick={() => setPicking(true)} disabled={form.allowed_dirs.length >= 10}><FolderSearch size={14} />Add folder</Button>
            </div>
            <Input label="Allowed IPs (optional)" placeholder="203.0.113.7, 10.0.0.0/24" value={form.allowed_ips} onChange={(e) => setForm({ ...form, allowed_ips: e.target.value })} />
            {form.uuid
              ? <Input label="Expires on (empty = never)" type="date" value={form.expires_at} onChange={(e) => setForm({ ...form, expires_at: e.target.value })} />
              : <Input label="Expires after (days, 0 = never)" type="number" value={form.expires_days} onChange={(e) => setForm({ ...form, expires_days: e.target.value })} />}
            <div className="flex justify-end gap-2">
              <Button type="button" variant="secondary" onClick={() => setForm(null)}>Cancel</Button>
              <Button type="submit" disabled={create.isPending || update.isPending || !form.name.trim() || !form.scopes.length}>{form.uuid ? (update.isPending ? 'Saving…' : 'Save') : create.isPending ? 'Creating…' : 'Create key'}</Button>
            </div>
          </form>
        )}
      </Modal>

      <KeyLogs keyItem={logKey} onClose={() => setLogKey(null)} />

      <FolderBrowser open={picking} onClose={() => setPicking(false)} onSelect={(p) => { setForm((f) => (f.allowed_dirs.includes(p) ? f : { ...f, allowed_dirs: [...f.allowed_dirs, p] })); setPicking(false) }} />

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

function Row({ icon: Icon, label, children }) {
  return (
    <div className="flex items-start gap-2">
      <Icon size={13} className="mt-0.5 shrink-0 text-text-tertiary" />
      <dt className="w-20 shrink-0 text-text-secondary">{label}</dt>
      <dd className="min-w-0 flex-1">{children}</dd>
    </div>
  )
}

export default ApiKeys
