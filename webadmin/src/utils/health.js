// Client-side twin of backend/modules/dashboard/health.go, so the gauge can follow
// the 3-second overview poll. Keep the thresholds and weights in sync with it.
// Factors that need slower probes (5xx rate from logs, service status) are taken
// from the server's own verdict.

import { formatBytes } from './format'

const CPU_BURST_SAMPLES = 5 // 15 s of 3 s samples; keep in sync with cpuBurstSamples in health.go
const lowerBetter = (v, good, bad) => (v <= good ? 100 : v >= bad ? 0 : (100 * (bad - v)) / (bad - good))
const clamp = (v) => Math.max(0, Math.min(100, v))

export function levelFor(score) {
  if (score >= 85) return ['smooth', 'Running smoothly']
  if (score >= 65) return ['fair', 'Slightly busy']
  if (score >= 40) return ['slow', 'Running slow']
  return ['struggle', 'Struggling']
}

// o: /dashboard/overview data, recent: /dashboard/history points, server: insights.health (or undefined)
export function computeHealth(o, recent = [], server) {
  if (!o) return server
  const factors = []
  const add = (id, label, value, used, score, weight, hint) =>
    factors.push({ id, label, value, used: Math.round(clamp(used)), score: Math.round(score), weight, hint })

  // Sustained use matters, one spike does not; but a spike that lasts is felt at once, so the last 15 seconds
  // (5 samples, 3 s apart) count too and the worse of the two wins.
  const mean = (pts) => pts.reduce((s, p) => s + p.cpu, 0) / pts.length
  const cpu = recent.length >= 3 ? Math.max(mean(recent), mean(recent.slice(-CPU_BURST_SAMPLES))) : o.cpu_percent
  add('cpu', 'CPU', `${cpu.toFixed(0)}%`, cpu, lowerBetter(cpu, 60, 100), 25, 'Worse of the last few minutes and the last 15 seconds. Fine below 60%, saturated at 100%')

  const perCore = o.load5 / Math.max(1, o.cpu_cores || 1)
  add('load', 'Load', `${perCore.toFixed(2)} / core`, perCore * 100, lowerBetter(perCore, 0.7, 2), 20, '5-minute load per core. Fine below 0.7, tasks queue up beyond 1.0, bad at 2.0')

  let mem = lowerBetter(o.mem_percent, 80, 98)
  let memValue = `${o.mem_percent.toFixed(0)}%`
  if (o.swap_total > 0) {
    const swap = (o.swap_used / o.swap_total) * 100
    mem = Math.min(mem, lowerBetter(swap, 30, 90))
    if (swap >= 1) memValue += ` · swap ${swap.toFixed(0)}%`
  }
  add('memory', 'Memory', memValue, o.mem_percent, mem, 20, 'Fine below 80% with little swap in use, heavy swapping makes everything slow')

  if (o.disks?.length) {
    const worst = o.disks.reduce((a, d) => (d.percent > a.percent ? d : a), o.disks[0])
    add('disk', 'Disk', `${formatBytes(Math.max(0, worst.total - worst.used))} free · ${worst.mount}`, worst.percent, lowerBetter(worst.percent, 80, 98), 15, 'Fullest disk. Fine below 80%, writes start failing near 100%')
  }

  // Slow-probe factors: keep the server's numbers.
  for (const f of server?.factors || []) if (f.id === 'errors' || f.id === 'services') factors.push(f)

  let sum = 0
  let wsum = 0
  let lowest = factors[0]
  for (const f of factors) {
    sum += f.score * f.weight
    wsum += f.weight
    if (f.score < lowest.score) lowest = f
  }
  let score = wsum ? sum / wsum : 100
  if (lowest && lowest.score < 25 && score > 60) score = 60
  score = Math.round(score)
  const [level, label] = levelFor(score)
  return {
    score, level, label,
    bottleneck: lowest && lowest.score < 85 ? lowest.id : '',
    factors: [...factors].sort((a, b) => a.score - b.score),
  }
}
