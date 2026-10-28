import client from './client'

export const getPanelSSL = () => client.get('/panel-ssl').then((r) => r.data)
export const issuePanelSSL = (payload) =>
  client.post('/panel-ssl/letsencrypt', payload, { timeout: 330000 }).then((r) => r.data)
export const customPanelSSL = (payload) => client.post('/panel-ssl/custom', payload, { timeout: 60000 }).then((r) => r.data)
