import { useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { ArrowUp, RefreshCw, FolderPlus, FilePlus, Upload, Trash2, Copy, Scissors, ClipboardPaste, Archive, Folder, File as FileIcon, Download, Pencil, Shield, PackageOpen } from 'lucide-react'
import Button from '../../components/ui/Button'
import Input from '../../components/ui/Input'
import Modal from '../../components/ui/Modal'
import ConfirmDialog from '../../components/ui/ConfirmDialog'
import Skeleton from '../../components/ui/Skeleton'
import * as api from '../../api/fileManager'
import { formatBytes } from '../../utils/format'

const parentOf = (p) => (p === '/' ? '/' : p.replace(/\/[^/]+\/?$/, '') || '/')
const join = (dir, name) => (dir === '/' ? `/${name}` : `${dir}/${name}`)
const errMsg = (e) => e.response?.data?.message || 'Operation failed'

function FileManager() {
  const qc = useQueryClient()
  const [path, setPath] = useState(null)
  const [selected, setSelected] = useState([])
  const [clip, setClip] = useState(null) // { op: 'copy'|'move', paths }
  const [prompt, setPrompt] = useState(null) // { title, label, value, onSubmit }
  const [confirm, setConfirm] = useState(null) // paths to delete
  const [editor, setEditor] = useState(null) // { path, content }
  const [busy, setBusy] = useState(false)
  const fileInput = useRef(null)

  const { data: rootsRes } = useQuery({ queryKey: ['fm-roots'], queryFn: api.getRoots })
  const cwd = path ?? rootsRes?.data?.[0]
  const { data, isLoading, error } = useQuery({
    queryKey: ['fm-list', cwd],
    queryFn: () => api.listDir(cwd),
    enabled: !!cwd,
    retry: false,
  })
  const entries = data?.data?.entries || []

  const refresh = () => qc.invalidateQueries({ queryKey: ['fm-list'] })
  const run = async (fn, okMsg) => {
    setBusy(true)
    try {
      await fn()
      if (okMsg) toast.success(okMsg)
      setSelected([])
      refresh()
    } catch (e) {
      toast.error(errMsg(e))
    } finally {
      setBusy(false)
    }
  }
  const go = (p) => { setPath(p); setSelected([]) }
  const toggle = (p) => setSelected((s) => (s.includes(p) ? s.filter((x) => x !== p) : [...s, p]))
  const openEntry = async (e) => {
    if (e.is_dir) return go(e.path)
    try {
      const r = await api.readFile(e.path)
      setEditor({ path: e.path, content: r.data.content })
    } catch (er) {
      toast.error(errMsg(er))
    }
  }
  const ask = (title, label, value, onSubmit) => setPrompt({ title, label, value, onSubmit })

  const crumbs = (cwd || '/').split('/').filter(Boolean)
  const one = selected.length === 1 ? entries.find((e) => e.path === selected[0]) : null

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <Button variant="secondary" onClick={() => go(parentOf(cwd))} disabled={!cwd || cwd === '/'}><ArrowUp size={14} /></Button>
        <Button variant="secondary" onClick={refresh}><RefreshCw size={14} /></Button>
        <Button variant="secondary" onClick={() => ask('New folder', 'Name', '', (v) => api.mkdir(join(cwd, v)))}><FolderPlus size={14} />Folder</Button>
        <Button variant="secondary" onClick={() => ask('New file', 'Name', '', (v) => api.touch(join(cwd, v)))}><FilePlus size={14} />File</Button>
        <Button variant="secondary" onClick={() => fileInput.current?.click()}><Upload size={14} />Upload</Button>
        <input
          ref={fileInput} type="file" multiple hidden
          onChange={(e) => {
            const files = [...e.target.files]
            e.target.value = ''
            if (files.length) run(() => api.uploadFiles(cwd, files), 'Uploaded')
          }}
        />
        <span className="mx-1 h-5 w-px bg-surface-border" />
        <Button variant="secondary" disabled={!selected.length} onClick={() => { setClip({ op: 'copy', paths: selected }); toast.success('Copied') }}><Copy size={14} />Copy</Button>
        <Button variant="secondary" disabled={!selected.length} onClick={() => { setClip({ op: 'move', paths: selected }); toast.success('Cut') }}><Scissors size={14} />Cut</Button>
        <Button variant="secondary" disabled={!clip || busy} onClick={() => run(async () => { await (clip.op === 'copy' ? api.copyPaths : api.movePaths)(clip.paths, cwd); setClip(null) }, 'Done')}><ClipboardPaste size={14} />Paste</Button>
        <Button variant="secondary" disabled={!selected.length} onClick={() => ask('Compress to zip', 'Archive name', 'archive.zip', (v) => api.compressPaths(selected, join(cwd, v)))}><Archive size={14} />Zip</Button>
        <Button variant="danger" disabled={!selected.length} onClick={() => setConfirm(selected)}><Trash2 size={14} />Delete</Button>
      </div>

      <div className="flex flex-wrap items-center gap-1 rounded-md border border-surface-border bg-surface-card px-3 py-2 text-sm">
        <button className="text-primary hover:underline" onClick={() => go('/')}>/</button>
        {crumbs.map((c, i) => (
          <span key={i} className="flex items-center gap-1">
            <button className="text-primary hover:underline" onClick={() => go('/' + crumbs.slice(0, i + 1).join('/'))}>{c}</button>
            {i < crumbs.length - 1 && <span className="text-text-secondary">/</span>}
          </span>
        ))}
      </div>

      <div className="overflow-x-auto rounded-lg border border-surface-border bg-surface-card">
        {isLoading || !cwd ? (
          <Skeleton className="h-64 w-full" />
        ) : error ? (
          <p className="p-6 text-sm text-danger">{errMsg(error)}</p>
        ) : (
          <table className="w-full text-sm">
            <thead className="border-b border-surface-border text-left text-xs uppercase text-text-secondary">
              <tr>
                <th className="w-8 p-2"><input type="checkbox" checked={entries.length > 0 && selected.length === entries.length} onChange={(e) => setSelected(e.target.checked ? entries.map((x) => x.path) : [])} /></th>
                <th className="p-2">Name</th><th className="p-2">Size</th><th className="p-2">Mode</th><th className="p-2">Owner</th><th className="p-2">Modified</th>
              </tr>
            </thead>
            <tbody>
              {entries.map((e) => (
                <tr key={e.path} className="border-b border-surface-border last:border-0 hover:bg-surface-hover">
                  <td className="p-2"><input type="checkbox" checked={selected.includes(e.path)} onChange={() => toggle(e.path)} /></td>
                  <td className="p-2">
                    <button className="flex items-center gap-2 text-left text-text-primary hover:text-primary" onClick={() => openEntry(e)}>
                      {e.is_dir ? <Folder size={15} className="text-amber-500" /> : <FileIcon size={15} className="text-text-secondary" />}
                      {e.name}{e.is_link && <span className="text-xs text-text-secondary"> →</span>}
                    </button>
                  </td>
                  <td className="p-2 text-text-secondary">{e.is_dir ? '—' : formatBytes(e.size)}</td>
                  <td className="p-2 font-mono text-xs text-text-secondary">{e.mode}</td>
                  <td className="p-2 text-text-secondary">{e.owner}:{e.group}</td>
                  <td className="p-2 text-text-secondary">{new Date(e.mod_time).toLocaleString()}</td>
                </tr>
              ))}
              {!entries.length && <tr><td colSpan={6} className="p-6 text-center text-text-secondary">Empty directory</td></tr>}
            </tbody>
          </table>
        )}
      </div>

      {one && (
        <div className="flex flex-wrap items-center gap-2 rounded-md border border-surface-border bg-surface-card p-3 text-sm">
          <span className="mr-2 font-medium text-text-primary">{one.name}</span>
          <Button variant="secondary" onClick={() => ask('Rename', 'New name', one.name, (v) => api.renamePath(one.path, v))}><Pencil size={14} />Rename</Button>
          <Button variant="secondary" onClick={() => ask('Permissions', 'Octal mode (e.g. 755)', one.mode.slice(1), (v) => api.chmodPath(one.path, v, one.is_dir))}><Shield size={14} />Chmod</Button>
          {!one.is_dir && <Button variant="secondary" onClick={() => { window.location.href = api.downloadURL(one.path) }}><Download size={14} />Download</Button>}
          {!one.is_dir && one.name.toLowerCase().endsWith('.zip') && (
            <Button variant="secondary" onClick={() => run(() => api.extractZip(one.path, cwd), 'Extracted')}><PackageOpen size={14} />Extract here</Button>
          )}
        </div>
      )}

      <Modal open={!!prompt} onClose={() => setPrompt(null)} title={prompt?.title}>
        {prompt && (
          <form
            className="space-y-4"
            onSubmit={(ev) => {
              ev.preventDefault()
              const { onSubmit, value } = prompt
              setPrompt(null)
              run(() => onSubmit(value), 'Done')
            }}
          >
            <Input label={prompt.label} autoFocus value={prompt.value} onChange={(e) => setPrompt({ ...prompt, value: e.target.value })} />
            <div className="flex justify-end gap-2">
              <Button type="button" variant="secondary" onClick={() => setPrompt(null)}>Cancel</Button>
              <Button type="submit" disabled={!prompt.value}>OK</Button>
            </div>
          </form>
        )}
      </Modal>

      <ConfirmDialog
        open={!!confirm}
        onClose={() => setConfirm(null)}
        loading={busy}
        title="Delete"
        message={`Permanently delete ${confirm?.length} item(s)? This cannot be undone.`}
        onConfirm={async () => { const p = confirm; await run(() => api.deletePaths(p), 'Deleted'); setConfirm(null) }}
      />

      <Modal open={!!editor} onClose={() => setEditor(null)} title={editor?.path} size="xl">
        {editor && (
          <div className="space-y-3">
            <textarea
              spellCheck={false}
              className="h-[60vh] w-full resize-none rounded-md border border-surface-border bg-white p-3 font-mono text-xs text-text-primary outline-none focus:border-primary"
              value={editor.content}
              onChange={(e) => setEditor({ ...editor, content: e.target.value })}
            />
            <div className="flex justify-end gap-2">
              <Button variant="secondary" onClick={() => setEditor(null)}>Close</Button>
              <Button disabled={busy} onClick={() => run(() => api.writeFile(editor.path, editor.content), 'Saved')}>Save</Button>
            </div>
          </div>
        )}
      </Modal>
    </div>
  )
}

export default FileManager
