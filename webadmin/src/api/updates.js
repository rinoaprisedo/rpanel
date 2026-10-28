import client from './client'

export const getUpdates = () => client.get('/updates', { timeout: 200000 }).then((r) => r.data)
export const getUpdateProgress = () => client.get('/updates/progress').then((r) => r.data)
export const runUpdates = (kind) => client.post('/updates/run', { kind }).then((r) => r.data)
export const setAutoSecurity = (enabled) => client.put('/updates/auto', { enabled }, { timeout: 660000 }).then((r) => r.data)
export const reboot = () => client.post('/updates/reboot', { confirm: true }).then((r) => r.data)
