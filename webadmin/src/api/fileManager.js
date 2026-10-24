import client from './client'

const base = import.meta.env.VITE_API_URL || 'http://localhost:8800/api/v1'

export const getRoots = () => client.get('/file-manager/roots').then((r) => r.data)
export const listDir = (path) => client.get('/file-manager/list', { params: { path } }).then((r) => r.data)
export const readFile = (path) => client.get('/file-manager/read', { params: { path } }).then((r) => r.data)
export const writeFile = (path, content) => client.put('/file-manager/write', { path, content }).then((r) => r.data)
export const mkdir = (path) => client.post('/file-manager/mkdir', { path }).then((r) => r.data)
export const touch = (path) => client.post('/file-manager/touch', { path }).then((r) => r.data)
export const renamePath = (path, new_name) => client.post('/file-manager/rename', { path, new_name }).then((r) => r.data)
export const movePaths = (paths, dest) => client.post('/file-manager/move', { paths, dest }).then((r) => r.data)
export const copyPaths = (paths, dest) => client.post('/file-manager/copy', { paths, dest }).then((r) => r.data)
export const deletePaths = (paths) => client.post('/file-manager/delete', { paths }).then((r) => r.data)
export const chmodPath = (path, mode, recursive) => client.post('/file-manager/chmod', { path, mode, recursive }).then((r) => r.data)
export const compressPaths = (paths, dest) => client.post('/file-manager/compress', { paths, dest }).then((r) => r.data)
export const extractZip = (path, dest) => client.post('/file-manager/extract', { path, dest }).then((r) => r.data)
export const uploadFiles = (dir, files, onProgress) => {
  const form = new FormData()
  files.forEach((f) => form.append('files', f))
  return client
    .post('/file-manager/upload', form, {
      params: { dir }, // dir is a query param: the server streams the multipart body
      headers: { 'Content-Type': 'multipart/form-data' },
      timeout: 0,
      onUploadProgress: (e) => onProgress && e.total && onProgress(Math.round((e.loaded / e.total) * 100)),
    })
    .then((r) => r.data)
}
// Plain GET with the session cookie; the browser handles the attachment.
export const downloadURL = (path) => `${base}/file-manager/download?path=${encodeURIComponent(path)}`
