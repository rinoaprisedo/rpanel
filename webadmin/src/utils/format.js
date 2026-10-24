import { format } from 'date-fns'

export function formatBytes(n = 0) {
  if (!n) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(Math.floor(Math.log(n) / Math.log(1024)), u.length - 1)
  return `${(n / 1024 ** i).toFixed(i ? 1 : 0)} ${u[i]}`
}

export function formatUptime(sec = 0) {
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  return d ? `${d}d ${h}h ${m}m` : `${h}h ${m}m`
}

// "12 Jan 26 12:12" — compact, fixed-width timestamp for file lists.
export function formatShortDate(value) {
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return '—'
  return format(d, 'dd MMM yy HH:mm')
}
