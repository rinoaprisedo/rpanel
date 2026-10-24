import client from './client'

export const getFirewall = () => client.get('/firewall/status').then((r) => r.data)
export const installFirewall = () => client.post('/firewall/install', null, { timeout: 900000 }).then((r) => r.data)
export const enableFirewall = () => client.post('/firewall/enable').then((r) => r.data)
export const disableFirewall = () => client.post('/firewall/disable').then((r) => r.data)
export const setDefaultIncoming = (policy) => client.post('/firewall/default', { policy }).then((r) => r.data)
export const addRule = (d) => client.post('/firewall/rules', d).then((r) => r.data)
export const deleteRule = (rule, confirm) =>
  client.delete(`/firewall/rules/${rule.number}`, { params: { to: rule.to, action: rule.action, confirm: confirm ? 1 : 0 } }).then((r) => r.data)
