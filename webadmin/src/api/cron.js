import client from './client'

export const getJobs = () => client.get('/cron').then((r) => r.data)
export const createJob = (d) => client.post('/cron', d).then((r) => r.data)
export const updateJob = (id, d) => client.put(`/cron/${id}`, d).then((r) => r.data)
export const enableJob = (id) => client.post(`/cron/${id}/enable`).then((r) => r.data)
export const disableJob = (id) => client.post(`/cron/${id}/disable`).then((r) => r.data)
export const runJob = (id) => client.post(`/cron/${id}/run`).then((r) => r.data)
export const getRuns = (id) => client.get(`/cron/${id}/runs`).then((r) => r.data)
export const deleteJob = (id) => client.delete(`/cron/${id}`).then((r) => r.data)
