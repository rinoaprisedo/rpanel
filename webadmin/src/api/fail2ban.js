import client from './client'

export const getInstalled = () => client.get('/fail2ban/installed').then((r) => r.data)
export const getStatus = () => client.get('/fail2ban/status').then((r) => r.data)
export const getSettings = () => client.get('/fail2ban/settings').then((r) => r.data)
export const saveSettings = (payload) => client.put('/fail2ban/settings', payload, { timeout: 60000 }).then((r) => r.data)
export const banIP = (jail, ip) => client.post(`/fail2ban/jails/${jail}/ban`, { ip }).then((r) => r.data)
export const unbanIP = (jail, ip) => client.post(`/fail2ban/jails/${jail}/unban`, { ip }).then((r) => r.data)
export const control = (action) => client.post(`/fail2ban/service/${action}`, null, { timeout: 90000 }).then((r) => r.data)
export const getLog = (lines = 200) => client.get('/fail2ban/log', { params: { lines } }).then((r) => r.data)
