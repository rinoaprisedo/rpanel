import client from './client'

export const getLogSites = () => client.get('/logs/sites').then((r) => r.data)
export const getLog = (domain, params) => client.get(`/logs/sites/${encodeURIComponent(domain)}`, { params }).then((r) => r.data)
export const clearLog = (domain, type) => client.delete(`/logs/sites/${encodeURIComponent(domain)}`, { params: { type } }).then((r) => r.data)
export const downloadLog = (domain, type) =>
  client.get(`/logs/sites/${encodeURIComponent(domain)}/download`, { params: { type }, responseType: 'blob', timeout: 300000 }).then((r) => r.data)
