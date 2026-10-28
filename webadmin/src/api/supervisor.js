import client from './client'

export const getInstalled = () => client.get('/supervisor/installed').then((r) => r.data)
export const getStatus = () => client.get('/supervisor/status').then((r) => r.data)
export const programAction = (name, action) => client.post(`/supervisor/programs/${encodeURIComponent(name)}/${action}`, null, { timeout: 100000 }).then((r) => r.data)
export const getLog = (name, stream) => client.get(`/supervisor/programs/${encodeURIComponent(name)}/log`, { params: { stream } }).then((r) => r.data)
export const createProgram = (d) => client.post('/supervisor/programs', d, { timeout: 200000 }).then((r) => r.data)
export const updateProgram = (name, d) => client.put(`/supervisor/programs/${name}`, d, { timeout: 200000 }).then((r) => r.data)
export const deleteProgram = (name) => client.delete(`/supervisor/programs/${name}`, { timeout: 200000 }).then((r) => r.data)
export const daemon = (action) => client.post(`/supervisor/service/${action}`, null, { timeout: 100000 }).then((r) => r.data)
