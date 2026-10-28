import { useMemo, useState } from 'react'
import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { Lock, CircleCheck, CircleX, LockOpen, Activity, Network } from 'lucide-react'
import Skeleton from '../../components/ui/Skeleton'
import Badge from '../../components/ui/Badge'
import StatTile, { Meter } from '../../components/charts/StatTile'
import ChartCard, { LegendItem } from '../../components/charts/ChartCard'
import LineChart from '../../components/charts/LineChart'
import HBars from '../../components/charts/HBars'
import SplitBar from '../../components/charts/SplitBar'
import { tooltipTime, compact } from '../../components/charts/scale'
import { loadColor } from '../../components/charts/severity'
import { AttentionButton } from '../dashboard/Insights'
import HealthCard from '../dashboard/HealthCard'
import { computeHealth } from '../../utils/health'
import usePermission from '../../hooks/usePermission'
import { cn } from '../../utils/cn'
import { getOverview, getHistory, getHistoryRange, getProcesses, getInsights } from '../../api/dashboard'
import { getActivityLogs } from '../../api/activityLogs'
import { formatBytes, formatUptime } from '../../utils/format'
import { Section, Panel, RangeBar, rangeLabel, Facts } from './parts'

const POLL = 3000

function MonitorServer() {
  const navigate = useNavigate()
  const { canView } = usePermission()
  const [range, setRange] = useState('1h')
  const [sortBy, setSortBy] = useState('cpu')

  const { data: ov } = useQuery({ queryKey: ['dash-overview'], queryFn: getOverview, refetchInterval: POLL })
  const { data: hist } = useQuery({ queryKey: ['dash-history'], queryFn: getHistory, refetchInterval: POLL })
  const { data: ins } = useQuery({ queryKey: ['dash-insights'], queryFn: getInsights, refetchInterval: 30000 })
  const trend = useQuery({ queryKey: ['dash-trend', range], queryFn: () => getHistoryRange(range), refetchInterval: 60000, placeholderData: keepPreviousData })
  const { data: procs } = useQuery({ queryKey: ['dash-procs', sortBy], queryFn: () => getProcesses({ limit: 10, sort: sortBy }), refetchInterval: POLL * 4 })
  const { data: acts } = useQuery({ queryKey: ['dash-activity'], queryFn: () => getActivityLogs({ limit: 6 }), enabled: canView('activity-logs'), refetchInterval: 30000 })

  const o = ov?.data
  const live = hist?.data || []
  const insights = ins?.data
  const points = trend.data?.data || []
  const busy = trend.isFetching
  const health = useMemo(() => computeHealth(o, live, insights?.health), [o, live, insights?.health])
  const rootDisk = useMemo(() => o?.disks?.find((d) => d.mount === '/') || o?.disks?.[0], [o])

  if (!o) return <div className="space-y-4"><Skeleton className="h-24 w-full" /><Skeleton className="h-72 w-full" /></div>

  const cores = o.cpu_cores || 1
  const perCpu = o.per_cpu || []
  const mem = (() => {
    const cache = (o.mem_cached || 0) + (o.mem_buffers || 0)
    const used = Math.max(0, o.mem_used - 0)
    const free = Math.max(0, o.mem_total - used - cache)
    return { used, cache, free }
  })()
  const tcp = o.tcp || {}
  const sites = insights?.sites || []
  const peak = (k) => (points.length ? Math.max(...points.map((p) => p[k] || 0)) : 0)
  const avg = (k) => (points.length ? points.reduce((s, p) => s + (p[k] || 0), 0) / points.length : 0)

  return (
    <div className="space-y-6">
      {insights?.health && health ? (
        <HealthCard
          health={health}
          hostname={o.hostname}
          subtitle={`${o.os} · kernel ${o.kernel} · ${o.arch}`}
          right={
            <div className="flex flex-wrap items-center gap-3 text-xs text-text-secondary">
              <span className="inline-flex items-center gap-1.5"><Activity size={13} className="text-text-tertiary" />Up {formatUptime(o.uptime)}</span>
              {insights.updates?.known && <button onClick={() => navigate('/updates')} className="hover:text-text-primary hover:underline">{insights.updates.total} update{insights.updates.total === 1 ? '' : 's'}</button>}
              {insights.firewall?.known && <span className="inline-flex items-center gap-1">{insights.firewall.active ? <CircleCheck size={13} className="text-success" /> : <CircleX size={13} className="text-danger" />}Firewall {insights.firewall.active ? 'on' : 'off'}</span>}
              <AttentionButton items={insights.insights} />
            </div>
          }
        />
      ) : <Skeleton className="h-64 w-full" />}

      {/* ---- live resources ---- */}
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <StatTile label="CPU" value={o.cpu_percent.toFixed(0)} unit="%" meter={o.cpu_percent} sub={`${cores} core${cores > 1 ? 's' : ''} · ${(o.cpu_model || '').replace(/\(R\)|\(TM\)|CPU /g, '').trim()}`} trend={live.map((p) => p.cpu)} trendMax={100} />
        <StatTile label="Memory" value={o.mem_percent.toFixed(0)} unit="%" meter={o.mem_percent} sub={`${formatBytes(o.mem_used)} of ${formatBytes(o.mem_total)}`} trend={live.map((p) => p.mem)} trendMax={100} />
        <StatTile label={`Disk ${rootDisk?.mount || ''}`} value={(rootDisk?.percent || 0).toFixed(0)} unit="%" meter={rootDisk?.percent || 0} warnAt={80} sub={rootDisk ? `${formatBytes(rootDisk.total - rootDisk.used)} free of ${formatBytes(rootDisk.total)}` : '—'} />
        <StatTile label="Load average" value={o.load1.toFixed(2)} meter={(o.load5 / cores) * 100} warnAt={100} critAt={200} sub={`5m ${o.load5.toFixed(2)} · 15m ${o.load15.toFixed(2)} · ${(o.load5 / cores).toFixed(2)} per core`} trend={live.map((p) => p.load1)} />
      </div>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <StatTile label="Network" value={`${formatBytes(o.net_rx_bps)}/s`} sub={`↑ ${formatBytes(o.net_tx_bps)}/s · total ↓ ${formatBytes(o.net_rx_total)} ↑ ${formatBytes(o.net_tx_total)}`} trend={live.map((p) => p.net_rx)} />
        <StatTile label="Disk I/O" value={`${formatBytes(o.disk_read_bps)}/s`} sub={`read · write ${formatBytes(o.disk_write_bps)}/s`} trend={live.map((p) => (p.disk_read || 0) + (p.disk_write || 0))} />
        <StatTile label="Swap" value={o.swap_total ? ((o.swap_used / o.swap_total) * 100).toFixed(0) : '—'} unit={o.swap_total ? '%' : ''} meter={o.swap_total ? (o.swap_used / o.swap_total) * 100 : undefined} warnAt={30} critAt={60} sub={o.swap_total ? `${formatBytes(o.swap_used)} of ${formatBytes(o.swap_total)}` : 'No swap configured'} />
        <StatTile label="Connections" value={(tcp.established ?? 0).toLocaleString()} sub={`established · ${(tcp.listen ?? 0).toLocaleString()} listening · ${(tcp.time_wait ?? 0).toLocaleString()} waiting`} />
      </div>

      {/* ---- system facts ---- */}
      <Panel title="System">
        <Facts items={[
          ['Hostname', o.hostname], ['Operating system', o.os], ['Kernel', o.kernel], ['Architecture', o.arch],
          ['Processor', (o.cpu_model || '—').replace(/\(R\)|\(TM\)/g, '')], ['Cores', `${cores}`], ['Processes', (o.processes || 0).toLocaleString()],
          ['Uptime', formatUptime(o.uptime)],
          o.boot_time ? ['Booted', new Date(o.boot_time * 1000).toLocaleString(undefined, { day: '2-digit', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false })] : null,
          ['Memory', formatBytes(o.mem_total)], ['Swap', o.swap_total ? formatBytes(o.swap_total) : 'none'],
          ['Primary IP', (o.interfaces || []).find((i) => i.ip)?.ip || '—'],
        ]} />
      </Panel>

      <RangeBar range={range} setRange={setRange} right={`${rangeLabel(range)}${busy ? ' · updating…' : ''}`} />

      {/* ---- history ---- */}
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
          <ChartCard
            busy={busy} title="Disk I/O" subtitle="Read and write throughput of the physical disks"
            legend={[<LegendItem key="r" shape="line" color="var(--viz-1)" label="Read" value={`${formatBytes(o.disk_read_bps)}/s`} />, <LegendItem key="w" shape="line" color="var(--viz-2)" label="Write" value={`${formatBytes(o.disk_write_bps)}/s`} />]}
            table={{ columns: ['Time', 'Read', 'Write'], rows: points.map((p) => [tooltipTime(p.t, 0), `${formatBytes(p.disk_read || 0)}/s`, `${formatBytes(p.disk_write || 0)}/s`]) }}
          >
            <LineChart points={points} fmt={(v) => `${formatBytes(v)}/s`} series={[{ key: 'disk_read', label: 'Read', color: 'var(--viz-1)' }, { key: 'disk_write', label: 'Write', color: 'var(--viz-2)' }]} ariaLabel={`Disk throughput over the ${rangeLabel(range)}`} />
          </ChartCard>
          <ChartCard
            busy={busy} title={`Disk usage ${rootDisk?.mount || ''}`} subtitle="How full the main file system is"
            legend={[<LegendItem key="d" shape="line" color="var(--viz-1)" label="Used" value={`${(rootDisk?.percent || 0).toFixed(0)}%`} />]}
            table={{ columns: ['Time', 'Used %'], rows: points.map((p) => [tooltipTime(p.t, 0), (p.disk || 0).toFixed(1)]) }}
          >
            <LineChart points={points} yMax={100} area fmt={(v) => `${Math.round(v)}%`} series={[{ key: 'disk', label: 'Used', color: 'var(--viz-1)' }]} ariaLabel={`Disk usage over the ${rangeLabel(range)}`} />
          </ChartCard>
          <Panel title="Peaks in this range" hint={rangeLabel(range)}>
            {points.length > 1 ? (
              <ul className="divide-y divide-surface-border text-sm">
                {[
                  ['CPU', `${peak('cpu').toFixed(0)}%`, `avg ${avg('cpu').toFixed(0)}%`],
                  ['Memory', `${peak('mem').toFixed(0)}%`, `avg ${avg('mem').toFixed(0)}%`],
                  ['Load (1m)', peak('load1').toFixed(2), `avg ${avg('load1').toFixed(2)}`],
                  ['Download', `${formatBytes(peak('net_rx'))}/s`, `avg ${formatBytes(avg('net_rx'))}/s`],
                  ['Upload', `${formatBytes(peak('net_tx'))}/s`, `avg ${formatBytes(avg('net_tx'))}/s`],
                  ['Disk read', `${formatBytes(peak('disk_read'))}/s`, `avg ${formatBytes(avg('disk_read'))}/s`],
                  ['Disk write', `${formatBytes(peak('disk_write'))}/s`, `avg ${formatBytes(avg('disk_write'))}/s`],
                ].map(([k, v, a]) => (
                  <li key={k} className="flex items-baseline justify-between py-2"><span className="text-text-secondary">{k}</span><span className="tabular-nums text-text-primary"><b className="font-semibold">{v}</b> <span className="ml-2 text-xs text-text-tertiary">{a}</span></span></li>
                ))}
              </ul>
            ) : <p className="py-8 text-center text-xs text-text-tertiary">Collecting data…</p>}
          </Panel>
        </div>
      </Section>

      {/* ---- live breakdowns ---- */}
      <Section title="Right now">
        <div className="grid grid-cols-1 gap-3 xl:grid-cols-3">
          <Panel title="CPU per core" hint={`${cores} core${cores > 1 ? 's' : ''}`}>
            {perCpu.length ? (
              <ul className="grid gap-x-4 gap-y-2.5 sm:grid-cols-2">
                {perCpu.map((p, i) => (
                  <li key={i}>
                    <div className="mb-1 flex items-baseline justify-between text-xs"><span className="text-text-secondary">Core {i + 1}</span><span className="tabular-nums font-medium text-text-primary" style={{ color: loadColor(p) }}>{p.toFixed(0)}%</span></div>
                    <Meter percent={p} />
                  </li>
                ))}
              </ul>
            ) : <p className="py-6 text-center text-xs text-text-tertiary">No per-core data</p>}
          </Panel>
          <Panel title="Memory breakdown" hint={`${formatBytes(o.mem_total)} installed`}>
            <SplitBar items={[
              { key: 'used', label: 'In use by programs', value: mem.used, text: formatBytes(mem.used), color: 'var(--viz-1)' },
              { key: 'cache', label: 'Cache & buffers', value: mem.cache, text: formatBytes(mem.cache), color: 'var(--viz-2)' },
              { key: 'free', label: 'Free', value: mem.free, text: formatBytes(mem.free), color: 'rgb(var(--c-border-hover))' },
            ]} />
            <p className="mt-3 text-[11px] text-text-tertiary">Cache is memory Linux borrows for speed and gives back when programs need it. {formatBytes(o.mem_available)} is available to start new work.</p>
          </Panel>
          <Panel title="Connections" hint="TCP sockets by state">
            <HBars items={[
              ['Established', tcp.established], ['Listening', tcp.listen], ['Time-wait', tcp.time_wait], ['Close-wait', tcp.close_wait], ['SYN received', tcp.syn_recv], ['Other', tcp.other],
            ].filter(([, v]) => v > 0).map(([k, v]) => ({ key: k, label: k, value: v, text: compact(v), color: k === 'Close-wait' || k === 'SYN received' ? 'var(--viz-warn)' : undefined }))} empty="No connection data" />
            <p className="mt-3 text-[11px] text-text-tertiary">{(tcp.total || 0).toLocaleString()} sockets in total. Many close-wait or SYN-received entries can point to an overloaded or attacked service.</p>
          </Panel>
        </div>
      </Section>

      <div className="grid grid-cols-1 gap-3 xl:grid-cols-3">
        <Panel title="Storage" className="xl:col-span-1">
          <div className="space-y-4">
            {o.disks.map((d) => (
              <div key={d.mount}>
                <div className="mb-1.5 flex items-baseline justify-between gap-2 text-xs">
                  <span className="truncate font-mono text-text-primary" title={d.fstype}>{d.mount} <span className="font-sans text-text-tertiary">{d.fstype}</span></span>
                  <span className="shrink-0 text-text-secondary">{formatBytes(d.used)} / {formatBytes(d.total)} · <b className="font-semibold text-text-primary">{d.percent.toFixed(0)}%</b></span>
                </div>
                <Meter percent={d.percent} warnAt={80} />
                {d.inodes_percent > 0 && <p className="mt-1 text-[11px] text-text-tertiary">{d.inodes_percent.toFixed(0)}% of inodes used{d.inodes_percent > 85 ? ' — running out of file slots' : ''}</p>}
              </div>
            ))}
          </div>
        </Panel>
        <Panel title="Network interfaces" className="xl:col-span-2" action={<Network size={15} className="text-text-tertiary" />}>
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead className="text-left text-text-tertiary"><tr><th className="pb-2 font-medium">Interface</th><th className="pb-2 font-medium">IPv4</th><th className="pb-2 text-right font-medium">Download</th><th className="pb-2 text-right font-medium">Upload</th><th className="pb-2 text-right font-medium">Received</th><th className="pb-2 text-right font-medium">Sent</th></tr></thead>
              <tbody>
                {(o.interfaces || []).map((n) => (
                  <tr key={n.name} className="border-t border-surface-border">
                    <td className="py-2 pr-3 font-mono text-text-primary">{n.name}</td>
                    <td className="py-2 pr-3 font-mono text-text-secondary">{n.ip || '—'}</td>
                    <td className="py-2 text-right tabular-nums text-text-primary">{formatBytes(n.rx_bps)}/s</td>
                    <td className="py-2 text-right tabular-nums text-text-primary">{formatBytes(n.tx_bps)}/s</td>
                    <td className="py-2 text-right tabular-nums text-text-secondary">{formatBytes(n.rx_total)}</td>
                    <td className="py-2 text-right tabular-nums text-text-secondary">{formatBytes(n.tx_total)}</td>
                  </tr>
                ))}
                {!(o.interfaces || []).length && <tr><td colSpan={6} className="py-8 text-center text-text-tertiary">No interface data</td></tr>}
              </tbody>
            </table>
          </div>
        </Panel>
      </div>

      <div className="grid grid-cols-1 gap-3 xl:grid-cols-3">
        <Panel
          title="Top processes" className="xl:col-span-2"
          action={
            <div className="inline-flex rounded-md border border-surface-border p-0.5" role="group" aria-label="Sort processes">
              {[['cpu', 'CPU'], ['mem', 'Memory']].map(([k, label]) => <button key={k} onClick={() => setSortBy(k)} aria-pressed={sortBy === k} className={cn('h-6 rounded-[7px] px-2.5 text-[11px] font-medium', sortBy === k ? 'bg-primary text-white' : 'text-text-secondary hover:bg-surface-hover')}>{label}</button>)}
            </div>
          }
        >
          <table className="w-full text-xs">
            <thead className="text-left text-text-tertiary"><tr><th className="pb-1.5 font-medium">Process</th><th className="pb-1.5 font-medium">PID</th><th className="pb-1.5 font-medium">User</th><th className="pb-1.5 text-right font-medium">CPU</th><th className="pb-1.5 text-right font-medium">Memory</th></tr></thead>
            <tbody>
              {(procs?.data || []).map((p) => (
                <tr key={p.pid} className="border-t border-surface-border">
                  <td className="max-w-[12rem] truncate py-1.5 pr-2 text-text-primary">{p.name}</td>
                  <td className="pr-2 font-mono text-text-tertiary">{p.pid}</td>
                  <td className="max-w-[6rem] truncate pr-2 text-text-secondary">{p.user}</td>
                  <td className={cn('text-right tabular-nums', sortBy === 'cpu' ? 'font-medium text-text-primary' : 'text-text-secondary')}>{p.cpu.toFixed(1)}%</td>
                  <td className={cn('text-right tabular-nums', sortBy === 'mem' ? 'font-medium text-text-primary' : 'text-text-secondary')}>{p.mem.toFixed(1)}%</td>
                </tr>
              ))}
            </tbody>
          </table>
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

      <div className="grid grid-cols-1 gap-3 xl:grid-cols-3">
        <Panel title="Websites" className="xl:col-span-2" action={<button onClick={() => navigate('/websites')} className="text-xs font-medium text-primary hover:underline">Manage →</button>}>
          {sites.length === 0 ? <p className="py-8 text-center text-xs text-text-tertiary">No websites yet</p> : (
            <div className="overflow-x-auto">
              <table className="w-full text-xs">
                <thead className="text-left text-text-tertiary"><tr><th className="pb-2 font-medium">Domain</th><th className="pb-2 font-medium">Status</th><th className="pb-2 font-medium">HTTPS certificate</th></tr></thead>
                <tbody>
                  {sites.map((s) => (
                    <tr key={s.domain} className="border-t border-surface-border">
                      <td className="py-2 pr-3 font-medium text-text-primary">{s.domain}</td>
                      <td className="py-2 pr-3">{s.status === 'running' ? <Badge variant="success">running</Badge> : <Badge>stopped</Badge>}</td>
                      <td className="py-2 pr-3">
                        {!s.ssl_enabled ? <span className="inline-flex items-center gap-1 text-text-tertiary"><LockOpen size={12} />No HTTPS</span>
                          : <span className={cn('inline-flex items-center gap-1', s.days_left !== null && s.days_left < 14 ? (s.days_left < 7 ? 'text-danger' : 'text-warning') : 'text-text-secondary')}><Lock size={12} />{s.days_left === null ? 'On' : s.days_left < 0 ? 'Expired' : `valid for ${s.days_left} days`}</span>}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
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

export default MonitorServer
