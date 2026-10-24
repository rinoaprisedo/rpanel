import client from './client'

export const getSettings = () => client.get('/settings').then((r) => r.data)
// No session required — safe to call before login (e.g. on the login page)
// since the backend only returns whitelisted branding fields.
export const getPublicSettings = () => client.get('/settings/public').then((r) => r.data)
export const updateSettings = (payload) => client.put('/settings', payload).then((r) => r.data)
export const uploadSetting = (formData) =>
  client.post('/settings/upload', formData, { headers: { 'Content-Type': 'multipart/form-data' } }).then((r) => r.data)
