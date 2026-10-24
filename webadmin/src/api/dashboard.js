import client from './client'

export const getOverview = () => client.get('/dashboard/overview').then((r) => r.data)
export const getHistory = () => client.get('/dashboard/history').then((r) => r.data)
export const getProcesses = (params) => client.get('/dashboard/processes', { params }).then((r) => r.data)
