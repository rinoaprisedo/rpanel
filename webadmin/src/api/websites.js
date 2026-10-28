import client from './client'

export const getWebsites = () => client.get('/websites').then((r) => r.data)
export const getWebsiteEnv = () => client.get('/websites/env').then((r) => r.data)
export const createWebsite = (data) => client.post('/websites', data).then((r) => r.data)
export const updateWebsite = (uuid, data) => client.put(`/websites/${uuid}`, data).then((r) => r.data)
export const startWebsite = (uuid) => client.post(`/websites/${uuid}/start`).then((r) => r.data)
export const stopWebsite = (uuid) => client.post(`/websites/${uuid}/stop`).then((r) => r.data)
export const issueSSL = (uuid, email) => client.post(`/websites/${uuid}/ssl`, { email }, { timeout: 330000 }).then((r) => r.data)
export const deleteWebsite = (uuid, deleteFiles) =>
  client.delete(`/websites/${uuid}`, { params: { delete_files: deleteFiles ? 1 : 0 } }).then((r) => r.data)
export const getWebsiteLogs = (uuid, type, lines = 200) =>
  client.get(`/websites/${uuid}/logs`, { params: { type, lines } }).then((r) => r.data)
export const installComponent = (component, version) =>
  client.post('/websites/install', { component, version }, { timeout: 1000000 }).then((r) => r.data)
export const getWebsiteDirs = (path) => client.get('/websites/dirs', { params: path ? { path } : {} }).then((r) => r.data)
export const getSiteConfig = (uuid) => client.get(`/websites/${uuid}/config`).then((r) => r.data)
export const saveSiteConfig = (uuid, extra_config) => client.put(`/websites/${uuid}/config`, { extra_config }).then((r) => r.data)
export const getSiteSSL = (uuid) => client.get(`/websites/${uuid}/ssl`).then((r) => r.data)
export const disableSSL = (uuid) => client.delete(`/websites/${uuid}/ssl`).then((r) => r.data)
export const saveProxyRules = (uuid, rules) => client.put(`/websites/${uuid}/proxies`, { rules }).then((r) => r.data)
export const startSSL = (uuid, payload = {}) => client.post(`/websites/${uuid}/ssl/start`, payload).then((r) => r.data)
export const getSSLProgress = (uuid) => client.get(`/websites/${uuid}/ssl/progress`).then((r) => r.data)
