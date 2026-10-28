import { useState } from 'react'
import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { Radio, Bot, Smartphone, Monitor as MonitorIcon, Tablet } from 'lucide-react'
import Skeleton from '../../components/ui/Skeleton'
import StatTile from '../../components/charts/StatTile'
import ChartCard, { LegendItem } from '../../components/charts/ChartCard'
import StackedColumns from '../../components/charts/StackedColumns'
import LineChart from '../../components/charts/LineChart'
import HBars from '../../components/charts/HBars'
import Columns from '../../components/charts/Columns'
import SplitBar from '../../components/charts/SplitBar'
import { compact, pct, tooltipTime } from '../../components/charts/scale'
import { cn } from '../../utils/cn'
import { getTraffic } from '../../api/dashboard'
import { formatBytes } from '../../utils/format'
import { Section, Panel, RangeBar, rangeLabel, pctDelta } from './parts'

// The traffic classes mean good/bad, so they use the fixed status roles (never a free series hue).
const C_OK = 'var(--viz-1)'
const C_CLIENT = 'var(--viz-warn)'
const C_SERVER = 'var(--viz-crit)'
const C_A = 'var(--viz-1)'
const C_B = 'var(--viz-2)'
const C_MUTED = 'rgb(var(--c-border-hover))'
const WEEKDAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
const DEVICE_ICON = { Desktop: MonitorIcon, Mobile: Smartphone, Tablet, Bot }

const statusTone = (c) => (c >= 500 ? 'text-danger' : c >= 400 ? 'text-warning' : c >= 300 ? 'text-info' : 'text-success')
const clock = (t) => new Date(t * 1000).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })
const items = (list, total, extra) => (list || []).map((x) => ({ key: x.key, label: x.key, value: x.count, text: compact(x.count), sub: total ? pct(x.count, total, 0) : undefined, ...(extra?.(x) || {}) }))

function MonitorTraffic() {
  const navigate = useNavigate()
  const [range, setRange] = useState('24h')
  const [site, setSite] = useState('')

  const traffic = useQuery({ queryKey: ['dash-traffic', range, site], queryFn: () => getTraffic(range, site), refetchInterval: 30000, placeholderData: keepPreviousData })
  const t = traffic.data?.data
  const busy = traffic.isFetching

  if (!t) return <div className="space-y-4"><Skeleton className="h-16 w-full" /><Skeleton className="h-72 w-full" /></div>

  const tot = t.totals
  const prev = t.previous
  const domains = t.domains || []
  const hasTraffic = t.available && tot.requests > 0
  const errRate = tot.requests ? (tot.server_err / tot.requests) * 100 : 0
  const prevErrRate = prev?.requests ? (prev.server_err / prev.requests) * 100 : null
  const errDelta = prevErrRate === null ? null : (() => {
    const d = errRate - prevErrRate
    return Math.abs(d) < 0.05 ? { text: '0 pp', dir: 'flat' } : { text: `${d > 0 ? '+' : '−'}${Math.abs(d).toFixed(1)} pp`, dir: d > 0 ? 'up' : 'down', good: 'down' }
  })()
  const vsNote = prev ? `vs previous ${range === '7d' ? '7 days' : range === '1h' ? 'hour' : range}` : 'no earlier data yet'
  const withGood = (d, good) => (d ? { ...d, good } : null)
  const daily = t.bucket_sec >= 86400
  const bucketText = daily ? 'day' : t.bucket_sec >= 3600 ? `${t.bucket_sec / 3600} hour${t.bucket_sec > 3600 ? 's' : ''}` : `${t.bucket_sec / 60} minutes`
  const spanSec = t.until - t.since
  const humans = Math.max(0, tot.requests - tot.bots)

  const trafficSeries = [
    { key: 'ok', label: 'Successful (2xx–3xx)', color: C_OK },
    { key: 'client_err', label: 'Client errors (4xx)', color: C_CLIENT },
    { key: 'server_err', label: 'Server errors (5xx)', color: C_SERVER },
  ]
  const classColor = { '2xx': C_OK, '3xx': 'var(--viz-2)', '4xx': C_CLIENT, '5xx': C_SERVER }
  const buckets = t.buckets || []
  const peakHour = t.hours?.indexOf(Math.max(...(t.hours || [0]))) ?? 0
  const quietHour = t.hours?.indexOf(Math.min(...(t.hours || [0]))) ?? 0
  const peakDay = t.weekdays?.indexOf(Math.max(...(t.weekdays || [0]))) ?? 0
  const hh = (h) => `${String(h).padStart(2, '0')}:00`

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-lg font-semibold text-text-primary">Traffic</h1>
          <p className="text-xs text-text-tertiary">Visits to your websites, read from the nginx access logs{t.truncated ? ' (very large logs: newest part only)' : ''}</p>
        </div>
        <div className="inline-flex items-center gap-2 rounded-full border border-surface-border bg-surface-card px-3 py-1.5 text-xs text-text-secondary" title="Requests in the last 5 minutes">
          <span className="relative flex h-2 w-2"><span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-success opacity-60" /><span className="relative inline-flex h-2 w-2 rounded-full bg-success" /></span>
          <Radio size={13} className="text-text-tertiary" />
          <b className="font-semibold text-text-primary tabular-nums">{t.live.per_min.toFixed(1)}</b> req/min · <b className="font-semibold text-text-primary tabular-nums">{t.live.visitors}</b> visitor{t.live.visitors === 1 ? '' : 's'} in the last 5 min
        </div>
      </div>

      <RangeBar range={range} setRange={setRange} right={`${rangeLabel(range)}${busy ? ' · updating…' : ''} · times in ${t.timezone}`}>
        <select value={site} onChange={(e) => setSite(e.target.value)} aria-label="Website" className="h-8 rounded-md border border-surface-border bg-field px-2.5 text-xs text-text-primary outline-none focus:border-primary">
          <option value="">All websites</option>
          {domains.map((d) => <option key={d} value={d}>{d}</option>)}
        </select>
      </RangeBar>

      {!hasTraffic ? (
        <div className="rounded-lg border border-dashed border-surface-border-hover p-12 text-center">
          <p className="text-sm font-medium text-text-primary">{!domains.length ? 'No websites yet' : 'No requests in this period'}</p>
          <p className="mx-auto mt-1 max-w-md text-xs text-text-secondary">
            {!domains.length ? 'Add a website and its visitors, errors and top pages show up here.' : 'Traffic is read from each site’s nginx access log. Try a longer range, or wait for the first visits.'}
          </p>
          {!domains.length && <button onClick={() => navigate('/websites')} className="mt-3 text-xs font-medium text-primary hover:underline">Add a website →</button>}
        </div>
      ) : (
        <>
          {/* ---- headline numbers ---- */}
          <div className={cn('grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-6', busy && 'opacity-60 transition-opacity')}>
            <StatTile label="Requests" value={compact(tot.requests)} delta={withGood(pctDelta(tot.requests, prev?.requests), 'up')} deltaNote={vsNote} sub={`${compact(Math.round(tot.requests / Math.max(1, spanSec / 3600)))} per hour on average`} />
            <StatTile label="Page views" value={compact(tot.page_views)} delta={withGood(pctDelta(tot.page_views, prev?.page_views), 'up')} deltaNote={vsNote} sub={`${pct(tot.page_views, tot.requests, 0)} of requests are pages`} />
            <StatTile label="Unique visitors" value={compact(tot.visitors)} delta={withGood(pctDelta(tot.visitors, prev?.visitors), 'up')} deltaNote={vsNote} sub={`${(tot.page_views / Math.max(1, tot.visitors)).toFixed(1)} pages per visitor`} />
            <StatTile label="Data served" value={formatBytes(tot.bytes)} delta={pctDelta(tot.bytes, prev?.bytes)} deltaNote={vsNote} sub={`${formatBytes(tot.bytes / Math.max(1, tot.requests))} per request`} />
            <StatTile label="Bots & scripts" value={pct(tot.bots, tot.requests, 0)} sub={`${compact(tot.bots)} of ${compact(tot.requests)} requests`} meter={(tot.bots / Math.max(1, tot.requests)) * 100} warnAt={60} critAt={85} />
            <StatTile label="Server errors (5xx)" value={errRate.toFixed(errRate < 10 ? 2 : 1)} unit="%" delta={errDelta} deltaNote={vsNote} sub={`${tot.server_err.toLocaleString()} 5xx · ${tot.client_err.toLocaleString()} 4xx`} />
          </div>

          {/* ---- over time ---- */}
          <Section title="Over time">
            <div className="grid grid-cols-1 gap-3 xl:grid-cols-3">
              <ChartCard
                className="xl:col-span-2" busy={busy} title="Requests" subtitle={`Per ${bucketText}, split by outcome`}
                legend={trafficSeries.map((s) => <LegendItem key={s.key} color={s.color} label={s.label} value={buckets.reduce((n, b) => n + (b[s.key] || 0), 0).toLocaleString()} />)}
                table={{ columns: ['Time', 'Successful', '4xx', '5xx'], rows: buckets.map((b) => [tooltipTime(b.t, spanSec, daily), b.ok, b.client_err, b.server_err]) }}
              >
                <StackedColumns buckets={buckets} series={trafficSeries} bucketSec={t.bucket_sec} daily={daily} ariaLabel={`Requests over the ${rangeLabel(range)}, split into successful, client error and server error responses`} />
              </ChartCard>
              <Panel title="Status classes" hint="How requests were answered">
                <SplitBar items={(t.classes || []).map((c) => ({ key: c.key, label: { '2xx': 'Success (2xx)', '3xx': 'Redirect (3xx)', '4xx': 'Client error (4xx)', '5xx': 'Server error (5xx)' }[c.key], value: c.count, text: compact(c.count), color: classColor[c.key] }))} />
                <div className="mt-4 border-t border-surface-border pt-3">
                  <p className="mb-2 text-xs font-medium text-text-secondary">Top response codes</p>
                  <HBars items={(t.statuses || []).slice(0, 5).map((s) => ({ key: s.key, label: s.key, value: s.count, text: compact(s.count), sub: pct(s.count, tot.requests, 0), color: Number(s.key) >= 500 ? C_SERVER : Number(s.key) >= 400 ? C_CLIENT : C_OK }))} />
                </div>
              </Panel>
              <ChartCard
                busy={busy} title="Unique visitors" subtitle={`Distinct clients per ${bucketText}`}
                legend={[<LegendItem key="v" shape="line" color={C_A} label="Visitors" value={tot.visitors.toLocaleString()} />]}
                table={{ columns: ['Time', 'Visitors'], rows: buckets.map((b) => [tooltipTime(b.t, spanSec, daily), b.visitors]) }}
              >
                <LineChart points={buckets} area fmt={(v) => compact(Math.round(v))} series={[{ key: 'visitors', label: 'Visitors', color: C_A }]} ariaLabel={`Unique visitors over the ${rangeLabel(range)}`} />
              </ChartCard>
              <ChartCard
                busy={busy} title="Bandwidth" subtitle={`Data sent to visitors per ${bucketText}`}
                legend={[<LegendItem key="b" shape="line" color={C_B} label="Data" value={formatBytes(tot.bytes)} />]}
                table={{ columns: ['Time', 'Data'], rows: buckets.map((b) => [tooltipTime(b.t, spanSec, daily), formatBytes(b.bytes)]) }}
              >
                <LineChart points={buckets} area fmt={(v) => formatBytes(v)} series={[{ key: 'bytes', label: 'Data', color: C_B }]} ariaLabel={`Bandwidth over the ${rangeLabel(range)}`} />
              </ChartCard>
              <Panel title="Humans vs bots" hint="By requests">
                <SplitBar items={[
                  { key: 'h', label: 'People', value: humans, text: compact(humans), color: C_A },
                  { key: 'b', label: 'Bots & scripts', value: tot.bots, text: compact(tot.bots), color: C_B },
                ]} />
                <div className="mt-4 border-t border-surface-border pt-3">
                  <p className="mb-2 flex items-center gap-1.5 text-xs font-medium text-text-secondary"><Bot size={13} />Most active bots</p>
                  <HBars items={items(t.bots, tot.bots)} empty="No bots seen" />
                </div>
              </Panel>
            </div>
          </Section>

          {/* ---- patterns ---- */}
          <Section title="When people visit" hint={`Server time (${t.timezone})`}>
            <div className="grid grid-cols-1 gap-3 xl:grid-cols-3">
              <Panel title="By hour of day" className="xl:col-span-2" hint={`Busiest ${hh(peakHour)}, quietest ${hh(quietHour)}`}>
                <Columns values={t.hours} labels={Array.from({ length: 24 }, (_, i) => String(i).padStart(2, '0'))} every={2} ariaLabel="Requests by hour of the day" />
              </Panel>
              <Panel title="By weekday" hint={t.weekdays[peakDay] ? `Busiest on ${WEEKDAYS[peakDay]}` : undefined}>
                <Columns values={t.weekdays} labels={WEEKDAYS} color={C_B} ariaLabel="Requests by weekday" />
              </Panel>
            </div>
          </Section>

          {/* ---- content ---- */}
          <Section title="What they visit">
            <div className="grid grid-cols-1 gap-3 lg:grid-cols-3">
              <Panel title="Top pages" hint="Hits, without images & scripts">
                <HBars items={(t.top_paths || []).map((p) => ({ key: p.key, label: p.key, value: p.count, text: compact(p.count), sub: p.errors ? `${p.errors} err` : undefined }))} />
              </Panel>
              <Panel title="Top referrers" hint="Where visitors come from">
                <HBars items={items(t.referrers, tot.requests)} empty="No external referrers (direct visits or hidden)" />
              </Panel>
              <Panel title="Top clients" hint="Busiest IP addresses">
                <HBars items={(t.top_ips || []).map((p) => ({ key: p.key, label: p.key, value: p.count, text: compact(p.count), sub: pct(p.count, tot.requests, 0) }))} />
              </Panel>
            </div>
          </Section>

          {/* ---- audience ---- */}
          <Section title="Who visits" hint="People only (bots excluded) unless noted">
            <div className="grid grid-cols-1 gap-3 lg:grid-cols-2 xl:grid-cols-4">
              <Panel title="Devices" hint="Including bots">
                <SplitBar items={(t.devices || []).map((d) => ({ key: d.key, label: d.key, value: d.count, text: compact(d.count), color: { Desktop: C_A, Mobile: C_B, Tablet: 'var(--viz-3, #7c6fd6)', Bot: C_MUTED }[d.key] || C_MUTED }))} />
                <ul className="mt-3 flex flex-wrap gap-3 text-[11px] text-text-tertiary">
                  {(t.devices || []).map((d) => { const I = DEVICE_ICON[d.key] || MonitorIcon; return <li key={d.key} className="inline-flex items-center gap-1"><I size={12} />{d.key}</li> })}
                </ul>
              </Panel>
              <Panel title="Browsers"><HBars items={items(t.browsers, humans)} /></Panel>
              <Panel title="Operating systems"><HBars items={items(t.systems, humans)} /></Panel>
              <Panel title="HTTP methods"><HBars items={items(t.methods, tot.requests)} /></Panel>
            </div>
          </Section>

          {/* ---- problems ---- */}
          <Section title="Problems worth a look">
            <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
              <Panel title="Most requested missing pages (404)" hint="Broken links, or scanners probing for files">
                <HBars items={items(t.not_found)} color="var(--viz-warn)" empty="No missing pages — nice" />
              </Panel>
              <Panel title="Pages answering with server errors (5xx)" hint="Fix these first">
                <HBars items={items(t.error_paths)} color="var(--viz-crit)" empty="No server errors — nice" />
              </Panel>
            </div>
          </Section>

          {/* ---- websites + live feed ---- */}
          <div className="grid grid-cols-1 gap-3 xl:grid-cols-5">
            <Panel title="Websites" className="xl:col-span-2" action={<button onClick={() => navigate('/websites')} className="text-xs font-medium text-primary hover:underline">Manage →</button>}>
              <div className="overflow-x-auto">
                <table className="w-full text-xs">
                  <thead className="text-left text-text-tertiary"><tr><th className="pb-2 font-medium">Domain</th><th className="pb-2 text-right font-medium">Requests</th><th className="pb-2 text-right font-medium">Visitors</th><th className="pb-2 text-right font-medium">4xx</th><th className="pb-2 text-right font-medium">5xx</th><th className="pb-2 text-right font-medium">Data</th></tr></thead>
                  <tbody>
                    {(t.sites || []).map((s) => (
                      <tr key={s.domain} className="border-t border-surface-border">
                        <td className="py-2 pr-3 font-medium text-text-primary"><button onClick={() => setSite(s.domain)} className="hover:text-primary hover:underline" title="Filter to this website">{s.domain}</button></td>
                        <td className="py-2 text-right tabular-nums text-text-primary">{s.requests.toLocaleString()}</td>
                        <td className="py-2 text-right tabular-nums text-text-secondary">{s.visitors.toLocaleString()}</td>
                        <td className="py-2 text-right tabular-nums text-text-secondary">{s.client_err.toLocaleString()}</td>
                        <td className={cn('py-2 text-right tabular-nums', s.server_err ? 'font-medium text-danger' : 'text-text-secondary')}>{s.server_err.toLocaleString()}</td>
                        <td className="py-2 text-right tabular-nums text-text-secondary">{formatBytes(s.bytes)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </Panel>
            <Panel title="Latest requests" className="xl:col-span-3" hint="Newest first, last 10 minutes" action={<button onClick={() => navigate('/logs')} className="text-xs font-medium text-primary hover:underline">All logs →</button>}>
              {(t.latest || []).length === 0 ? <p className="py-8 text-center text-xs text-text-tertiary">No request in the last 10 minutes</p> : (
                <div className="overflow-x-auto">
                  <table className="w-full text-xs">
                    <thead className="text-left text-text-tertiary"><tr><th className="pb-2 font-medium">Time</th><th className="pb-2 font-medium">Site</th><th className="pb-2 font-medium">Client</th><th className="pb-2 font-medium">Request</th><th className="pb-2 text-right font-medium">Status</th><th className="pb-2 text-right font-medium">Size</th></tr></thead>
                    <tbody>
                      {t.latest.map((r, i) => (
                        <tr key={`${r.t}-${i}`} className="border-t border-surface-border">
                          <td className="whitespace-nowrap py-1.5 pr-3 tabular-nums text-text-tertiary">{clock(r.t)}</td>
                          <td className="max-w-[8rem] truncate pr-3 text-text-secondary">{r.site}</td>
                          <td className="pr-3"><span className="font-mono text-text-primary">{r.ip}</span><span className="ml-1.5 text-text-tertiary">{r.client}</span></td>
                          <td className="max-w-[14rem] truncate pr-3 font-mono text-text-primary" title={`${r.method} ${r.path}`}><span className="text-text-tertiary">{r.method}</span> {r.path}</td>
                          <td className={cn('text-right font-medium tabular-nums', statusTone(r.status))}>{r.status}</td>
                          <td className="pl-3 text-right tabular-nums text-text-secondary">{formatBytes(r.bytes)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </Panel>
          </div>
        </>
      )}
    </div>
  )
}

export default MonitorTraffic
