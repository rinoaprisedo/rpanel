import { useMemo, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { Plus, Globe, FolderSearch, Search, X, CircleCheck, CircleX, Lock, TriangleAlert } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import Button from '../../components/ui/Button'
import Input from '../../components/ui/Input'
import Select from '../../components/ui/Select'
import Modal from '../../components/ui/Modal'
import ConfirmDialog from '../../components/ui/ConfirmDialog'
import Skeleton from '../../components/ui/Skeleton'
import FolderPicker from './FolderPicker'
import SiteManager from './SiteManager'
import SiteCard, { TYPES } from './SiteCard'
import { cn } from '../../utils/cn'
import { getTraffic, getInsights } from '../../api/dashboard'
import * as api from '../../api/websites'

const errMsg = (e) => e.response?.data?.message || 'Operation failed'
const emptyForm = { domain: '', type: 'static', root_dir: '', php_version: '', proxy_target: '' }

function Websites() {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const { data, isLoading } = useQuery({ queryKey: ['websites'], queryFn: api.getWebsites })
  const { data: envRes } = useQuery({ queryKey: ['websites-env'], queryFn: api.getWebsiteEnv })
  const env = envRes?.data
  const sites = useMemo(() => data?.data || [], [data])
  // Traffic and certificate days come from the dashboard API; the page works without them.
  const { data: trafficRes } = useQuery({ queryKey: ['dash-traffic', '24h', ''], queryFn: () => getTraffic('24h', ''), retry: false, refetchInterval: 60000 })
  const { data: insRes } = useQuery({ queryKey: ['dash-insights'], queryFn: getInsights, retry: false, refetchInterval: 30000 })
  const statsBy = useMemo(() => Object.fromEntries((trafficRes?.data?.sites || []).map((x) => [x.domain, x])), [trafficRes])
  const daysBy = useMemo(() => Object.fromEntries((insRes?.data?.sites || []).map((x) => [x.domain, x.days_left])), [insRes])
  const [q, setQ] = useState('')
  const [filter, setFilter] = useState('all')

  const [form, setForm] = useState(null)
  const [picking, setPicking] = useState(false)
  const [del, setDel] = useState(null)
  const [deleteFiles, setDeleteFiles] = useState(false)
  const [logs, setLogs] = useState(null) // { site, type, text }
  const [manage, setManage] = useState(null) // { uuid, tab }

  const refresh = () => qc.invalidateQueries({ queryKey: ['websites'] })
  const act = (fn, ok, after) => ({
    mutationFn: fn,
    onSuccess: () => { toast.success(ok); refresh(); after?.() },
    onError: (e) => toast.error(errMsg(e)),
  })
  const create = useMutation(act((d) => api.createWebsite(d), 'Website created', () => setForm(null)))
  const toggle = useMutation(act((s) => (s.status === 'running' ? api.stopWebsite(s.uuid) : api.startWebsite(s.uuid)), 'Updated'))
  const remove = useMutation(act((s) => api.deleteWebsite(s.uuid, deleteFiles), 'Website deleted', () => setDel(null)))
  const install = useMutation({
    mutationFn: ({ component, version }) => api.installComponent(component, version),
    onSuccess: () => { toast.success('Installed'); qc.invalidateQueries({ queryKey: ['websites-env'] }) },
    onError: (e) => toast.error(errMsg(e)),
  })

  const openLogs = async (site, type) => {
    try {
      const r = await api.getWebsiteLogs(site.uuid, type)
      setLogs({ site, type, text: r.data.log })
    } catch (e) {
      toast.error(errMsg(e))
    }
  }
  const set = (k) => (e) => setForm({ ...form, [k]: e.target.value })

  const submit = (e) => {
    e.preventDefault()
    const body = { domain: form.domain, type: form.type, root_dir: form.root_dir }
    if (form.type === 'php') body.php_version = form.php_version
    if (form.type === 'proxy') body.proxy_target = form.proxy_target
    create.mutate(body)
  }

  const counts = {
    all: sites.length,
    running: sites.filter((s) => s.status === 'running').length,
    stopped: sites.filter((s) => s.status !== 'running').length,
    nohttps: sites.filter((s) => !s.ssl_enabled).length,
  }
  const attention = sites.filter((s) => s.status === 'running' && ((s.ssl_enabled && daysBy[s.domain] != null && daysBy[s.domain] < 14) || statsBy[s.domain]?.server_err > 0)).length
  const shown = sites.filter((s) => {
    if (filter === 'running' && s.status !== 'running') return false
    if (filter === 'stopped' && s.status === 'running') return false
    if (filter === 'nohttps' && s.ssl_enabled) return false
    if (['static', 'php', 'proxy'].includes(filter) && s.type !== filter) return false
    return !q.trim() || s.domain.includes(q.trim().toLowerCase())
  })

  const Chip = ({ ok, children }) => (
    <span className="inline-flex items-center gap-1.5 text-xs text-text-secondary">
      {ok ? <CircleCheck size={13} className="text-success" /> : <CircleX size={13} className="text-danger" />}{children}
    </span>
  )

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-lg font-semibold text-text-primary">Websites</h1>
          <div className="mt-1 flex flex-wrap items-center gap-x-4 gap-y-1">
            <Chip ok={env?.nginx}>Nginx {env?.nginx ? (env.nginx_version || '').replace('nginx/', '') : 'missing'}</Chip>
            <Chip ok={env?.certbot}>Certbot {env?.certbot ? '' : 'missing'}</Chip>
            <span className="inline-flex items-center gap-1.5 text-xs text-text-secondary">PHP {env?.php_versions?.length ? env.php_versions.join(', ') : <button onClick={() => navigate('/app-store')} className="text-primary hover:underline">not installed — App Store</button>}</span>
            {env && !env.nginx && <button disabled={install.isPending} onClick={() => install.mutate({ component: 'nginx' })} className="text-xs font-medium text-primary hover:underline">Install Nginx</button>}
            {env && !env.certbot && <button disabled={install.isPending} onClick={() => install.mutate({ component: 'certbot' })} className="text-xs font-medium text-primary hover:underline">Install Certbot</button>}
          </div>
        </div>
        <Button onClick={() => setForm({ ...emptyForm, php_version: env?.php_versions?.[0] || '' })}><Plus size={14} />Add website</Button>
      </div>
      {install.isPending && <p className="text-sm text-text-secondary">Installing via apt, this can take a few minutes…</p>}

      {sites.length > 0 && (
        <>
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
            {[
              { k: 'all', label: 'Websites', value: counts.all, icon: Globe },
              { k: 'running', label: 'Running', value: counts.running, icon: CircleCheck, tone: 'text-success' },
              { k: 'nohttps', label: 'Without HTTPS', value: counts.nohttps, icon: Lock, tone: counts.nohttps ? 'text-warning' : '' },
              { k: 'attention', label: 'Need attention', value: attention, icon: TriangleAlert, tone: attention ? 'text-danger' : '' },
            ].map((c) => (
              <button key={c.k} onClick={() => c.k !== 'attention' && setFilter(filter === c.k ? 'all' : c.k)} className={cn('flex items-center gap-3 rounded-lg border bg-surface-card p-3.5 text-left transition-colors', filter === c.k ? 'border-primary' : 'border-surface-border hover:border-surface-border-hover')}>
                <span className={cn('flex h-9 w-9 items-center justify-center rounded-md bg-surface-hover text-text-secondary', c.tone)}><c.icon size={17} /></span>
                <span><span className="block text-xl font-semibold leading-none text-text-primary">{c.value}</span><span className="text-xs text-text-tertiary">{c.label}</span></span>
              </button>
            ))}
          </div>

          <div className="flex flex-wrap items-center gap-3">
            <div className="relative w-full max-w-xs">
              <Search size={15} className="absolute left-3 top-1/2 -translate-y-1/2 text-text-tertiary" />
              <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Search domain…" className="h-[38px] w-full rounded-md border-[1.5px] border-surface-border bg-field pl-9 pr-8 text-sm text-text-primary outline-none transition-colors focus:border-primary" />
              {q && <button onClick={() => setQ('')} aria-label="Clear" className="absolute right-2 top-1/2 -translate-y-1/2 text-text-tertiary hover:text-text-primary"><X size={14} /></button>}
            </div>
            <div className="inline-flex flex-wrap rounded-md border border-surface-border bg-surface-card p-0.5">
              {[['all', 'All'], ['running', 'Running'], ['stopped', 'Stopped'], ['static', 'Static'], ['php', 'PHP'], ['proxy', 'Proxy']].map(([k, label]) => (
                <button key={k} onClick={() => setFilter(k)} aria-pressed={filter === k} className={cn('h-8 rounded-[8px] px-3 text-[13px] font-medium transition-colors', filter === k ? 'bg-primary text-white' : 'text-text-secondary hover:bg-surface-hover')}>{label}</button>
              ))}
            </div>
          </div>
        </>
      )}

      {isLoading ? (
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">{Array.from({ length: 3 }, (_, i) => <Skeleton key={i} className="h-48 w-full" />)}</div>
      ) : sites.length === 0 ? (
        <div className="rounded-lg border border-dashed border-surface-border-hover px-6 py-14 text-center">
          <span className="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-primary/10 text-primary"><Globe size={22} /></span>
          <p className="mt-3 text-sm font-semibold text-text-primary">No websites yet</p>
          <p className="mx-auto mt-1 max-w-sm text-xs leading-5 text-text-secondary">Host a static site, a PHP app or put a reverse proxy in front of a service. Nginx config and HTTPS are handled for you.</p>
          <Button className="mt-4" onClick={() => setForm({ ...emptyForm, php_version: env?.php_versions?.[0] || '' })}><Plus size={14} />Add your first website</Button>
        </div>
      ) : shown.length === 0 ? (
        <p className="rounded-lg border border-surface-border bg-surface-card py-12 text-center text-sm text-text-secondary">No website matches your filter.</p>
      ) : (
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {shown.map((s) => (
            <SiteCard
              key={s.uuid} site={s} stats={statsBy[s.domain]} daysLeft={s.ssl_enabled ? daysBy[s.domain] : null} busy={toggle.isPending}
              onManage={() => setManage({ uuid: s.uuid, tab: 'Config' })}
              onToggle={() => toggle.mutate(s)}
              onLogs={() => openLogs(s, 'error')}
              onDelete={() => { setDeleteFiles(false); setDel(s) }}
              onFiles={() => navigate('/file-manager')}
            />
          ))}
        </div>
      )}

      <Modal open={!!form} onClose={() => setForm(null)} title="Add website">
        {form && (
          <form onSubmit={submit} className="space-y-4">
            <Input label="Domain" placeholder="example.com" value={form.domain} onChange={set('domain')} autoFocus />
            <div>
              <span className="mb-1 block text-xs font-medium uppercase tracking-[.05em] text-text-secondary">Type</span>
              <div className="grid grid-cols-3 gap-2">
                {Object.entries(TYPES).map(([k, T]) => (
                  <button type="button" key={k} onClick={() => setForm({ ...form, type: k })} aria-pressed={form.type === k} className={cn('flex flex-col items-start gap-1.5 rounded-lg border-[1.5px] p-3 text-left transition-colors', form.type === k ? 'border-primary bg-primary/5' : 'border-surface-border hover:border-surface-border-hover')}>
                    <span className="flex h-8 w-8 items-center justify-center rounded-md" style={{ backgroundColor: `${T.color}22`, color: T.color }}><T.icon size={16} /></span>
                    <span className="text-[13px] font-semibold text-text-primary">{T.label}</span>
                    <span className="text-[11px] leading-4 text-text-tertiary">{T.desc}</span>
                  </button>
                ))}
              </div>
            </div>
            {form.type !== 'proxy' && (
              <div className="flex items-end gap-2">
                <div className="min-w-0 flex-1"><Input label="Root directory (optional)" placeholder="default: /www/wwwroot/<domain>" value={form.root_dir} onChange={set('root_dir')} /></div>
                <Button type="button" variant="secondary" className="h-[42px]" onClick={() => setPicking(true)}><FolderSearch size={15} />Browse</Button>
              </div>
            )}
            {form.type === 'php' && (
              env?.php_versions?.length
                ? <Select label="PHP version" value={form.php_version} onChange={set('php_version')}>{env.php_versions.map((v) => <option key={v} value={v}>{v}</option>)}</Select>
                : <p className="rounded-md bg-warning-bg p-3 text-sm text-warning">No PHP is installed yet. Install a version from the App Store first.</p>
            )}
            {form.type === 'proxy' && <Input label="Proxy target" placeholder="http://127.0.0.1:3000" value={form.proxy_target} onChange={set('proxy_target')} />}
            <div className="flex justify-end gap-2">
              <Button type="button" variant="secondary" onClick={() => setForm(null)}>Cancel</Button>
              <Button type="submit" disabled={create.isPending || !form.domain || (form.type === 'php' && !form.php_version)}>{create.isPending ? 'Creating…' : 'Create'}</Button>
            </div>
          </form>
        )}
      </Modal>

      <FolderPicker open={picking} initial={form?.root_dir} onClose={() => setPicking(false)} onSelect={(p) => { setForm((f) => ({ ...f, root_dir: p })); setPicking(false) }} />

      <SiteManager
        site={sites.find((x) => x.uuid === manage?.uuid) || null}
        tab={manage?.tab}
        onTab={(tab) => setManage((m) => ({ ...m, tab }))}
        onClose={() => setManage(null)}
        onChanged={refresh}
      />

      <Modal open={!!logs} onClose={() => setLogs(null)} title={`${logs?.site.domain} — ${logs?.type} log`} size="xl">
        {logs && (
          <div className="space-y-3">
            <div className="flex gap-2">
              <Button variant={logs.type === 'error' ? 'primary' : 'secondary'} onClick={() => openLogs(logs.site, 'error')}>Error</Button>
              <Button variant={logs.type === 'access' ? 'primary' : 'secondary'} onClick={() => openLogs(logs.site, 'access')}>Access</Button>
              <Button variant="secondary" onClick={() => openLogs(logs.site, logs.type)}>Refresh</Button>
            </div>
            <pre className="h-[55vh] overflow-auto whitespace-pre-wrap break-all rounded-md bg-[#1e1f22] p-3 font-mono text-xs text-[#d4d4d4]">{logs.text || '(empty)'}</pre>
          </div>
        )}
      </Modal>

      <ConfirmDialog
        open={!!del}
        onClose={() => setDel(null)}
        loading={remove.isPending}
        title={`Delete ${del?.domain}`}
        message={
          <span className="block space-y-3">
            <span className="block">The nginx config is removed and the site stops serving.</span>
            <label className="flex items-center gap-2"><input type="checkbox" checked={deleteFiles} onChange={(e) => setDeleteFiles(e.target.checked)} /> Also delete site files ({del?.root_dir || 'n/a'})</label>
          </span>
        }
        onConfirm={() => remove.mutate(del)}
      />
    </div>
  )
}

export default Websites
