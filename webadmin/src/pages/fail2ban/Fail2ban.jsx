import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import {
  ShieldAlert, ShieldCheck, ShieldOff, Ban, Unlock, RotateCw, Play, Square, Plus, X, Search, Gavel, Eye, Hourglass, Gauge, RefreshCw,
} from 'lucide-react'
import Button from '../../components/ui/Button'
import Input from '../../components/ui/Input'
import Badge from '../../components/ui/Badge'
import Modal from '../../components/ui/Modal'
import Tabs from '../../components/ui/Tabs'
import ConfirmDialog from '../../components/ui/ConfirmDialog'
import Skeleton from '../../components/ui/Skeleton'
import usePermission from '../../hooks/usePermission'
import { cn } from '../../utils/cn'
import * as api from '../../api/fail2ban'

const tabs = ['Overview', 'Settings', 'Logs']
const errMsg = (e) => e.response?.data?.message || 'Operation failed'

// "600" -> "10 min", "1h" -> "1 hour", "-1" -> "permanent"
function humanTime(v) {
  if (v === undefined || v === null || v === '') return '—'
  const s = String(v).trim()
  if (s === '-1') return 'permanent'
  const m = s.match(/^(\d+)([smhdw]?)$/)
  if (!m) return s
  const units = { '': 1, s: 1, m: 60, h: 3600, d: 86400, w: 604800 }
  let sec = Number(m[1]) * units[m[2]]
  const parts = []
  for (const [n, label] of [[604800, 'wk'], [86400, 'day'], [3600, 'hr'], [60, 'min'], [1, 'sec']]) {
    if (sec >= n) { const q = Math.floor(sec / n); sec -= q * n; parts.push(`${q} ${label}${q > 1 && label !== 'min' && label !== 'sec' && label !== 'hr' && label !== 'wk' ? 's' : ''}`) }
    if (parts.length === 2) break
  }
  return parts.join(' ') || '0 sec'
}

function Stat({ icon: Icon, label, value, tone = 'text-text-primary', hint }) {
  return (
    <div className="rounded-xl border border-surface-border bg-surface-card p-4">
      <p className="mb-2 flex items-center gap-2 text-xs font-medium text-text-secondary"><Icon size={14} />{label}</p>
      <p className={cn('text-2xl font-semibold', tone)}>{value}</p>
      {hint && <p className="mt-1 text-[11px] text-text-tertiary">{hint}</p>}
    </div>
  )
}

function Fail2ban() {
  const qc = useQueryClient()
  const { canEdit } = usePermission()
  const editable = canEdit('fail2ban')
  const [tab, setTab] = useState('Overview')

  const { data: inst } = useQuery({ queryKey: ['fail2ban', 'installed'], queryFn: api.getInstalled })
  const installed = inst?.data?.installed
  const statusQ = useQuery({
    queryKey: ['fail2ban', 'status'], queryFn: api.getStatus, enabled: installed === true, refetchInterval: tab === 'Overview' ? 10000 : false,
  })
  const st = statusQ.data?.data

  const refresh = () => qc.invalidateQueries({ queryKey: ['fail2ban'] })
  const onError = (e) => toast.error(errMsg(e), { duration: 7000 })
  const ctl = useMutation({
    mutationFn: api.control,
    onSuccess: (_, a) => { toast.success(`Fail2ban ${{ start: 'started', stop: 'stopped', restart: 'restarted', reload: 'reloaded' }[a]}`); refresh() },
    onError,
  })

  if (installed === undefined) return <Skeleton className="h-64 w-full" />
  if (!installed) {
    return (
      <div className="rounded-xl border border-surface-border bg-surface-card p-10 text-center">
        <span className="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-danger-bg text-danger"><ShieldAlert size={28} /></span>
        <h2 className="text-lg font-semibold text-text-primary">Fail2ban is not installed</h2>
        <p className="mx-auto mt-1 max-w-md text-sm text-text-secondary">Install it from the App Store to automatically ban IP addresses that keep failing to log in.</p>
        <Link to="/app-store" className="mt-5 inline-flex h-[38px] items-center rounded-md bg-primary px-4 text-[13px] font-medium text-white hover:opacity-90">Open App Store</Link>
      </div>
    )
  }

  const running = !!st?.running
  const jails = st?.jails || []
  const totals = jails.reduce((a, j) => ({ banned: a.banned + j.currently_banned, total: a.total + j.total_banned, failed: a.failed + j.total_failed }), { banned: 0, total: 0, failed: 0 })

  return (
    <div className="space-y-5">
      <section className={cn('flex flex-wrap items-center justify-between gap-5 rounded-xl border p-5', !st ? 'border-surface-border bg-surface-card' : running ? 'border-success/30 bg-success-bg' : 'border-danger/30 bg-danger-bg')}>
        <div className="flex items-center gap-4">
          <span className={cn('flex h-14 w-14 items-center justify-center rounded-2xl text-white shadow-md', running ? 'bg-success' : 'bg-danger')}>
            {running ? <ShieldCheck size={28} /> : <ShieldOff size={28} />}
          </span>
          <div>
            <h2 className="text-lg font-semibold text-text-primary">Fail2ban is {!st ? '…' : running ? 'running' : 'stopped'}</h2>
            <p className="text-sm text-text-secondary">
              {running ? `${jails.length} active jail${jails.length === 1 ? '' : 's'} · ${totals.banned} IP${totals.banned === 1 ? '' : 's'} banned right now` : 'No IP is being protected while the service is stopped.'}
              {st?.version && <span className="ml-2 font-mono text-xs text-text-tertiary">v{st.version}</span>}
            </p>
          </div>
        </div>
        {editable && (
          <div className="flex flex-wrap gap-2">
            {running ? (
              <>
                <Button variant="secondary" disabled={ctl.isPending} onClick={() => ctl.mutate('reload')}><RefreshCw size={14} />Reload</Button>
                <Button variant="secondary" disabled={ctl.isPending} onClick={() => ctl.mutate('restart')}><RotateCw size={14} />Restart</Button>
                <Button variant="secondary" disabled={ctl.isPending} onClick={() => ctl.mutate('stop')}><Square size={14} />Stop</Button>
              </>
            ) : (
              <Button disabled={ctl.isPending} onClick={() => ctl.mutate('start')}><Play size={14} />Start</Button>
            )}
          </div>
        )}
      </section>
      {st?.error && <p className="text-sm text-danger">{st.error}</p>}

      <Tabs tabs={tabs} value={tab} onChange={setTab} />

      {tab === 'Overview' && (
        <Overview st={st} loading={statusQ.isLoading} totals={totals} jails={jails} editable={editable} refresh={refresh} onError={onError} />
      )}
      {tab === 'Settings' && <SettingsTab editable={editable} clientIP={st?.client_ip} refresh={refresh} onError={onError} />}
      {tab === 'Logs' && <LogsTab />}
    </div>
  )
}

function Overview({ st, loading, totals, jails, editable, refresh, onError }) {
  const [banFor, setBanFor] = useState(null) // jail name
  const [ip, setIp] = useState('')
  const [unban, setUnban] = useState(null) // { jail, ip }
  const [q, setQ] = useState('')

  const ban = useMutation({
    mutationFn: () => api.banIP(banFor, ip.trim()),
    onSuccess: () => { toast.success(`${ip.trim()} banned`); setBanFor(null); setIp(''); refresh() },
    onError,
  })
  const doUnban = useMutation({
    mutationFn: () => api.unbanIP(unban.jail, unban.ip),
    onSuccess: () => { toast.success(`${unban.ip} unbanned`); setUnban(null); refresh() },
    onError,
  })

  if (loading) return <Skeleton className="h-48 w-full" />
  if (!st?.running) {
    return <p className="rounded-xl border border-surface-border bg-surface-card p-8 text-center text-sm text-text-secondary">Start the service to see jails and banned addresses.</p>
  }

  return (
    <div className="space-y-5">
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <Stat icon={Eye} label="Active jails" value={jails.length} />
        <Stat icon={Ban} label="Banned now" value={totals.banned} tone={totals.banned ? 'text-danger' : undefined} />
        <Stat icon={Gavel} label="Banned in total" value={totals.total} hint="since the service started" />
        <Stat icon={ShieldAlert} label="Failed attempts" value={totals.failed} tone={totals.failed ? 'text-warning' : undefined} hint="since the service started" />
      </div>

      <div className="relative max-w-xs">
        <Search size={14} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-text-tertiary" />
        <Input className="pl-8" placeholder="Search banned IP…" value={q} onChange={(e) => setQ(e.target.value)} />
      </div>

      {jails.length === 0 && <p className="text-sm text-text-secondary">No jail is active. Enable one in the Settings tab.</p>}
      <div className="grid gap-4 xl:grid-cols-2">
        {jails.map((j) => {
          const ips = j.banned_ips.filter((x) => x.includes(q.trim()))
          return (
            <section key={j.name} className="rounded-xl border border-surface-border bg-surface-card">
              <header className="flex flex-wrap items-start justify-between gap-3 border-b border-surface-border p-4">
                <div>
                  <h3 className="flex items-center gap-2 font-semibold text-text-primary">
                    {j.title}
                    <span className="rounded bg-surface-hover px-1.5 py-0.5 font-mono text-[11px] font-normal text-text-secondary">{j.name}</span>
                  </h3>
                  <p className="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-xs text-text-secondary">
                    <span className="inline-flex items-center gap-1"><Hourglass size={12} />ban {humanTime(j.ban_time)}</span>
                    <span className="inline-flex items-center gap-1"><Gauge size={12} />{j.max_retry} tries in {humanTime(j.find_time)}</span>
                  </p>
                </div>
                {editable && (
                  <Button variant="secondary" className="h-8 px-3 text-xs" onClick={() => { setBanFor(j.name); setIp('') }}><Ban size={13} />Ban IP</Button>
                )}
              </header>
              <div className="grid grid-cols-4 divide-x divide-surface-border border-b border-surface-border text-center">
                {[['Failed now', j.currently_failed], ['Failed total', j.total_failed], ['Banned now', j.currently_banned], ['Banned total', j.total_banned]].map(([l, v]) => (
                  <div key={l} className="px-2 py-3"><p className="text-lg font-semibold text-text-primary">{v}</p><p className="text-[10px] uppercase tracking-wide text-text-tertiary">{l}</p></div>
                ))}
              </div>
              <div className="p-4">
                {ips.length === 0 ? (
                  <p className="text-sm text-text-tertiary">{j.banned_ips.length ? 'No match.' : 'No banned addresses.'}</p>
                ) : (
                  <ul className="divide-y divide-surface-border">
                    {ips.map((x) => (
                      <li key={x} className="flex items-center justify-between gap-3 py-2">
                        <span className="font-mono text-sm text-text-primary">{x}</span>
                        {editable && (
                          <button onClick={() => setUnban({ jail: j.name, ip: x })} className="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs text-text-secondary hover:bg-surface-hover hover:text-text-primary">
                            <Unlock size={13} />Unban
                          </button>
                        )}
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            </section>
          )
        })}
      </div>

      <Modal open={!!banFor} onClose={() => setBanFor(null)} title={`Ban an IP in ${banFor}`} size="sm">
        <form onSubmit={(e) => { e.preventDefault(); ban.mutate() }} className="space-y-4">
          <Input label="IP address" placeholder="203.0.113.7" value={ip} onChange={(e) => setIp(e.target.value)} autoFocus />
          <p className="text-xs text-text-tertiary">The ban lasts for the jail&apos;s ban time. Your own address cannot be banned.</p>
          <div className="flex justify-end gap-2">
            <Button type="button" variant="secondary" onClick={() => setBanFor(null)}>Cancel</Button>
            <Button type="submit" disabled={ban.isPending || !ip.trim()}>{ban.isPending ? 'Banning…' : 'Ban'}</Button>
          </div>
        </form>
      </Modal>
      <ConfirmDialog
        open={!!unban} onClose={() => setUnban(null)} onConfirm={() => doUnban.mutate()} loading={doUnban.isPending}
        title="Unban address" message={unban && `Allow ${unban.ip} to connect again (${unban.jail})?`} confirmLabel="Unban" loadingLabel="Unbanning…" variant="primary"
      />
    </div>
  )
}

const presets = [['10 min', '10m'], ['1 hour', '1h'], ['1 day', '1d'], ['1 week', '1w'], ['Permanent', '-1']]

function SettingsTab({ editable, clientIP, refresh, onError }) {
  const { data, isLoading } = useQuery({ queryKey: ['fail2ban', 'settings'], queryFn: api.getSettings })
  const [f, setF] = useState(null)
  const [newIP, setNewIP] = useState('')
  useEffect(() => { if (data?.data) setF(structuredClone(data.data)) }, [data])

  const save = useMutation({
    mutationFn: () => api.saveSettings(f),
    onSuccess: () => { toast.success('Settings saved and applied'); refresh() },
    onError,
  })
  const dirty = useMemo(() => f && data?.data && JSON.stringify(f) !== JSON.stringify(data.data), [f, data])

  if (isLoading || !f) return <Skeleton className="h-64 w-full" />
  const set = (k, v) => setF((p) => ({ ...p, [k]: v }))
  const setJail = (name, patch) => setF((p) => ({ ...p, jails: p.jails.map((j) => (j.name === name ? { ...j, ...patch } : j)) }))
  const addIP = (v) => {
    v = v.trim()
    if (!v || f.ignore_ip.includes(v)) return
    set('ignore_ip', [...f.ignore_ip, v]); setNewIP('')
  }
  const dis = !editable

  return (
    <div className="space-y-5">
      <section className="rounded-xl border border-surface-border bg-surface-card p-5">
        <h3 className="font-semibold text-text-primary">Ban rules</h3>
        <p className="mb-4 text-sm text-text-secondary">An address that fails <b>{f.max_retry}</b> times within <b>{humanTime(f.find_time)}</b> is banned for <b>{humanTime(f.ban_time)}</b>.</p>
        <div className="grid gap-4 md:grid-cols-3">
          <div>
            <Input label="Ban time" value={f.ban_time} disabled={dis} onChange={(e) => set('ban_time', e.target.value.trim())} />
            <div className="mt-2 flex flex-wrap gap-1">
              {presets.map(([l, v]) => (
                <button key={v} type="button" disabled={dis} onClick={() => set('ban_time', v)} className={cn('rounded-md px-2 py-1 text-[11px]', f.ban_time === v ? 'bg-primary text-white' : 'bg-surface-hover text-text-secondary hover:text-text-primary')}>{l}</button>
              ))}
            </div>
          </div>
          <div>
            <Input label="Find time" value={f.find_time} disabled={dis} onChange={(e) => set('find_time', e.target.value.trim())} />
            <p className="mt-2 text-[11px] text-text-tertiary">Window in which failures are counted: 600, 10m, 1h…</p>
          </div>
          <div>
            <Input label="Max retries" type="number" min={1} value={f.max_retry} disabled={dis} onChange={(e) => set('max_retry', Number(e.target.value))} />
            <p className="mt-2 text-[11px] text-text-tertiary">Failures allowed before the ban.</p>
          </div>
        </div>
      </section>

      <section className="rounded-xl border border-surface-border bg-surface-card p-5">
        <h3 className="font-semibold text-text-primary">Never ban (whitelist)</h3>
        <p className="mb-3 text-sm text-text-secondary">These addresses and ranges are always allowed. Add your own IP so a typo can never lock you out.</p>
        <div className="mb-3 flex flex-wrap gap-2">
          {f.ignore_ip.map((x) => (
            <span key={x} className="inline-flex items-center gap-1 rounded-md bg-surface-hover py-1 pl-2 pr-1 font-mono text-xs text-text-primary">
              {x}
              {!dis && <button onClick={() => set('ignore_ip', f.ignore_ip.filter((y) => y !== x))} className="rounded p-0.5 text-text-tertiary hover:text-danger" aria-label={`Remove ${x}`}><X size={12} /></button>}
            </span>
          ))}
        </div>
        {!dis && (
          <form onSubmit={(e) => { e.preventDefault(); addIP(newIP) }} className="flex max-w-md flex-wrap gap-2">
            <Input className="flex-1" placeholder="203.0.113.5 or 10.0.0.0/24" value={newIP} onChange={(e) => setNewIP(e.target.value)} />
            <Button type="submit" variant="secondary" disabled={!newIP.trim()}><Plus size={14} />Add</Button>
            {clientIP && !f.ignore_ip.includes(clientIP) && (
              <Button type="button" variant="secondary" onClick={() => addIP(clientIP)}>Add my IP ({clientIP})</Button>
            )}
          </form>
        )}
      </section>

      <section className="rounded-xl border border-surface-border bg-surface-card p-5">
        <h3 className="font-semibold text-text-primary">Jails</h3>
        <p className="mb-4 text-sm text-text-secondary">A jail watches one service and bans abusers. Leave the overrides empty to use the ban rules above.</p>
        <div className="divide-y divide-surface-border">
          {f.jails.map((j) => (
            <div key={j.name} className="py-4 first:pt-0 last:pb-0">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="flex items-center gap-2 font-medium text-text-primary">{j.title}<span className="rounded bg-surface-hover px-1.5 py-0.5 font-mono text-[11px] font-normal text-text-secondary">{j.name}</span></p>
                  <p className="text-xs text-text-secondary">{j.description}</p>
                  {!j.available && <p className="mt-1 text-xs text-warning">Unavailable: {j.unavailable}</p>}
                </div>
                <label className={cn('inline-flex items-center gap-2 text-sm', (!j.available && !j.enabled) || dis ? 'opacity-50' : 'cursor-pointer')}>
                  <input type="checkbox" className="h-4 w-4 accent-primary" checked={j.enabled} disabled={dis || (!j.available && !j.enabled)} onChange={(e) => setJail(j.name, { enabled: e.target.checked })} />
                  {j.enabled ? <Badge variant="success">Enabled</Badge> : <Badge>Disabled</Badge>}
                </label>
              </div>
              {j.enabled && (
                <div className="mt-3 grid max-w-xl gap-3 sm:grid-cols-3">
                  <Input label="Ban time" placeholder={`default (${f.ban_time})`} value={j.ban_time} disabled={dis} onChange={(e) => setJail(j.name, { ban_time: e.target.value.trim() })} />
                  <Input label="Find time" placeholder={`default (${f.find_time})`} value={j.find_time} disabled={dis} onChange={(e) => setJail(j.name, { find_time: e.target.value.trim() })} />
                  <Input label="Max retries" type="number" min={0} placeholder={`default (${f.max_retry})`} value={j.max_retry || ''} disabled={dis} onChange={(e) => setJail(j.name, { max_retry: Number(e.target.value) || 0 })} />
                </div>
              )}
              {j.name === 'sshd' && !j.enabled && <p className="mt-2 text-xs text-warning">SSH protection is off: brute-force attempts will not be banned.</p>}
            </div>
          ))}
        </div>
      </section>

      {editable && (
        <div className="sticky bottom-4 flex items-center justify-end gap-3 rounded-xl border border-surface-border bg-surface-card/95 p-3 shadow-md backdrop-blur">
          <span className="text-xs text-text-tertiary">{dirty ? 'Unsaved changes' : 'All changes saved'}</span>
          <Button variant="secondary" disabled={!dirty || save.isPending} onClick={() => setF(structuredClone(data.data))}>Reset</Button>
          <Button disabled={!dirty || save.isPending} onClick={() => save.mutate()}>{save.isPending ? 'Applying…' : 'Save & apply'}</Button>
        </div>
      )}
    </div>
  )
}

function lineTone(l) {
  if (/\bBan\b/.test(l)) return 'text-danger'
  if (/\bUnban\b/.test(l)) return 'text-success'
  if (/\bFound\b/.test(l)) return 'text-warning'
  if (/ERROR|CRITICAL/.test(l)) return 'text-danger'
  return 'text-slate-300'
}

function LogsTab() {
  const [q, setQ] = useState('')
  const { data, isLoading, isFetching, refetch } = useQuery({ queryKey: ['fail2ban', 'log'], queryFn: () => api.getLog(300), refetchInterval: 15000 })
  const lines = (data?.data?.lines || []).filter((l) => l.toLowerCase().includes(q.toLowerCase()))

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="relative w-full max-w-xs">
          <Search size={14} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-text-tertiary" />
          <Input className="pl-8" placeholder="Filter lines (IP, jail, Ban…)" value={q} onChange={(e) => setQ(e.target.value)} />
        </div>
        <Button variant="secondary" onClick={() => refetch()} disabled={isFetching}><RefreshCw size={14} className={isFetching ? 'animate-spin' : ''} />Refresh</Button>
      </div>
      {isLoading ? <Skeleton className="h-64 w-full" /> : (
        <pre className="max-h-[560px] overflow-auto rounded-xl border border-ide-border bg-[#0b0f14] p-4 font-mono text-xs leading-5">
          {lines.length === 0 ? <span className="text-slate-500">No log lines.</span> : [...lines].reverse().map((l, i) => <div key={i} className={lineTone(l)}>{l}</div>)}
        </pre>
      )}
      <p className="text-[11px] text-text-tertiary">Newest first · last 300 lines of /var/log/fail2ban.log · refreshes every 15 s.</p>
    </div>
  )
}

export default Fail2ban
