// Pure helpers shared by the SVG charts.

// Rounds a step up to 1, 2, 2.5, 5 × 10^k so axis ticks land on clean numbers.
function niceStep(raw) {
  const exp = Math.floor(Math.log10(raw))
  const f = raw / 10 ** exp
  const nice = f <= 1 ? 1 : f <= 2 ? 2 : f <= 2.5 ? 2.5 : f <= 5 ? 5 : 10
  return nice * 10 ** exp
}

export function niceTicks(max, count = 4) {
  if (!max || max <= 0) return { ticks: [0, 1], top: 1 }
  const step = niceStep(max / count)
  const top = Math.ceil(max / step) * step
  const ticks = []
  for (let v = 0; v <= top + step / 1000; v += step) ticks.push(Number(v.toFixed(6)))
  return { ticks, top }
}

// 1,200 → 1.2K · 3,400,000 → 3.4M
export function compact(n) {
  const a = Math.abs(n)
  if (a >= 1e9) return `${(n / 1e9).toFixed(a >= 1e10 ? 0 : 1).replace(/\.0$/, '')}B`
  if (a >= 1e6) return `${(n / 1e6).toFixed(a >= 1e7 ? 0 : 1).replace(/\.0$/, '')}M`
  if (a >= 1e3) return `${(n / 1e3).toFixed(a >= 1e4 ? 0 : 1).replace(/\.0$/, '')}K`
  return String(Math.round(n * 10) / 10)
}

const hm = new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit', hour12: false })
const day = new Intl.DateTimeFormat(undefined, { weekday: 'short', day: 'numeric' })
const full = new Intl.DateTimeFormat(undefined, { weekday: 'short', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit', hour12: false })
const fullDay = new Intl.DateTimeFormat(undefined, { weekday: 'long', day: 'numeric', month: 'short' })

// Axis label for a unix-seconds timestamp: clock time within a day, weekday + date beyond.
export const axisTime = (t, spanSec) => (spanSec > 2 * 86400 ? day : hm).format(new Date(t * 1000))
export const tooltipTime = (t, spanSec, daily) => (daily ? fullDay : spanSec > 2 * 86400 ? full : hm).format(new Date(t * 1000))

// Picks about `n` evenly spaced indices (always including the last) for x labels.
export function labelIndices(len, n) {
  if (len <= n) return Array.from({ length: len }, (_, i) => i)
  const out = []
  for (let i = 0; i < n; i++) out.push(Math.round((i * (len - 1)) / (n - 1)))
  return [...new Set(out)]
}

export function pct(part, whole, digits = 1) {
  return whole ? `${((part / whole) * 100).toFixed(digits)}%` : '0%'
}
