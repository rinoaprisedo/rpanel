import client from './client'

export const getKeys = () => client.get('/api-keys').then((r) => r.data)
export const createKey = (d) => client.post('/api-keys', d).then((r) => r.data)
export const revokeKey = (id) => client.post(`/api-keys/${id}/revoke`).then((r) => r.data)
export const deleteKey = (id) => client.delete(`/api-keys/${id}`).then((r) => r.data)
export const updateKey = (id, d) => client.put(`/api-keys/${id}`, d).then((r) => r.data)
export const getKeyLogs = (id, before) => client.get(`/api-keys/${id}/logs`, { params: { before, limit: 50 } }).then((r) => r.data)
