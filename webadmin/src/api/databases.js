import client from './client'

const s = (id) => `/databases/servers/${id}`
export const getServers = () => client.get('/databases/servers').then((r) => r.data)
export const addServer = (d) => client.post('/databases/servers', d).then((r) => r.data)
export const installServer = (engine) => client.post('/databases/servers/install', { engine }, { timeout: 1500000 }).then((r) => r.data)
export const deleteServer = (id) => client.delete(s(id)).then((r) => r.data)
export const getDatabases = (id) => client.get(`${s(id)}/databases`).then((r) => r.data)
export const createDatabase = (id, d) => client.post(`${s(id)}/databases`, d).then((r) => r.data)
export const dropDatabase = (id, name) => client.delete(`${s(id)}/databases/${encodeURIComponent(name)}`).then((r) => r.data)
export const getUsers = (id) => client.get(`${s(id)}/users`).then((r) => r.data)
export const createUser = (id, d) => client.post(`${s(id)}/users`, d).then((r) => r.data)
export const dropUser = (id, user, host) => client.delete(`${s(id)}/users`, { params: { user, host } }).then((r) => r.data)
export const setPassword = (id, d) => client.put(`${s(id)}/users/password`, d).then((r) => r.data)
export const grant = (id, d) => client.post(`${s(id)}/users/grant`, d).then((r) => r.data)
export const getBackups = (id) => client.get(`${s(id)}/backups`).then((r) => r.data)
export const createBackup = (id, database) => client.post(`${s(id)}/backups`, { database }, { timeout: 1900000 }).then((r) => r.data)
export const restoreBackup = (id, file, target) => client.post(`${s(id)}/backups/restore`, { file, target }, { timeout: 1900000 }).then((r) => r.data)
export const deleteBackup = (id, file) => client.delete(`${s(id)}/backups/${encodeURIComponent(file)}`).then((r) => r.data)
const base = import.meta.env.VITE_API_URL || 'http://localhost:8800/api/v1'
export const backupDownloadURL = (id, file) => `${base}${s(id)}/backups/${encodeURIComponent(file)}/download`
