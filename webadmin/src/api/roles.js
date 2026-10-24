import client from './client'

export const getRoles = (params) => client.get('/roles', { params }).then((r) => r.data)
export const getRole = (uuid) => client.get(`/roles/${uuid}`).then((r) => r.data)
export const createRole = (payload) => client.post('/roles', payload).then((r) => r.data)
export const updateRole = (uuid, payload) => client.put(`/roles/${uuid}`, payload).then((r) => r.data)
export const deleteRole = (uuid) => client.delete(`/roles/${uuid}`).then((r) => r.data)
export const getRolePermissions = (uuid) => client.get(`/roles/${uuid}/permissions`).then((r) => r.data)
export const updateRolePermissions = (uuid, payload) =>
  client.put(`/roles/${uuid}/permissions`, payload).then((r) => r.data)
