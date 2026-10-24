const apiBase = import.meta.env.VITE_API_URL || 'http://localhost:8800/api/v1'
export const apiOrigin = apiBase.replace(/\/api\/v1\/?$/, '')

// Settings store relative storage paths (e.g. "settings/abc123.png"); the
// files are served by the backend at /uploads/<path>, not the frontend origin.
export function fileURL(path) {
  if (!path) return null
  if (/^https?:\/\//.test(path)) return path
  return `${apiOrigin}/uploads/${path.replace(/^\/+/, '')}`
}

// Private files (e.g. QC ticket screenshots) aren't under /uploads: the
// backend serves them at /api/v1/files/<path> only to staff with access
// (session cookie, sent automatically since the API is same-site).
export function privateFileURL(path) {
  if (!path) return null
  if (/^https?:\/\//.test(path)) return path
  return `${apiOrigin}/api/v1/files/${path.replace(/^\/+/, '').replace(/^uploads\//, '')}`
}
