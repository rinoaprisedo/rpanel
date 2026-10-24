import client from './client'

export const getTemplates = () => client.get('/terminal/templates').then((r) => r.data)
export const createTemplate = (d) => client.post('/terminal/templates', d).then((r) => r.data)
export const updateTemplate = (id, d) => client.put(`/terminal/templates/${id}`, d).then((r) => r.data)
export const deleteTemplate = (id) => client.delete(`/terminal/templates/${id}`).then((r) => r.data)
