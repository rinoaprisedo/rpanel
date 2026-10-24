import client from './client'

export const getActivityLogs = (params) => client.get('/activity-logs', { params }).then((r) => r.data)
export const getActivityLog = (uuid) => client.get(`/activity-logs/${uuid}`).then((r) => r.data)
export const cleanupActivityLogs = () => client.delete('/activity-logs/cleanup').then((r) => r.data)
