import client from './client'

export const getOverview = () => client.get('/dashboard/overview').then((r) => r.data)
export const getHistory = () => client.get('/dashboard/history').then((r) => r.data)
export const getProcesses = (params) => client.get('/dashboard/processes', { params }).then((r) => r.data)
export const getHistoryRange = (range) => client.get('/dashboard/history', { params: { range } }).then((r) => r.data)
export const getTraffic = (range, site) => client.get('/dashboard/traffic', { params: { range, site: site || undefined } }).then((r) => r.data)
export const getInsights = () => client.get('/dashboard/insights').then((r) => r.data)
