import { useMemo, useState } from 'react'
import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { Lock, CircleCheck, CircleX, LockOpen, Activity } from 'lucide-react'
import Skeleton from '../../components/ui/Skeleton'
import Badge from '../../components/ui/Badge'
import StatTile, { Meter } from '../../components/charts/StatTile'
import ChartCard, { LegendItem } from '../../components/charts/ChartCard'
import StackedColumns from '../../components/charts/StackedColumns'
import LineChart from '../../components/charts/LineChart'
import HBars from '../../components/charts/HBars'
import { compact, pct, tooltipTime } from '../../components/charts/scale'
import Insights from './Insights'
import usePermission from '../../hooks/usePermission'
import { cn } from '../../utils/cn'
import { getOverview, getHistory, getHistoryRange, getProcesses, getTraffic, getInsights } from '../../api/dashboard'
import { getActivityLogs } from '../../api/activityLogs'
import { formatBytes, formatUptime } from '../../utils/format'

const POLL = 3000
const RANGES = [['1h', 'Last hour', '1h'], ['6h', 'Last 6 hours', '6h'], ['24h', 'Last 24 hours', '24h'], ['7d', 'Last 7 days', '7d']]
const rangeLabel = (r) => RANGES.find((x) => x[0] === r)?.[1].toLowerCase()

// The traffic classes mean good/bad, so they use the fixed status roles (never a free series hue).
const C_OK = 'var(--viz-1)'
const C_CLIENT = 'var(--viz-warn)'
const C_SERVER = 'var(--viz-crit)'

const pctDelta = (cur, prev) => {
  if (!prev) return null
  const d = ((cur - prev) / prev) * 100
  if (Math.abs(d) < 0.5) return { text: '0%', dir: 'flat' }
  return { text: `${d > 0 ? '+' : ''}${Math.abs(d) >= 100 ? Math.round(d) : d.toFixed(0)}%`, dir: d > 0 ? 'up' : 'down' }
}

function Section({ title, hint, children, right }) {
  return (
    <section className="space-y-3">
      <div className="flex flex-wrap items-end justify-between gap-2">
        <div>
          <h2 className="text-[15px] font-semibold text-text-primary">{title}</h2>
          {hint && <p className="text-xs text-text-tertiary">{hint}</p>}
        </div>
        {right}
      </div>
      {children}
    </section>
  )
}

function Panel({ title, children, action, className }) {
  return (
    <section className={cn('rounded-lg border border-surface-border bg-surface-card p-4', className)}>
      <header className="mb-3 flex items-center justify-between gap-2">
        <h3 className="text-sm font-semibold text-text-primary">{title}</h3>
        {action}
      </header>
      {children}
    </section>
  )
}

function Dashboard() {
  const navigate = useNavigate()
  const { canView } = usePermission()
  const [range, setRange] = useState('24h')
  const [site, setSite] = useState('')
  const [sortBy, setSortBy] = useState('cpu')

  const { data: ov } = useQuery({ queryKey: ['dash-overview'], queryFn: getOverview, refetchInterval: POLL })
  const { data: hist } = useQuery({ queryKey: ['dash-history'], queryFn: getHistory, refetchInterval: POLL })
  const { data: ins } = useQuery({ queryKey: ['dash-insights'], queryFn: getInsights, refetchInterval: 30000 })
  const traffic = useQuery({ queryKey: ['dash-traffic', range, site], queryFn: () => getTraffic(range, site), refetchInterval: 60000, placeholderData: keepPreviousData })
  const trend = useQuery({ queryKey: ['dash-trend', range], queryFn: () => getHistoryRange(range), refetchInterval: 60000, placeholderData: keepPreviousData })
  const { data: procs } = useQuery({ queryKey: ['dash-procs', sortBy], queryFn: () => getProcesses({ limit: 7, sort: sortBy }), refetchInterval: POLL * 4 })
  const { data: acts } = useQuery({ queryKey: ['dash-activity'], queryFn: () => getActivityLogs({ limit: 6 }), enabled: canView('activity-logs'), refetchInterval: 30000 })

  const o = ov?.data
  const live = hist?.data || []
  const insights = ins?.data
  const t = traffic.data?.data
  const points = trend.data?.data || []
  const busy = traffic.isFetching || trend.isFetching

  const rootDisk = useMemo(() => o?.disks?.find((d) => d.mount === '/') || o?.disks?.[0], [o])
  const sites = useMemo(() => {
    const stats = Object.fromEntries((t?.sites || []).map((s) => [s.domain, s]))
    return (insights?.sites || []).map((s) => ({ ...s, ...(stats[s.domain] || { requests: 0, client_err: 0, server_err: 0, bytes: 0 }) }))
      .sort((a, b) => b.requests - a.requests || a.domain.localeCompare(b.domain))
  }, [insights, t])

  if (!o) return <div className="space-y-4"><Skeleton className="h-24 w-full" /><Skeleton className="h-72 w-full" /></div>

  const tot = t?.totals
  const prev = t?.previous
  const errRate = tot?.requests ? (tot.server_err / tot.requests) * 100 : 0
  const prevErrRate = prev?.requests ? (prev.server_err / prev.requests) * 100 : null
  const errDelta = prevErrRate === null ? null : (() => {
    const d = errRate - prevErrRate
    return Math.abs(d) < 0.05 ? { text: '0 pp', dir: 'flat' } : { text: `${d > 0 ? '+' : '−'}${Math.abs(d).toFixed(1)} pp`, dir: d > 0 ? 'up' : 'down', good: 'down' }
  })()
  const vsNote = prev ? `vs previous ${range === '7d' ? '7 days' : range === '1h' ? 'hour' : range}` : 'no earlier data yet'
  const withGood = (d, good) => (d ? { ...d, good } : null)
  const hasTraffic = t?.available && tot?.requests > 0
  const cores = o.cpu_cores || 1
  const last = points[points.length - 1]

  const trafficSeries = [
    { key: 'ok', label: 'Successful (2xx–3xx)', color: C_OK },
    { key: 'client_err', label: 'Client errors (4xx)', color: C_CLIENT },
    { key: 'server_err', label: 'Server errors (5xx)', color: C_SERVER },
  ]

  return (
    <div className="space-y-6">
      {/* ---- header ---- */}
      <div className="flex flex-wrap items-center justify-between gap-x-6 gap-y-1">
        <div className="min-w-0">
          <h1 className="truncate text-lg font-semibold text-text-primary">{o.hostname}</h1>
          <p className="truncate text-xs text-text-tertiary">{o.os} · kernel {o.kernel} · {o.arch}</p>
        </div>
        <div className="flex items-center gap-4 text-xs text-text-secondary">
          <span className="inline-flex items-center gap-1.5"><Activity size={13} className="text-text-tertiary" />Up {formatUptime(o.uptime)}</span>
          {insights?.updates?.known && <span>{insights.updates.total} update{insights.updates.total === 1 ? '' : 's'}</span>}
          {insights?.firewall?.known && <span className="inline-flex items-center gap-1">{insights.firewall.active ? <CircleCheck size={13} className="text-success" /> : <CircleX size={13} className="text-danger" />}Firewall {insights.firewall.active ? 'on' : 'off'}</span>}
        </div>
      </div>

      {/* ---- what to do now ---- */}
      {insights ? <Insights items={insights.insights} /> : <Skeleton className="h-28 w-full" />}

      {/* ---- live resources ---- */}
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <StatTile label="CPU" value={o.cpu_percent.toFixed(0)} unit="%" meter={o.cpu_percent} sub={`${cores} core${cores > 1 ? 's' : ''} · ${(o.cpu_model || '').replace(/\(R\)|\(TM\)|CPU /g, '').trim()}`} trend={live.map((p) => p.cpu)} trendMax={100} />
        <StatTile label="Memory" value={o.mem_percent.toFixed(0)} unit="%" meter={o.mem_percent} sub={`${formatBytes(o.mem_used)} of ${formatBytes(o.mem_total)}${o.swap_total ? ` · swap ${formatBytes(o.swap_used)}` : ''}`} trend={live.map((p) => p.mem)} trendMax={100} />
        <StatTile label={`Disk ${rootDisk?.mount || ''}`} value={(rootDisk?.percent || 0).toFixed(0)} unit="%" meter={rootDisk?.percent || 0} warnAt={80} sub={rootDisk ? `${formatBytes(rootDisk.total - rootDisk.used)} free of ${formatBytes(rootDisk.total)}` : '—'} />
        <StatTile label="Load average" value={o.load1.toFixed(2)} meter={(o.load5 / cores) * 100} warnAt={100} critAt={200} sub={`5m ${o.load5.toFixed(2)} · 15m ${o.load15.toFixed(2)} · ${(o.load5 / cores).toFixed(2)} per core`} trend={live.map((p) => p.load1)} />
      </div>

      {/* ---- filters: one row, scopes everything below ---- */}
      <div className="sticky top-[60px] z-10 -mx-1 flex flex-wrap items-center gap-3 rounded-lg border border-surface-border bg-surface-card/95 px-3 py-2 backdrop-blur">
        <div className="inline-flex rounded-md border border-surface-border p-0.5" role="group" aria-label="Time range">
          {RANGES.map(([k, , short]) => (
            <button key={k} onClick={() => setRange(k)} aria-pressed={range === k} className={cn('h-7 rounded-[8px] px-3 text-xs font-medium transition-colors', range === k ? 'bg-primary text-white' : 'text-text-secondary hover:bg-surface-hover')}>{short}</button>
          ))}
        </div>
        <select value={site} onChange={(e) => setSite(e.target.value)} aria-label="Website" className="h-8 rounded-md border border-surface-border bg-field px-2.5 text-xs text-text-primary outline-none focus:border-primary">
          <option value="">All websites</option>
          {(insights?.sites || []).map((s) => <option key={s.domain} value={s.domain}>{s.domain}</option>)}
        </select>
        <span className="ml-auto text-xs text-text-tertiary">{rangeLabel(range)}{busy ? ' · updating…' : ''}</span>
      </div>

      {/* ---- traffic ---- */}
      <Section title="Traffic" hint={t?.available ? `From the nginx access logs${t.truncated ? ' (very large logs: newest part only)' : ''}` : undefined}>
        {!t ? <Skeleton className="h-64 w-full" /> : !t.available || !tot?.requests ? (
          <div className="rounded-lg border border-dashed border-surface-border-hover p-10 text-center">
            <p className="text-sm font-medium text-text-primary">{!(insights?.sites || []).length ? 'No websites yet' : 'No requests in this period'}</p>
            <p className="mx-auto mt-1 max-w-md text-xs text-text-secondary">
              {!(insights?.sites || []).length ? 'Add a website and its visits, errors and top pages show up here.' : 'Traffic is read from each site’s nginx access log. Try a longer range, or wait for the first visits.'}
            </p>
            {!(insights?.sites || []).length && <button onClick={() => navigate('/websites')} className="mt-3 text-xs font-medium text-primary hover:underline">Add a website →</button>}
          </div>
        ) : (
          <>
            <div className={cn('grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4', busy && 'opacity-60 transition-opacity')}>
              <StatTile label="Requests" value={compact(tot.requests)} delta={withGood(pctDelta(tot.requests, prev?.requests), 'up')} deltaNote={vsNote} sub={`${compact(Math.round(tot.requests / Math.max(1, (t.until - t.since) / 3600)))} per hour on average`} />
              <StatTile label="Unique visitors" value={compact(tot.visitors)} delta={withGood(pctDelta(tot.visitors, prev?.visitors), 'up')} deltaNote={vsNote} sub={`${pct(tot.bots, tot.requests, 0)} of requests are bots or scripts`} />
              <StatTile label="Data served" value={formatBytes(tot.bytes)} delta={pctDelta(tot.bytes, prev?.bytes)} deltaNote={vsNote} sub={`${formatBytes(tot.bytes / Math.max(1, tot.requests))} per request`} />
              <StatTile label="Server errors (5xx)" value={errRate.toFixed(errRate < 10 ? 2 : 1)} unit="%" delta={errDelta} deltaNote={vsNote} sub={`${tot.server_err.toLocaleString()} 5xx · ${tot.client_err.toLocaleString()} 4xx of ${tot.requests.toLocaleString()}`} />
            </div>

            <div className="grid grid-cols-1 gap-3 xl:grid-cols-3">
              <ChartCard
                className="xl:col-span-2" busy={busy} title="Requests over time" subtitle={`Per ${t.bucket_sec >= 86400 ? 'day' : t.bucket_sec >= 3600 ? `${t.bucket_sec / 3600} hour${t.bucket_sec > 3600 ? 's' : ''}` : `${t.bucket_sec / 60} minutes`}, split by outcome`}
                legend={trafficSeries.map((s) => <LegendItem key={s.key} color={s.color} label={s.label} value={t.buckets.reduce((n, b) => n + (b[s.key] || 0), 0).toLocaleString()} />)}
                table={{ columns: ['Time', 'Successful', '4xx', '5xx'], rows: t.buckets.map((b) => [tooltipTime(b.t, t.until - t.since, t.bucket_sec >= 86400), b.ok, b.client_err, b.server_err]) }}
              >
                <StackedColumns buckets={t.buckets} series={trafficSeries} bucketSec={t.bucket_sec} daily={t.bucket_sec >= 86400} ariaLabel={`Requests over the ${rangeLabel(range)}, split into successful, client error and server error responses`} />
              </ChartCard>
              <Panel title="Busiest websites" action={<span className="text-xs text-text-tertiary">requests</span>}>
                <HBars items={t.sites.filter((s) => s.requests).slice(0, 6).map((s) => ({ key: s.domain, label: s.domain, value: s.requests, text: compact(s.requests), sub: s.server_err ? `${s.server_err} 5xx` : undefined }))} />
              </Panel>
            </div>

            <div className="grid grid-cols-1 gap-3 lg:grid-cols-3">
              <Panel title="Top pages" action={<span className="text-xs text-text-tertiary">hits, without images &amp; scripts</span>}>
                <HBars items={t.top_paths.map((p) => ({ key: p.key, label: p.key, value: p.count, text: compact(p.count), sub: p.errors ? `${p.errors} err` : undefined }))} />
              </Panel>
              <Panel title="Top clients" action={<span className="text-xs text-text-tertiary">by requests</span>}>
                <HBars items={t.top_ips.map((p) => ({ key: p.key, label: p.key, value: p.count, text: compact(p.count), sub: pct(p.count, tot.requests, 0) }))} />
              </Panel>
              <Panel title="Response codes" action={<span className="text-xs text-text-tertiary">all requests</span>}>
                <HBars items={t.statuses.map((s) => ({ key: s.key, label: s.key, value: s.count, text: compact(s.count), sub: pct(s.count, tot.requests, 0), color: Number(s.key) >= 500 ? C_SERVER : Number(s.key) >= 400 ? C_CLIENT : C_OK }))} />
              </Panel>
            </div>
          </>
        )}
      </Section>

      {/* ---- resource history ---- */}
      <Section title="Resources over time" hint="Averages, kept in memory while the panel runs">
        <div className="grid grid-cols-1 gap-3 xl:grid-cols-3">
          <ChartCard
            busy={busy} title="CPU & memory" subtitle="Share of capacity in use"
            legend={[<LegendItem key="c" shape="line" color="var(--viz-1)" label="CPU" value={`${o.cpu_percent.toFixed(0)}%`} />, <LegendItem key="m" shape="line" color="var(--viz-2)" label="Memory" value={`${o.mem_percent.toFixed(0)}%`} />]}
            table={{ columns: ['Time', 'CPU %', 'Memory %'], rows: points.map((p) => [tooltipTime(p.t, 0), p.cpu.toFixed(1), p.mem.toFixed(1)]) }}
          >
            <LineChart points={points} yMax={100} fmt={(v) => `${Math.round(v)}%`} series={[{ key: 'cpu', label: 'CPU', color: 'var(--viz-1)' }, { key: 'mem', label: 'Memory', color: 'var(--viz-2)' }]} ariaLabel={`CPU and memory usage over the ${rangeLabel(range)}`} />
          </ChartCard>
          <ChartCard
            busy={busy} title="Network" subtitle="Throughput on all interfaces"
            legend={[<LegendItem key="d" shape="line" color="var(--viz-1)" label="Download" value={`${formatBytes(o.net_rx_bps)}/s`} />, <LegendItem key="u" shape="line" color="var(--viz-2)" label="Upload" value={`${formatBytes(o.net_tx_bps)}/s`} />]}
            table={{ columns: ['Time', 'Download', 'Upload'], rows: points.map((p) => [tooltipTime(p.t, 0), `${formatBytes(p.net_rx)}/s`, `${formatBytes(p.net_tx)}/s`]) }}
          >
            <LineChart points={points} fmt={(v) => `${formatBytes(v)}/s`} series={[{ key: 'net_rx', label: 'Download', color: 'var(--viz-1)' }, { key: 'net_tx', label: 'Upload', color: 'var(--viz-2)' }]} ariaLabel={`Network throughput over the ${rangeLabel(range)}`} />
          </ChartCard>
          <ChartCard
            busy={busy} title="Load average" subtitle={`1-minute load; ${cores} core${cores > 1 ? 's' : ''} can run that many tasks at once`}
            legend={[<LegendItem key="l" shape="line" color="var(--viz-1)" label="Load" value={o.load1.toFixed(2)} />]}
            table={{ columns: ['Time', 'Load (1m)'], rows: points.map((p) => [tooltipTime(p.t, 0), p.load1.toFixed(2)]) }}
          >
            <LineChart points={points} area refLine={{ value: cores, label: `${cores} core${cores > 1 ? 's' : ''}` }} fmt={(v) => v.toFixed(1)} series={[{ key: 'load1', label: 'Load (1m)', color: 'var(--viz-1)' }]} ariaLabel={`Load average over the ${rangeLabel(range)}`} />
          </ChartCard>
        </div>
        {last && points.length > 1 && <p className="text-xs text-text-tertiary">Peak CPU in this range: <b className="text-text-secondary">{Math.max(...points.map((p) => p.cpu)).toFixed(0)}%</b> · peak memory <b className="text-text-secondary">{Math.max(...points.map((p) => p.mem)).toFixed(0)}%</b></p>}
      </Section>

      {/* ---- websites & services ---- */}
      <div className="grid grid-cols-1 gap-3 xl:grid-cols-3">
        <Panel title="Websites" className="xl:col-span-2" action={<button onClick={() => navigate('/websites')} className="text-xs font-medium text-primary hover:underline">Manage →</button>}>
          {sites.length === 0 ? <p className="py-8 text-center text-xs text-text-tertiary">No websites yet</p> : (
            <div className="overflow-x-auto">
              <table className="w-full text-xs">
                <thead className="text-left text-text-tertiary">
                  <tr><th className="pb-2 font-medium">Domain</th><th className="pb-2 font-medium">HTTPS</th><th className="pb-2 text-right font-medium">Requests</th><th className="pb-2 text-right font-medium">4xx</th><th className="pb-2 text-right font-medium">5xx</th><th className="pb-2 text-right font-medium">Data</th></tr>
                </thead>
                <tbody>
                  {sites.map((s) => (
                    <tr key={s.domain} className="border-t border-surface-border">
                      <td className="py-2 pr-3"><span className="font-medium text-text-primary">{s.domain}</span>{s.status !== 'running' && <Badge className="ml-2">stopped</Badge>}</td>
                      <td className="py-2 pr-3">
                        {!s.ssl_enabled ? <span className="inline-flex items-center gap-1 text-text-tertiary"><LockOpen size={12} />None</span>
                          : <span className={cn('inline-flex items-center gap-1', s.days_left !== null && s.days_left < 14 ? (s.days_left < 7 ? 'text-danger' : 'text-warning') : 'text-text-secondary')}><Lock size={12} />{s.days_left === null ? 'On' : s.days_left < 0 ? 'Expired' : `${s.days_left} days`}</span>}
                      </td>
                      <td className="py-2 text-right tabular-nums text-text-primary">{s.requests.toLocaleString()}</td>
                      <td className="py-2 text-right tabular-nums text-text-secondary">{s.client_err.toLocaleString()}</td>
                      <td className={cn('py-2 text-right tabular-nums', s.server_err ? 'font-medium text-danger' : 'text-text-secondary')}>{s.server_err.toLocaleString()}</td>
                      <td className="py-2 text-right tabular-nums text-text-secondary">{formatBytes(s.bytes)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Panel>
        <Panel title="Services" action={<button onClick={() => navigate('/app-store')} className="text-xs font-medium text-primary hover:underline">App Store →</button>}>
          {!insights ? <Skeleton className="h-32 w-full" /> : (
            <ul className="divide-y divide-surface-border">
              {insights.services.map((s) => (
                <li key={s.id} className="flex items-center justify-between py-2 text-xs">
                  <span className="min-w-0 truncate text-text-primary">{s.name}{s.version && <span className="ml-1.5 font-mono text-text-tertiary">{s.version}</span>}</span>
                  <span className={cn('inline-flex shrink-0 items-center gap-1 font-medium', s.running ? 'text-success' : 'text-danger')}>
                    {s.running ? <CircleCheck size={13} /> : <CircleX size={13} />}{s.running ? 'Running' : 'Stopped'}
                  </span>
                </li>
              ))}
              {!insights.services.length && <li className="py-6 text-center text-xs text-text-tertiary">No services installed</li>}
            </ul>
          )}
        </Panel>
      </div>

      {/* ---- system detail ---- */}
      <div className="grid grid-cols-1 gap-3 xl:grid-cols-3">
        <Panel title="Storage">
          <div className="space-y-4">
            {o.disks.map((d) => (
              <div key={d.mount}>
                <div className="mb-1.5 flex items-baseline justify-between gap-2 text-xs">
                  <span className="truncate font-mono text-text-primary">{d.mount}</span>
                  <span className="shrink-0 text-text-secondary">{formatBytes(d.total - d.used)} free · <b className="font-semibold text-text-primary">{d.percent.toFixed(0)}%</b></span>
                </div>
                <Meter percent={d.percent} warnAt={80} />
              </div>
            ))}
          </div>
        </Panel>
        <Panel
          title="Top processes"
          action={
            <div className="inline-flex rounded-md border border-surface-border p-0.5" role="group" aria-label="Sort processes">
              {[['cpu', 'CPU'], ['mem', 'Memory']].map(([k, label]) => <button key={k} onClick={() => setSortBy(k)} aria-pressed={sortBy === k} className={cn('h-6 rounded-[7px] px-2.5 text-[11px] font-medium', sortBy === k ? 'bg-primary text-white' : 'text-text-secondary hover:bg-surface-hover')}>{label}</button>)}
            </div>
          }
        >
          <table className="w-full text-xs">
            <thead className="text-left text-text-tertiary"><tr><th className="pb-1.5 font-medium">Process</th><th className="pb-1.5 font-medium">User</th><th className="pb-1.5 text-right font-medium">CPU</th><th className="pb-1.5 text-right font-medium">Mem</th></tr></thead>
            <tbody>
              {(procs?.data || []).map((p) => (
                <tr key={p.pid} className="border-t border-surface-border">
                  <td className="max-w-[9rem] truncate py-1.5 pr-2 text-text-primary" title={`PID ${p.pid}`}>{p.name}</td>
                  <td className="max-w-[5rem] truncate pr-2 text-text-secondary">{p.user}</td>
                  <td className={cn('text-right tabular-nums', sortBy === 'cpu' ? 'font-medium text-text-primary' : 'text-text-secondary')}>{p.cpu.toFixed(1)}%</td>
                  <td className={cn('text-right tabular-nums', sortBy === 'mem' ? 'font-medium text-text-primary' : 'text-text-secondary')}>{p.mem.toFixed(1)}%</td>
                </tr>
              ))}
            </tbody>
          </table>
        </Panel>
        <Panel title="Recent activity" action={canView('activity-logs') && <button onClick={() => navigate('/activity-logs')} className="text-xs font-medium text-primary hover:underline">All →</button>}>
          {!canView('activity-logs') ? <p className="py-8 text-center text-xs text-text-tertiary">Activity logs are not enabled for your role</p> : (
            <ul className="divide-y divide-surface-border">
              {(acts?.data || []).map((a) => (
                <li key={a.uuid} className="py-2 text-xs">
                  <p className="text-text-primary"><span className="font-medium">{a.user?.name || 'System'}</span> <span className="text-text-secondary">{a.action}</span> <span className="font-mono text-text-secondary">{a.module}</span></p>
                  <p className="text-text-tertiary">{new Date(a.created_at).toLocaleString(undefined, { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit', hour12: false })} · {a.ip_address}</p>
                </li>
              ))}
              {!(acts?.data || []).length && <li className="py-6 text-center text-xs text-text-tertiary">Nothing yet</li>}
            </ul>
          )}
        </Panel>
      </div>
    </div>
  )
}

export default Dashboard
