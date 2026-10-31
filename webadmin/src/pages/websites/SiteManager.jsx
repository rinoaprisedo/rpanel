import { useEffect, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { Lock, ShieldOff, Plus, Trash2, Pencil, ArrowRight, ChevronDown } from 'lucide-react'
import Modal from '../../components/ui/Modal'
import Tabs from '../../components/ui/Tabs'
import Button from '../../components/ui/Button'
import Input from '../../components/ui/Input'
import Select from '../../components/ui/Select'
import Badge from '../../components/ui/Badge'
import Skeleton from '../../components/ui/Skeleton'
import CodeEditor from '../../components/ui/CodeEditor'
import useAuthStore from '../../store/authStore'
import { cn } from '../../utils/cn'
import * as api from '../../api/websites'
import SslProgress from './SslProgress'
import Switch from '../../components/ui/Switch'

const TABS = ['Config', 'SSL', 'Reverse proxy']
const errMsg = (e) => e.response?.data?.message || 'Operation failed'

// ---------------------------------------------------------------- Config
function ConfigTab({ site }) {
  const isSuper = useAuthStore((s) => !!s.user?.is_superadmin)
  const qc = useQueryClient()
  const { data, isLoading } = useQuery({ queryKey: ['site-config', site.uuid], queryFn: () => api.getSiteConfig(site.uuid) })
  const cfg = data?.data
  const [text, setText] = useState('')
  const [showFull, setShowFull] = useState(false)
  useEffect(() => { if (cfg) setText(cfg.extra_config || '') }, [cfg])

  const save = useMutation({
    mutationFn: () => api.saveSiteConfig(site.uuid, text),
    onSuccess: () => { toast.success('Config saved and nginx reloaded'); qc.invalidateQueries({ queryKey: ['site-config', site.uuid] }) },
    onError: (e) => toast.error(errMsg(e), { duration: 8000 }),
  })
  const dirty = (cfg?.extra_config || '') !== text

  if (isLoading) return <Skeleton className="h-64 w-full" />
  return (
    <div className="space-y-4">
      <div>
        <p className="mb-2 text-sm text-text-secondary">
          Extra nginx directives for <b className="text-text-primary">{site.domain}</b>, added inside its <code className="font-mono text-xs">server</code> blocks (HTTP and HTTPS) — e.g. <code className="font-mono text-xs">client_max_body_size 100m;</code>, rewrites or extra <code className="font-mono text-xs">location</code> blocks.
          Nginx validates every save; a config it rejects is rolled back.
        </p>
        <CodeEditor path={`/etc/nginx/${site.domain}.conf`} value={text} onChange={setText} onSave={() => isSuper && dirty && save.mutate()} readOnly={!isSuper} className="h-[30vh]" />
        {!isSuper && <p className="mt-2 text-xs text-warning">Only a superadmin can edit raw nginx directives.</p>}
      </div>
      <div className="flex items-center justify-between">
        <button type="button" className="inline-flex items-center gap-1 text-sm text-primary hover:underline" onClick={() => setShowFull((v) => !v)}>
          <ChevronDown size={14} className={cn('transition-transform', showFull && 'rotate-180')} />{showFull ? 'Hide' : 'Show'} the full generated config
        </button>
        <Button disabled={!isSuper || !dirty || save.isPending} onClick={() => save.mutate()}>{save.isPending ? 'Validating…' : 'Save & reload'}</Button>
      </div>
      {showFull && <CodeEditor path={`/etc/nginx/${site.domain}.conf`} value={cfg?.rendered || ''} onChange={() => {}} readOnly className="h-[30vh]" />}
    </div>
  )
}

// ---------------------------------------------------------------- SSL
function SSLTab({ site, onChanged }) {
  const qc = useQueryClient()
  const { data, isLoading } = useQuery({ queryKey: ['site-ssl', site.uuid], queryFn: () => api.getSiteSSL(site.uuid) })
  const [confirmOff, setConfirmOff] = useState(false)
  const [job, setJob] = useState(null)
  const st = data?.data

  const done = (msg) => () => { toast.success(msg); qc.invalidateQueries({ queryKey: ['site-ssl', site.uuid] }); onChanged() }
  const fail = (e) => toast.error(errMsg(e), { duration: 8000 })
  const issue = useMutation({
    mutationFn: () => api.startSSL(site.uuid, { force_https: !!st?.force_https || !st?.enabled }),
    onSuccess: () => setJob({ uuid: site.uuid, domain: site.domain }),
    onError: fail,
  })
  const finished = () => { qc.invalidateQueries({ queryKey: ['site-ssl', site.uuid] }); onChanged() }
  const force = useMutation({ mutationFn: (v) => api.updateWebsite(site.uuid, { force_https: v }), onSuccess: done('Updated'), onError: fail })
  const off = useMutation({ mutationFn: () => api.disableSSL(site.uuid), onSuccess: () => { setConfirmOff(false); done('SSL disabled')() }, onError: fail })

  if (isLoading) return <Skeleton className="h-40 w-full" />
  const cert = st?.cert
  const expiring = cert && cert.days_left < 14

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between rounded-lg border border-surface-border p-4">
        <div className="flex items-center gap-3">
          <div className={cn('flex h-10 w-10 items-center justify-center rounded-lg', st?.enabled ? 'bg-success-bg text-success' : 'bg-surface-hover text-text-tertiary')}>
            {st?.enabled ? <Lock size={20} /> : <ShieldOff size={20} />}
          </div>
          <div>
            <p className="text-sm font-semibold text-text-primary">{st?.enabled ? 'HTTPS is enabled' : 'HTTPS is not enabled'}</p>
            <p className="text-xs text-text-secondary">{site.domain}</p>
          </div>
        </div>
        <Badge variant={st?.enabled ? 'success' : 'neutral'}>{st?.enabled ? 'On' : 'Off'}</Badge>
      </div>

      {cert && (
        <dl className="grid grid-cols-[110px_1fr] gap-y-2 rounded-lg bg-surface-hover p-4 text-sm">
          <dt className="text-text-secondary">Issuer</dt><dd className="text-text-primary">{cert.issuer || '—'}</dd>
          <dt className="text-text-secondary">Covers</dt><dd className="break-all font-mono text-xs text-text-primary">{(cert.names || []).join(', ') || '—'}</dd>
          <dt className="text-text-secondary">Expires</dt>
          <dd className="text-text-primary">{new Date(cert.not_after).toLocaleDateString(undefined, { day: '2-digit', month: 'short', year: 'numeric' })} <Badge variant={expiring ? 'warning' : 'success'} className="ml-2">{cert.days_left} days left</Badge></dd>
        </dl>
      )}

      {st?.enabled ? (
        <div className="space-y-3">
          <label className="flex items-center justify-between rounded-lg border border-surface-border p-4">
            <span>
              <span className="block text-sm font-medium text-text-primary">Force HTTPS</span>
              <span className="text-xs text-text-secondary">Redirect every HTTP request to HTTPS.</span>
            </span>
            <Switch checked={!!st.force_https} disabled={force.isPending} onChange={(v) => force.mutate(v)} label="Force HTTPS" />
          </label>
          <div className="flex flex-wrap justify-end gap-2">
            <Button variant="secondary" disabled={issue.isPending} onClick={() => issue.mutate()}>{issue.isPending ? 'Starting…' : 'Renew certificate'}</Button>
            <Button variant="secondary" className="!text-danger" onClick={() => setConfirmOff(true)}><ShieldOff size={14} />Disable SSL</Button>
          </div>
          <p className="text-xs text-text-tertiary">Certificates renew automatically through certbot; renewing here is only needed after changes.</p>
        </div>
      ) : (
        <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); issue.mutate() }}>
          <p className="text-sm text-text-secondary">Get a free Let's Encrypt certificate. The domain must already point to this server and port 80 must be reachable from the internet.</p>
          <div className="flex justify-end">
            <Button type="submit" disabled={issue.isPending || site.status !== 'running'}>{issue.isPending ? 'Starting…' : 'Get certificate'}</Button>
          </div>
          {site.status !== 'running' && <p className="text-xs text-warning">Start the site first.</p>}
        </form>
      )}

      <SslProgress target={job} onClose={() => { setJob(null); finished() }} onFinished={finished} onRetry={() => { setJob(null); issue.mutate() }} />

      <Modal open={confirmOff} onClose={() => setConfirmOff(false)} title="Disable SSL?" size="sm">
        <p className="text-sm text-text-secondary">The site goes back to plain HTTP. The certificate stays on the server, so you can enable HTTPS again later.</p>
        <div className="mt-5 flex justify-end gap-2">
          <Button variant="secondary" onClick={() => setConfirmOff(false)}>Cancel</Button>
          <Button variant="danger" disabled={off.isPending} onClick={() => off.mutate()}>{off.isPending ? 'Disabling…' : 'Disable SSL'}</Button>
        </div>
      </Modal>
    </div>
  )
}

// ---------------------------------------------------------------- Reverse proxy
const emptyRule = { name: '', path: '/api/', target: 'http://127.0.0.1:3000', pass_host: true, enabled: true }

function TargetEditor({ site, onChanged }) {
  const [target, setTarget] = useState(site.proxy_target || '')
  useEffect(() => { setTarget(site.proxy_target || '') }, [site.proxy_target])
  const save = useMutation({
    mutationFn: () => api.updateWebsite(site.uuid, { proxy_target: target.trim() }),
    onSuccess: () => { toast.success('Target updated and nginx reloaded'); onChanged() },
    onError: (e) => toast.error(errMsg(e), { duration: 8000 }),
  })
  const dirty = target.trim() !== (site.proxy_target || '')
  return (
    <form onSubmit={(e) => { e.preventDefault(); if (dirty) save.mutate() }} className="space-y-2 rounded-md bg-surface-hover p-3">
      <p className="text-xs text-text-secondary">This site is a reverse proxy itself: everything not matched by a rule below goes to this target.</p>
      <div className="flex items-end gap-2">
        <div className="flex-1"><Input label="Proxy target" value={target} onChange={(e) => setTarget(e.target.value)} placeholder="http://127.0.0.1:3000" /></div>
        <Button type="submit" disabled={!dirty || !target.trim() || save.isPending}>{save.isPending ? 'Saving…' : 'Save'}</Button>
      </div>
    </form>
  )
}

function ProxyTab({ site, onChanged }) {
  const rules = site.proxy_rules || []
  const [form, setForm] = useState(null) // { index: number | null, rule }
  const save = useMutation({
    mutationFn: (next) => api.saveProxyRules(site.uuid, next),
    onSuccess: () => { toast.success('Saved and nginx reloaded'); setForm(null); onChanged() },
    onError: (e) => toast.error(errMsg(e), { duration: 8000 }),
  })
  const set = (k) => (e) => setForm({ ...form, rule: { ...form.rule, [k]: e.target.value } })
  const submit = (e) => {
    e.preventDefault()
    const next = [...rules]
    if (form.index === null) next.push(form.rule)
    else next[form.index] = form.rule
    save.mutate(next)
  }

  return (
    <div className="space-y-4">
      <p className="text-sm text-text-secondary">
        Forward part of <b className="text-text-primary">{site.domain}</b> to another service, e.g. <code className="font-mono text-xs">/api/</code> → a Node or Docker app. The rest of the site keeps working as before.
      </p>
      {site.type === 'proxy' && <TargetEditor site={site} onChanged={onChanged} />}

      <div className="overflow-hidden rounded-lg border border-surface-border">
        {rules.length === 0 ? (
          <p className="p-6 text-center text-sm text-text-secondary">No proxy rules yet</p>
        ) : (
          <ul className="divide-y divide-surface-border">
            {rules.map((r, i) => (
              <li key={r.path} className="flex items-center gap-3 p-3">
                <Switch checked={r.enabled} disabled={save.isPending} label={`Enable ${r.path}`} onChange={(v) => save.mutate(rules.map((x, j) => (j === i ? { ...x, enabled: v } : x)))} />
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2 font-mono text-[13px]">
                    <span className="text-text-primary">{r.path}</span><ArrowRight size={13} className="text-text-tertiary" /><span className="truncate text-text-secondary">{r.target}</span>
                  </div>
                  <p className="text-xs text-text-tertiary">{r.name || 'Unnamed'} · {r.pass_host ? 'keeps the visitor’s Host header' : 'sends the target’s Host header'}</p>
                </div>
                <Button variant="ghost" className="h-8 w-8 !px-0" title="Edit" onClick={() => setForm({ index: i, rule: { ...r } })}><Pencil size={14} /></Button>
                <Button variant="ghost" className="h-8 w-8 !px-0" title="Delete" disabled={save.isPending} onClick={() => save.mutate(rules.filter((_, j) => j !== i))}><Trash2 size={14} className="text-danger" /></Button>
              </li>
            ))}
          </ul>
        )}
      </div>

      {form ? (
        <form onSubmit={submit} className="space-y-3 rounded-lg border border-surface-border p-4">
          <h4 className="text-sm font-semibold text-text-primary">{form.index === null ? 'New rule' : 'Edit rule'}</h4>
          <div className="grid gap-3 sm:grid-cols-2">
            <Input label="Name (optional)" value={form.rule.name} onChange={set('name')} placeholder="API" />
            <Input label="Path" value={form.rule.path} onChange={set('path')} placeholder="/api/" />
          </div>
          <Input label="Target" value={form.rule.target} onChange={set('target')} placeholder="http://127.0.0.1:3000" />
          <Select label="Host header" value={form.rule.pass_host ? 'visitor' : 'target'} onChange={(e) => setForm({ ...form, rule: { ...form.rule, pass_host: e.target.value === 'visitor' } })}>
            <option value="visitor">Keep the visitor's host (apps on this server)</option>
            <option value="target">Use the target's host (external APIs)</option>
          </Select>
          <p className="text-xs text-text-tertiary">A target ending in <code className="font-mono">/</code> (e.g. <code className="font-mono">http://127.0.0.1:3000/</code>) strips the path prefix before forwarding.</p>
          <div className="flex justify-end gap-2">
            <Button type="button" variant="secondary" onClick={() => setForm(null)}>Cancel</Button>
            <Button type="submit" disabled={save.isPending || !form.rule.path || !form.rule.target}>{save.isPending ? 'Saving…' : 'Save rule'}</Button>
          </div>
        </form>
      ) : (
        <div className="flex justify-end"><Button onClick={() => setForm({ index: null, rule: { ...emptyRule } })}><Plus size={14} />Add rule</Button></div>
      )}
    </div>
  )
}

// ---------------------------------------------------------------- shell
function SiteManager({ site, tab, onTab, onClose, onChanged }) {
  return (
    <Modal open={!!site} onClose={onClose} title={site ? `Manage ${site.domain}` : ''} size="lg">
      {site && (
        <div className="space-y-4">
          <Tabs tabs={TABS} value={tab} onChange={onTab} />
          {tab === 'Config' && <ConfigTab site={site} />}
          {tab === 'SSL' && <SSLTab site={site} onChanged={onChanged} />}
          {tab === 'Reverse proxy' && <ProxyTab site={site} onChanged={onChanged} />}
        </div>
      )}
    </Modal>
  )
}

export default SiteManager
