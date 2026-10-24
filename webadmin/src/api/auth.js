import client from './client'

export const login = (payload) => client.post('/auth/login', payload).then((r) => r.data)
export const logout = () => client.post('/auth/logout').then((r) => r.data)
export const me = () => client.get('/auth/me').then((r) => r.data)
export const forgotPassword = (payload) => client.post('/auth/forgot-password', payload).then((r) => r.data)
export const resetPassword = (payload) => client.post('/auth/reset-password', payload).then((r) => r.data)
export const changePassword = (payload) => client.put('/auth/change-password', payload).then((r) => r.data)
export const twoFASetup = () => client.post('/auth/2fa/setup').then((r) => r.data)
export const twoFAEnable = (code) => client.post('/auth/2fa/enable', { code }).then((r) => r.data)
export const twoFADisable = (password, code) => client.post('/auth/2fa/disable', { password, code }).then((r) => r.data)
export const twoFARecovery = (code) => client.post('/auth/2fa/recovery-codes', { code }).then((r) => r.data)
