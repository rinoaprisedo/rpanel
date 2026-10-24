import client from './client'

const s = (id) => `/databases/servers/${id}`
export const getServers = () => client.get('/databases/servers').then((r) => r.data)
export const addServer = (d) => client.post('/databases/servers', d).then((r) => r.data)
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

const b = (id, db) => `${s(id)}/browse/${encodeURIComponent(db)}`
const t = (id, db, table) => `${b(id, db)}/tables/${encodeURIComponent(table)}`
export const getTables = (id, db) => client.get(`${b(id, db)}/tables`).then((r) => r.data)
export const getTable = (id, db, table, params) => client.get(t(id, db, table), { params }).then((r) => r.data)
export const insertRow = (id, db, table, values) => client.post(`${t(id, db, table)}/rows`, { values }).then((r) => r.data)
export const updateRow = (id, db, table, pk, values) => client.put(`${t(id, db, table)}/rows`, { pk, values }).then((r) => r.data)
export const deleteRow = (id, db, table, pk) => client.delete(`${t(id, db, table)}/rows`, { data: { pk } }).then((r) => r.data)
export const truncateTable = (id, db, table) => client.post(`${t(id, db, table)}/truncate`).then((r) => r.data)
export const dropTable = (id, db, table) => client.delete(t(id, db, table)).then((r) => r.data)
export const runQuery = (id, db, sql) => client.post(`${b(id, db)}/query`, { sql }, { timeout: 40000 }).then((r) => r.data)
