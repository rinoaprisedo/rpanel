const DAYS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']
const num = (s) => /^\d+$/.test(s)
const hhmm = (h, m) => `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`

// Plain-language reading of the common 5-field patterns; anything else is "Custom schedule".
export function describeSchedule(expr = '') {
  const f = expr.trim().split(/\s+/)
  if (f.length !== 5) return 'Custom schedule'
  const [m, h, dom, mon, dow] = f
  if (expr.trim() === '* * * * *') return 'Every minute'
  if (/^\*\/\d+$/.test(m) && f.slice(1).every((x) => x === '*')) return `Every ${m.slice(2)} minutes`
  if (num(m) && h === '*' && dom === '*' && mon === '*' && dow === '*') return Number(m) === 0 ? 'Every hour' : `Every hour at :${m.padStart(2, '0')}`
  if (num(m) && /^\*\/\d+$/.test(h) && dom === '*' && mon === '*' && dow === '*') return `Every ${h.slice(2)} hours`
  if (num(m) && num(h) && mon === '*') {
    if (dom === '*' && dow === '*') return `Every day at ${hhmm(h, m)}`
    if (dom === '*' && num(dow)) return `Every ${DAYS[Number(dow) % 7]} at ${hhmm(h, m)}`
    if (num(dom) && dow === '*') return `Monthly on day ${dom} at ${hhmm(h, m)}`
  }
  return 'Custom schedule'
}

export function ago(value) {
  const s = Math.round((Date.now() - new Date(value).getTime()) / 1000)
  if (!Number.isFinite(s)) return ''
  if (s < 45) return 'just now'
  if (s < 3600) return `${Math.round(s / 60)} min ago`
  if (s < 86400) return `${Math.round(s / 3600)} h ago`
  return `${Math.round(s / 86400)} d ago`
}
