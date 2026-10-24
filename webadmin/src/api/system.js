import client from './client'

export const getVersion = () => client.get('/version').then((r) => r.data)
