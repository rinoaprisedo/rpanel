// Splits an nginx "combined" access-log line into coloured parts; null when the line has another format.
const ACCESS = /^(?:\[([^\]]+)\] )?(\S+) (\S+) (\S+) \[([^\]]+)\] "([^"]*)" (\d{3}) (\S+) "([^"]*)" "([^"]*)"(.*)$/

export function parseAccess(line) {
  const m = ACCESS.exec(line)
  if (!m) return null
  return { site: m[1], ip: m[2], time: m[5], request: m[6], status: Number(m[7]), size: m[8], referer: m[9], agent: m[10], rest: m[11] }
}

export function statusTone(code) {
  if (code >= 500) return 'text-red-400'
  if (code >= 400) return 'text-amber-400'
  if (code >= 300) return 'text-sky-400'
  return 'text-emerald-400'
}

// Splits text around case-insensitive matches of kw: [{ text, hit }]
export function splitHits(text, kw) {
  const k = (kw || '').trim().toLowerCase()
  if (!k) return [{ text, hit: false }]
  const out = []
  const lower = text.toLowerCase()
  let i = 0
  for (;;) {
    const j = lower.indexOf(k, i)
    if (j < 0) break
    if (j > i) out.push({ text: text.slice(i, j), hit: false })
    out.push({ text: text.slice(j, j + k.length), hit: true })
    i = j + k.length
  }
  if (i < text.length) out.push({ text: text.slice(i), hit: false })
  return out
}
