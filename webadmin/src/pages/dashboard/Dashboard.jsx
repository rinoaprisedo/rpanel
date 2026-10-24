import { useQuery } from '@tanstack/react-query'
import Skeleton from '../../components/ui/Skeleton'
import Sparkline from '../../components/ui/Sparkline'
import { getOverview, getHistory, getProcesses } from '../../api/dashboard'
import { formatBytes, formatUptime } from '../../utils/format'

const POLL = 3000

function Card({ title, children }) {
  return (
    <div className="rounded-lg border border-surface-border bg-surface-card p-4">
      <p className="mb-2 text-xs font-medium uppercase tracking-wide text-text-secondary">{title}</p>
      {children}
    </div>
  )
}

function Bar({ percent }) {
  const p = Math.max(0, Math.min(100, percent || 0))
  return (
    <div className="h-1.5 w-full rounded-full bg-surface-hover">
      <div className={`h-1.5 rounded-full ${p > 90 ? 'bg-danger' : p > 75 ? 'bg-warning' : 'bg-primary'}`} style={{ width: `${p}%` }} />
    </div>
  )
}

function Dashboard() {
  const { data: ov } = useQuery({ queryKey: ['dash-overview'], queryFn: getOverview, refetchInterval: POLL })
  const { data: hist } = useQuery({ queryKey: ['dash-history'], queryFn: getHistory, refetchInterval: POLL })
  const { data: procs } = useQuery({ queryKey: ['dash-procs'], queryFn: () => getProcesses({ limit: 8 }), refetchInterval: POLL * 3 })

  const o = ov?.data
  const points = hist?.data || []
  if (!o) return <Skeleton className="h-96 w-full" />

  return (
    <div className="space-y-4">
      <div className="rounded-lg border border-surface-border bg-surface-card p-4 text-sm text-text-secondary">
        <span className="font-medium text-text-primary">{o.hostname}</span> · {o.os} · kernel {o.kernel} · {o.arch} · up {formatUptime(o.uptime)}
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
        <Card title={`CPU (${o.cpu_cores} cores)`}>
          <p className="text-2xl font-semibold text-text-primary">{o.cpu_percent.toFixed(1)}%</p>
          <Bar percent={o.cpu_percent} />
          <p className="mt-2 truncate text-xs text-text-secondary">{o.cpu_model}</p>
        </Card>
        <Card title="Memory">
          <p className="text-2xl font-semibold text-text-primary">{o.mem_percent.toFixed(1)}%</p>
          <Bar percent={o.mem_percent} />
          <p className="mt-2 text-xs text-text-secondary">
            {formatBytes(o.mem_used)} / {formatBytes(o.mem_total)} · swap {formatBytes(o.swap_used)} / {formatBytes(o.swap_total)}
          </p>
        </Card>
        <Card title="Load average">
          <p className="text-2xl font-semibold text-text-primary">{o.load1.toFixed(2)}</p>
          <p className="mt-2 text-xs text-text-secondary">5m {o.load5.toFixed(2)} · 15m {o.load15.toFixed(2)}</p>
        </Card>
        <Card title="Network">
          <p className="text-sm text-text-primary">↓ {formatBytes(o.net_rx_bps)}/s · ↑ {formatBytes(o.net_tx_bps)}/s</p>
          <p className="mt-2 text-xs text-text-secondary">total ↓ {formatBytes(o.net_rx_total)} · ↑ {formatBytes(o.net_tx_total)}</p>
        </Card>
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <Card title="CPU & memory (last 5 min)">
          <Sparkline
            max={100}
            series={[
              { label: 'cpu', color: '#3b82f6', values: points.map((p) => p.cpu) },
              { label: 'mem', color: '#10b981', values: points.map((p) => p.mem) },
            ]}
          />
          <p className="text-xs text-text-secondary"><span className="text-blue-500">━</span> CPU <span className="ml-2 text-emerald-500">━</span> Memory</p>
        </Card>
        <Card title="Network (last 5 min)">
          <Sparkline
            series={[
              { label: 'rx', color: '#3b82f6', values: points.map((p) => p.net_rx) },
              { label: 'tx', color: '#f59e0b', values: points.map((p) => p.net_tx) },
            ]}
          />
          <p className="text-xs text-text-secondary"><span className="text-blue-500">━</span> Download <span className="ml-2 text-amber-500">━</span> Upload</p>
        </Card>
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <Card title="Disks">
          <div className="space-y-3">
            {o.disks.map((d) => (
              <div key={d.mount}>
                <div className="mb-1 flex justify-between text-xs text-text-secondary">
                  <span className="text-text-primary">{d.mount} <span className="text-text-secondary">({d.fstype})</span></span>
                  <span>{formatBytes(d.used)} / {formatBytes(d.total)} · {d.percent.toFixed(0)}%</span>
                </div>
                <Bar percent={d.percent} />
              </div>
            ))}
          </div>
        </Card>
        <Card title="Top processes">
          <table className="w-full text-xs">
            <thead className="text-left text-text-secondary">
              <tr><th className="pb-1">PID</th><th>Name</th><th>User</th><th className="text-right">CPU%</th><th className="text-right">MEM%</th></tr>
            </thead>
            <tbody className="text-text-primary">
              {(procs?.data || []).map((p) => (
                <tr key={p.pid}>
                  <td className="py-0.5">{p.pid}</td><td className="max-w-[8rem] truncate">{p.name}</td><td>{p.user}</td>
                  <td className="text-right">{p.cpu.toFixed(1)}</td><td className="text-right">{p.mem.toFixed(1)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
      </div>
    </div>
  )
}

export default Dashboard
