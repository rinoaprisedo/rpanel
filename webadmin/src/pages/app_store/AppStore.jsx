import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import toast from 'react-hot-toast'
import * as Icons from 'lucide-react'
import { Search, Download, Trash2, Lock, LoaderCircle, AlertTriangle, Store, ArrowRight, Plus, ShieldCheck, Play, Square, RotateCw } from 'lucide-react'
import Button from '../../components/ui/Button'
import Badge from '../../components/ui/Badge'
import Modal from '../../components/ui/Modal'
import Select from '../../components/ui/Select'
import EmptyState from '../../components/ui/EmptyState'
import Skeleton from '../../components/ui/Skeleton'
import { cn } from '../../utils/cn'
import * as api from '../../api/appStore'

const errMsg = (e) => e.response?.data?.message || 'Operation failed'
const busy = (a) => a.state === 'installing' || a.state === 'removing'
const STATUS = [['all', 'All'], ['installed', 'Installed'], ['available', 'Available']]

function AppIcon({ app }) {
  const Icon = Icons[app.icon] || Icons.Package
  return (
    <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg" style={{ backgroundColor: `${app.color}1f`, color: app.color }}>
      <Icon size={20} strokeWidth={1.7} />
    </div>
  )
}

function StatusBadge({ app }) {
  if (app.state === 'installing') return <Badge variant="primary">Installing…</Badge>
  if (app.state === 'removing') return <Badge variant="warning">Removing…</Badge>
  if (app.state === 'failed') return <Badge variant="danger">Failed</Badge>
  if (!app.installed) return <Badge>Not installed</Badge>
  if (!app.has_service) return <Badge variant="success">Installed</Badge>
  return app.running ? <Badge variant="success">Running</Badge> : <Badge variant="warning">Stopped</Badge>
}

function AppCard({ app, onInstall, onRemove, onControl, onOpenDatabases, pending }) {
  const working = busy(app)
  const versioned = !!app.versions?.length
  const version = app.installed ? app.version : app.candidate
  return (
    <div className="relative flex flex-col overflow-hidden rounded-lg border border-surface-border bg-surface-card p-3.5 shadow-card transition-shadow hover:shadow-md">
      {working && <div className="absolute inset-x-0 top-0 h-0.5 animate-pulse bg-primary" />}
      <div className="flex items-center gap-3">
        <AppIcon app={app} />
        <div className="min-w-0 flex-1">
          <h3 className="truncate text-sm font-semibold text-text-primary">{app.name}</h3>
          <div className="flex items-center gap-1.5 text-xs text-text-tertiary">
            <span className="truncate">{app.category}</span>
            {!versioned && version && <span className="shrink-0 rounded bg-surface-hover px-1.5 font-mono text-[11px] text-text-secondary">{app.installed ? '' : 'avail. '}v{version}</span>}
            {versioned && !app.installed && <span className="shrink-0 rounded bg-surface-hover px-1.5 text-[11px] text-text-secondary">{app.versions.length} versions</span>}
            {versioned && app.installed && !app.multi && version && <span className="shrink-0 rounded bg-surface-hover px-1.5 font-mono text-[11px] text-text-secondary">v{version}</span>}
          </div>
        </div>
        <StatusBadge app={app} />
      </div>

      <p className="mt-2.5 line-clamp-2 min-h-[2.5rem] text-xs leading-5 text-text-secondary">{app.description}</p>

      {app.multi && app.installed && (
        <div className="mt-2 flex flex-wrap gap-1.5">
          {app.versions.filter((v) => v.installed).map((v) => (
            <span key={v.label} className="inline-flex items-center gap-1 rounded-full bg-surface-hover py-0.5 pl-2 pr-0.5 font-mono text-[11px] text-text-secondary">
              <span className={cn('h-1.5 w-1.5 rounded-full', v.running ? 'bg-success' : 'bg-text-tertiary')} />{v.label}
              {!working && (
                <button
                  type="button"
                  disabled={pending}
                  title={v.running ? `Stop PHP ${v.label}` : `Start PHP ${v.label}`}
                  onClick={() => onControl(app, v.running ? 'stop' : 'start', v.label)}
                  className="flex h-5 w-5 items-center justify-center rounded-full text-text-secondary transition-colors hover:bg-surface-border disabled:opacity-50"
                >
                  {v.running ? <Square size={10} /> : <Play size={10} />}
                </button>
              )}
            </span>
          ))}
        </div>
      )}

      {app.state === 'failed' && (
        <div className="mt-2 flex gap-2 rounded-md bg-danger-bg p-2 text-xs text-danger">
          <AlertTriangle size={13} className="mt-px shrink-0" />
          <span className="line-clamp-3 whitespace-pre-wrap break-words">{app.error || 'Operation failed'}</span>
        </div>
      )}

      <div className="mt-3 flex min-h-8 items-center justify-between gap-2">
        {app.required ? (
          <span className="inline-flex items-center gap-1.5 text-xs text-text-tertiary"><Lock size={12} />Required by RPanel</span>
        ) : working ? (
          <span className="text-xs text-text-tertiary">May take a few minutes</span>
        ) : <span />}

        <div className="flex items-center gap-1">
          {app.id === 'mariadb' && app.installed && !working && (
            <Button variant="ghost" className="h-8 px-2 text-xs" onClick={onOpenDatabases}>Databases<ArrowRight size={12} /></Button>
          )}
          {working ? (
            <Button variant="secondary" className="h-8 px-3 text-xs" disabled><LoaderCircle size={13} className="animate-spin" />{app.state === 'installing' ? 'Installing' : 'Removing'}</Button>
          ) : !app.installed ? (
            <Button className="h-8 px-3 text-xs" disabled={pending} onClick={() => onInstall(app)}><Download size={13} />{app.state === 'failed' ? 'Retry' : 'Install'}</Button>
          ) : (
            <>
              {app.has_service && !app.multi && !app.required && (
                app.running ? (
                  <>
                    <Button variant="ghost" className="h-8 w-8 px-0" title="Restart" disabled={pending} onClick={() => onControl(app, 'restart')}><RotateCw size={14} /></Button>
                    <Button variant="ghost" className="h-8 w-8 px-0" title="Stop" disabled={pending} onClick={() => onControl(app, 'stop')}><Square size={14} /></Button>
                  </>
                ) : (
                  <Button variant="secondary" className="h-8 px-2.5 text-xs" disabled={pending} onClick={() => onControl(app, 'start')}><Play size={13} />Start</Button>
                )
              )}
              {app.multi && <Button variant="secondary" className="h-8 px-2.5 text-xs" disabled={pending} onClick={() => onInstall(app)}><Plus size={13} />Add version</Button>}
              {!app.required && <Button variant="ghost" className="h-8 px-2.5 text-xs !text-danger" disabled={pending} onClick={() => onRemove(app)}><Trash2 size={13} />Uninstall</Button>}
            </>
          )}
        </div>
      </div>
    </div>
  )
}

function Chips({ items }) {
  return <span className="flex flex-wrap gap-1">{items.map((p) => <span key={p} className="rounded bg-surface-hover px-1.5 py-0.5 font-mono text-[11px] text-text-secondary">{p}</span>)}</span>
}

function InstallDialog({ app, loading, onClose, onConfirm }) {
  const options = useMemo(() => (app?.versions || []).filter((v) => !v.installed), [app])
  const [label, setLabel] = useState('')
  useEffect(() => {
    if (!app) return
    const first = options.find((v) => v.source === 'distro') || options[0]
    setLabel(first?.label || '')
  }, [app, options])
  if (!app) return null

  const versioned = !!app.versions?.length
  const chosen = options.find((v) => v.label === label)
  const version = versioned ? chosen?.label : app.candidate
  const viaRepo = chosen?.source === 'repo'
  const packages = versioned ? chosen?.packages || [] : app.packages

  return (
    <Modal open onClose={onClose} title={`Install ${app.name}${versioned && label ? ` ${label}` : ''}?`}>
      <div className="space-y-4">
        {versioned && (
          <Select label="Version" value={label} onChange={(e) => setLabel(e.target.value)}>
            {options.map((v) => <option key={v.label} value={v.label}>{app.name} {v.label}{v.source === 'distro' ? ' — from the Ubuntu/Debian repository' : ` — from ${v.repo}`}</option>)}
          </Select>
        )}
        <div className="flex items-center justify-between rounded-md bg-surface-hover px-3 py-2 text-sm">
          <span className="text-text-secondary">Version to install</span>
          <span className="font-mono font-medium text-text-primary">{version ? `v${version}` : 'latest in the repository'}</span>
        </div>
        {viaRepo ? (
          <div className="flex gap-2 rounded-md border border-warning/30 bg-warning-bg p-3 text-xs leading-5 text-warning">
            <ShieldCheck size={15} className="mt-0.5 shrink-0" />
            <span>This version is not in your server's own repository. RPanel will add the <b>{chosen.repo}</b> apt repository and install from it. Its signing key is downloaded over HTTPS and only accepted if it matches the vendor's published fingerprint.</span>
          </div>
        ) : (
          <p className="text-xs leading-5 text-text-secondary">Installed from your server's own package repository (apt).</p>
        )}
        <div>
          <span className="mb-1 block text-xs font-medium uppercase tracking-wide text-text-tertiary">Packages</span>
          <Chips items={packages} />
        </div>
        <p className="text-xs leading-5 text-text-secondary">This can take a few minutes{app.has_service ? ', and the service is started and enabled at boot' : ''}. You can leave this page while it runs.</p>
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onClose} disabled={loading}>Cancel</Button>
          <Button onClick={() => onConfirm(app, versioned ? label : '')} disabled={loading || (versioned && !label)}>{loading ? 'Starting…' : 'Install'}</Button>
        </div>
      </div>
    </Modal>
  )
}

function RemoveDialog({ app, loading, onClose, onConfirm }) {
  const installed = useMemo(() => (app?.versions || []).filter((v) => v.installed), [app])
  const [label, setLabel] = useState('')
  useEffect(() => { setLabel(installed[0]?.label || '') }, [installed])
  if (!app) return null
  const pick = app.multi

  return (
    <Modal open onClose={onClose} title={`Uninstall ${app.name}${pick && label ? ` ${label}` : ''}`} size="sm">
      <div className="space-y-4">
        {pick && (
          <Select label="Version" value={label} onChange={(e) => setLabel(e.target.value)}>
            {installed.map((v) => <option key={v.label} value={v.label}>{app.name} {v.label}</option>)}
          </Select>
        )}
        <p className="text-sm text-text-secondary">The packages are removed but configuration and data are kept, so reinstalling restores your setup. Websites or apps that depend on it will stop working.</p>
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onClose} disabled={loading}>Cancel</Button>
          <Button variant="danger" onClick={() => onConfirm(app, pick ? label : '')} disabled={loading || (pick && !label)}>{loading ? 'Starting…' : 'Uninstall'}</Button>
        </div>
      </div>
    </Modal>
  )
}

function AppStore() {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [q, setQ] = useState('')
  const [status, setStatus] = useState('all')
  const [category, setCategory] = useState('All')
  const [removing, setRemoving] = useState(null)
  const [installing, setInstalling] = useState(null)
  const prev = useRef({})

  const { data, isLoading, isError } = useQuery({
    queryKey: ['app-store'],
    queryFn: api.getApps,
    refetchInterval: (query) => ((query.state.data?.data || []).some(busy) ? 3000 : false),
  })
  const apps = useMemo(() => data?.data || [], [data])

  // Installs run in the background on the server, so report their outcome when the state flips.
  useEffect(() => {
    apps.forEach((a) => {
      const was = prev.current[a.id]
      if (was && (was === 'installing' || was === 'removing') && !busy(a)) {
        if (a.state === 'failed') toast.error(`${a.name}: ${a.error || 'failed'}`)
        else toast.success(was === 'installing' ? `${a.name} installed` : `${a.name} uninstalled`)
      }
      prev.current[a.id] = a.state
    })
  }, [apps])

  const refresh = () => qc.invalidateQueries({ queryKey: ['app-store'] })
  const install = useMutation({
    mutationFn: ({ app, version }) => api.installApp(app.id, version),
    onSuccess: () => { setInstalling(null); refresh() },
    onError: (e) => { toast.error(errMsg(e)); setInstalling(null); refresh() },
  })
  const control = useMutation({
    mutationFn: ({ app, action, version }) => api.controlApp(app.id, action, version),
    onSuccess: (_, { app, action, version }) => { toast.success(`${app.name}${version ? ` ${version}` : ''} ${{ start: 'started', stop: 'stopped', restart: 'restarted' }[action]}`); refresh() },
    onError: (e) => { toast.error(errMsg(e)); refresh() },
  })
  const remove = useMutation({
    mutationFn: ({ app, version }) => api.uninstallApp(app.id, version),
    onSuccess: () => { setRemoving(null); refresh() },
    onError: (e) => { toast.error(errMsg(e)); setRemoving(null); refresh() },
  })

  const categories = useMemo(() => ['All', ...new Set(apps.map((a) => a.category))], [apps])
  const counts = useMemo(() => {
    const c = { All: apps.length }
    apps.forEach((a) => { c[a.category] = (c[a.category] || 0) + 1 })
    return c
  }, [apps])

  const shown = apps.filter((a) => {
    if (category !== 'All' && a.category !== category) return false
    if (status === 'installed' && !a.installed) return false
    if (status === 'available' && a.installed) return false
    const s = q.trim().toLowerCase()
    return !s || a.name.toLowerCase().includes(s) || a.description.toLowerCase().includes(s) || a.category.toLowerCase().includes(s)
  })
  const installedCount = apps.filter((a) => a.installed).length
  const runningCount = apps.filter((a) => a.installed && a.has_service && a.running).length

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-4 rounded-lg border border-surface-border bg-surface-card p-4">
        <div className="flex items-center gap-3">
          <div className="flex h-11 w-11 items-center justify-center rounded-lg bg-primary/10 text-primary"><Store size={22} strokeWidth={1.7} /></div>
          <div>
            <h2 className="text-base font-semibold text-text-primary">App Store</h2>
            <p className="text-xs text-text-secondary">Install the services your server needs with one click.</p>
          </div>
        </div>
        <div className="flex gap-6 text-center">
          <div><p className="text-xl font-semibold text-text-primary">{installedCount}<span className="text-sm font-normal text-text-tertiary"> / {apps.length}</span></p><p className="text-[11px] uppercase tracking-wide text-text-tertiary">Installed</p></div>
          <div><p className="text-xl font-semibold text-success">{runningCount}</p><p className="text-[11px] uppercase tracking-wide text-text-tertiary">Running</p></div>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <div className="relative w-full max-w-xs">
          <Search size={15} className="absolute left-3 top-1/2 -translate-y-1/2 text-text-tertiary" />
          <input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Search apps…"
            className="h-[38px] w-full rounded-md border-[1.5px] border-surface-border bg-field pl-9 pr-3 text-sm text-text-primary outline-none transition-colors focus:border-primary"
          />
        </div>
        <div className="inline-flex rounded-md border border-surface-border bg-surface-card p-0.5">
          {STATUS.map(([k, label]) => (
            <button key={k} onClick={() => setStatus(k)} className={cn('h-8 rounded-[8px] px-3 text-[13px] font-medium transition-colors', status === k ? 'bg-primary text-white' : 'text-text-secondary hover:bg-surface-hover')}>{label}</button>
          ))}
        </div>
      </div>

      <div className="flex flex-wrap gap-2">
        {categories.map((c) => (
          <button
            key={c}
            onClick={() => setCategory(c)}
            className={cn('rounded-full border px-3 py-1 text-xs font-medium transition-colors', category === c ? 'border-primary bg-primary/10 text-primary' : 'border-surface-border bg-surface-card text-text-secondary hover:border-surface-border-hover')}
          >
            {c}<span className="ml-1.5 text-text-tertiary">{counts[c] || 0}</span>
          </button>
        ))}
      </div>

      {isLoading ? (
        <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">{Array.from({ length: 6 }, (_, i) => <Skeleton key={i} className="h-36 w-full" />)}</div>
      ) : isError ? (
        <EmptyState icon={AlertTriangle} title="Could not load apps" message="Check that the panel backend is running, then refresh." />
      ) : shown.length === 0 ? (
        <EmptyState icon={Search} title="No apps found" message="Try another search or filter." />
      ) : (
        <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
          {shown.map((a) => (
            <AppCard key={a.id} app={a} pending={install.isPending || remove.isPending || control.isPending} onInstall={setInstalling} onRemove={setRemoving} onControl={(app, action, version) => control.mutate({ app, action, version })} onOpenDatabases={() => navigate('/databases')} />
          ))}
        </div>
      )}

      <InstallDialog app={installing} loading={install.isPending} onClose={() => setInstalling(null)} onConfirm={(app, version) => install.mutate({ app, version })} />
      <RemoveDialog app={removing} loading={remove.isPending} onClose={() => setRemoving(null)} onConfirm={(app, version) => remove.mutate({ app, version })} />
    </div>
  )
}

export default AppStore
