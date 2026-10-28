import client from './client'

export const getSelfUpdate = (refresh = false) => client.get('/system/update', { params: refresh ? { refresh: 1 } : {} }).then((r) => r.data)
export const getSelfUpdateJob = () => client.get('/system/update/job', { timeout: 8000 }).then((r) => r.data)
export const startSelfUpdate = () => client.post('/system/update').then((r) => r.data)
