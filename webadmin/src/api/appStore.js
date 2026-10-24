import client from './client'

export const getApps = () => client.get('/app-store/apps').then((r) => r.data)
export const installApp = (id, version) => client.post(`/app-store/apps/${id}/install`, { version }).then((r) => r.data)
export const uninstallApp = (id, version) => client.post(`/app-store/apps/${id}/uninstall`, { version }).then((r) => r.data)
export const controlApp = (id, action, version) => client.post(`/app-store/apps/${id}/service/${action}`, { version }).then((r) => r.data)
