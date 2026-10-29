import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { ArrowUp, RefreshCw, FolderPlus, FilePlus, Upload, Trash2, Copy, Scissors, ClipboardPaste, Archive, Search, X, TerminalSquare } from 'lucide-react'
import { iconFor } from '../../utils/fileIcon'
import Button from '../../components/ui/Button'
import Input from '../../components/ui/Input'
import Modal from '../../components/ui/Modal'
import ConfirmDialog from '../../components/ui/ConfirmDialog'
import Skeleton from '../../components/ui/Skeleton'
import CodeEditor from '../../components/ui/CodeEditor'
import RowActions from './RowActions'
import UploadFlow from './UploadFlow'
import usePermission from '../../hooks/usePermission'
import useAuthStore from '../../store/authStore'
import { isImage } from '../../utils/highlight'
import * as api from '../../api/fileManager'
import { formatBytes, formatShortDate } from '../../utils/format'

const parentOf = (p) => (p === '/' ? '/' : p.replace(/\/[^/]+\/?$/, '') || '/')
const join = (dir, name) => (dir === '/' ? `/${name}` : `${dir}/${name}`)
const errMsg = (e) => e.response?.data?.message || 'Operation failed'

function useDebounced(value, ms) {
  const [v, setV] = useState(value)
  useEffect(() => {
    const t = setTimeout(() => setV(value), ms)
    return () => clearTimeout(t)
  }, [value, ms])
  return v
}

function FileManager() {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const { canCreate } = usePermission()
  const userKey = useAuthStore((s) => s.user?.uuid || s.user?.id || 'me')
  const lastKey = `rpanel-fm-path:${userKey}` // reopen where the user left off
  const [path, setPath] = useState(() => localStorage.getItem(lastKey))
  const [upload, setUpload] = useState(null) // { dir, files }
  const [selected, setSelected] = useState([])
  const [clip, setClip] = useState(null) // { op: 'copy'|'move', paths }
  const [prompt, setPrompt] = useState(null) // { title, label, value, onSubmit }
  const [confirm, setConfirm] = useState(null) // paths to delete
  const [editor, setEditor] = useState(null) // { path, content, saved } or { path, image: true }
  const [busy, setBusy] = useState(false)
  const [query, setQuery] = useState('')
  const [deep, setDeep] = useState(false) // also search inside sub-folders
  const [moreFor, setMoreFor] = useState(null) // path whose "More" menu is open
  const fileInput = useRef(null)
  // Scroll position of each folder we left, so going back (Up, breadcrumb) lands where you were
  // instead of at the top. `restore` is the position to apply once the next folder has rendered.
  const scrollMemory = useRef(new Map())
  const restore = useRef(null)

  const { data: rootsRes } = useQuery({ queryKey: ['fm-roots'], queryFn: api.getRoots })
  const cwd = path ?? rootsRes?.data?.[0]
  const { data, isLoading, error } = useQuery({
    queryKey: ['fm-list', cwd],
    queryFn: () => api.listDir(cwd),
    enabled: !!cwd,
    retry: false,
  })

  // The remembered folder may have been deleted or become inaccessible: fall back to the first root.
  useEffect(() => {
    if (error && path && path === localStorage.getItem(lastKey)) {
      localStorage.removeItem(lastKey)
      setPath(null)
    }
  }, [error, path, lastKey])

  const term = query.trim()
  const debounced = useDebounced(term, 300)
  const searching = deep && !!term
  const search = useQuery({
    queryKey: ['fm-search', cwd, debounced],
    queryFn: () => api.searchFiles(cwd, debounced),
    enabled: !!cwd && deep && !!debounced,
    retry: false,
  })

  const listed = data?.data?.entries

  // The page is the scroller. Wait for the listing: while it loads the page is short and the browser would clamp us to the top.
  useLayoutEffect(() => {
    if (restore.current === null || !listed) return
    window.scrollTo(0, restore.current)
    restore.current = null
  }, [cwd, listed])
  const entries = useMemo(() => {
    if (searching) return search.data?.data?.entries || []
    const all = listed || []
    return term ? all.filter((e) => e.name.toLowerCase().includes(term.toLowerCase())) : all
  }, [searching, search.data, listed, term])
  const truncated = searching && search.data?.data?.truncated

  const refresh = () => {
    qc.invalidateQueries({ queryKey: ['fm-list'] })
    qc.invalidateQueries({ queryKey: ['fm-search'] })
  }
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
  const go = (p) => {
    if (p !== cwd) {
      scrollMemory.current.set(cwd, window.scrollY)
      restore.current = scrollMemory.current.get(p) ?? 0 // folders never visited start at the top
    }
    setPath(p); localStorage.setItem(lastKey, p); setSelected([]); setQuery(''); setMoreFor(null)
  }
  const toggle = (p) => setSelected((s) => (s.includes(p) ? s.filter((x) => x !== p) : [...s, p]))
  const openEntry = async (e) => {
    if (e.is_dir) return go(e.path)
    if (isImage(e.path)) return setEditor({ path: e.path, image: true })
    try {
      const r = await api.readFile(e.path)
      setEditor({ path: e.path, content: r.data.content, saved: r.data.content })
    } catch (er) {
      toast.error(errMsg(er))
    }
  }
  const dirty = !!editor && !editor.image && editor.content !== editor.saved
  const closeEditor = () => {
    if (dirty && !window.confirm('Discard unsaved changes?')) return
    setEditor(null)
  }
  const saveEditor = () => run(async () => {
    await api.writeFile(editor.path, editor.content)
    setEditor((cur) => (cur ? { ...cur, saved: cur.content } : cur))
  }, 'Saved')
  const ask = (title, label, value, onSubmit) => setPrompt({ title, label, value, onSubmit })

  const copyPath = async (p) => {
    try {
      await navigator.clipboard.writeText(p)
      toast.success('Path copied')
    } catch {
      window.prompt('Copy the path:', p)
    }
  }
  const stash = (op, paths) => { setClip({ op, paths }); toast.success(op === 'copy' ? 'Copied' : 'Cut') }

  const actionsFor = (e) => {
    const dir = parentOf(e.path)
    const main = [
      { label: e.is_dir ? 'Open' : isImage(e.path) ? 'View' : 'Edit', run: () => openEntry(e) },
      ...(e.is_dir ? [] : [{ label: 'Download', run: () => { window.location.href = api.downloadURL(e.path) } }]),
      { label: 'Copy', run: () => stash('copy', [e.path]) },
      { label: 'Cut', run: () => stash('move', [e.path]) },
      { label: 'Rename', run: () => ask('Rename', 'New name', e.name, (v) => api.renamePath(e.path, v)) },
      { label: 'Perms', run: () => ask('Permissions', 'Octal mode (e.g. 755)', e.mode.slice(1), (v) => api.chmodPath(e.path, v, e.is_dir)) },
      { label: 'Zip', run: () => ask('Compress to zip', 'Archive name', `${e.name}.zip`, (v) => api.compressPaths([e.path], join(dir, v))) },
      { label: 'Delete', danger: true, run: () => setConfirm([e.path]) },
    ]
    const more = [
      { label: 'Copy path', run: () => copyPath(e.path) },
      ...(!e.is_dir && e.name.toLowerCase().endsWith('.zip') ? [{ label: 'Extract here', run: () => run(() => api.extractZip(e.path, dir), 'Extracted') }] : []),
      ...(e.is_dir && clip ? [{ label: 'Paste into folder', run: () => run(async () => { await (clip.op === 'copy' ? api.copyPaths : api.movePaths)(clip.paths, e.path); setClip(null) }, 'Done') }] : []),
    ]
    return { main, more }
  }

  const crumbs = (cwd || '/').split('/').filter(Boolean)
  const colSpan = 7

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
            if (files.length) setUpload({ dir: cwd, files })
          }}
        />
        {canCreate('terminal') && (
          <Button variant="secondary" disabled={!cwd} title="Open a terminal in this folder" onClick={() => navigate(`/terminal?cwd=${encodeURIComponent(cwd)}`)}><TerminalSquare size={14} />Terminal</Button>
        )}
        <span className="mx-1 h-5 w-px bg-surface-border" />
        <Button variant="secondary" disabled={!selected.length} onClick={() => stash('copy', selected)}><Copy size={14} />Copy</Button>
        <Button variant="secondary" disabled={!selected.length} onClick={() => stash('move', selected)}><Scissors size={14} />Cut</Button>
        <Button variant="secondary" disabled={!clip || busy} onClick={() => run(async () => { await (clip.op === 'copy' ? api.copyPaths : api.movePaths)(clip.paths, cwd); setClip(null) }, 'Done')}><ClipboardPaste size={14} />Paste{clip ? ` (${clip.paths.length})` : ''}</Button>
        <Button variant="secondary" disabled={!selected.length} onClick={() => ask('Compress to zip', 'Archive name', 'archive.zip', (v) => api.compressPaths(selected, join(cwd, v)))}><Archive size={14} />Zip</Button>
        <Button variant="danger" disabled={!selected.length} onClick={() => setConfirm(selected)}><Trash2 size={14} />Delete</Button>

        <div className="ml-auto flex items-center gap-3">
          <label className="flex cursor-pointer select-none items-center gap-1.5 text-xs text-text-secondary">
            <input type="checkbox" checked={deep} onChange={(e) => setDeep(e.target.checked)} />Include sub-folders
          </label>
          <div className="relative w-56">
            <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-text-tertiary" />
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => e.key === 'Escape' && setQuery('')}
              placeholder="Search name…"
              className="h-[38px] w-full rounded-md border-[1.5px] border-surface-border bg-field pl-8 pr-8 text-sm text-text-primary outline-none transition-colors focus:border-primary"
            />
            {query && <button className="absolute right-2 top-1/2 -translate-y-1/2 text-text-tertiary hover:text-text-primary" onClick={() => setQuery('')} aria-label="Clear search"><X size={14} /></button>}
          </div>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-1 rounded-md border border-surface-border bg-surface-card px-3 py-2 text-sm">
        <button className="text-primary hover:underline" onClick={() => go('/')}>/</button>
        {crumbs.map((c, i) => (
          <span key={i} className="flex items-center gap-1">
            <button className="text-primary hover:underline" onClick={() => go('/' + crumbs.slice(0, i + 1).join('/'))}>{c}</button>
            {i < crumbs.length - 1 && <span className="text-text-secondary">/</span>}
          </span>
        ))}
        {term && (
          <span className="ml-auto text-xs text-text-secondary">
            {searching && search.isFetching ? 'Searching…' : `${entries.length}${truncated ? '+' : ''} result${entries.length === 1 ? '' : 's'} for “${term}”${searching ? ' in this folder and below' : ''}`}
          </span>
        )}
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
                <th className="p-2">Name</th><th className="p-2">Size</th><th className="p-2">Mode</th><th className="p-2">Owner</th><th className="whitespace-nowrap p-2">Modified</th><th className="w-0 p-0" />
              </tr>
            </thead>
            <tbody>
              {entries.map((e) => {
                const { main, more } = actionsFor(e)
                const where = searching ? parentOf(e.path).replace(cwd === '/' ? '' : cwd, '') || '/' : ''
                return (
                  <tr key={e.path} className="group border-b border-surface-border last:border-0 hover:bg-surface-hover">
                    <td className="p-2"><input type="checkbox" checked={selected.includes(e.path)} onChange={() => toggle(e.path)} /></td>
                    <td className="p-2">
                      <button className="flex items-center gap-2 text-left text-text-primary hover:text-primary" onClick={() => openEntry(e)}>
                        {(() => { const k = iconFor(e); return <k.Icon size={16} strokeWidth={1.8} className={`shrink-0 ${k.color}`} title={k.label} aria-label={k.label} /> })()}
                        <span>{e.name}{e.is_link && <span className="text-xs text-text-secondary"> →</span>}</span>
                      </button>
                      {searching && <div className="ml-[23px] truncate font-mono text-[11px] text-text-tertiary">in {where}</div>}
                    </td>
                    <td className="whitespace-nowrap p-2 text-text-secondary">{e.is_dir ? '—' : formatBytes(e.size)}</td>
                    <td className="p-2 font-mono text-xs text-text-secondary">{e.mode}</td>
                    <td className="whitespace-nowrap p-2 text-text-secondary">{e.owner}:{e.group}</td>
                    <td className="whitespace-nowrap p-2 text-text-secondary">{formatShortDate(e.mod_time)}</td>
                    <td className="relative w-0 p-0">
                      <RowActions actions={main} more={more} open={moreFor === e.path} onToggleMore={() => setMoreFor((cur) => (cur === e.path ? null : e.path))} />
                    </td>
                  </tr>
                )
              })}
              {!entries.length && (
                <tr><td colSpan={colSpan} className="p-6 text-center text-text-secondary">
                  {term ? (searching && search.isFetching ? 'Searching…' : `Nothing matches “${term}”`) : 'Empty directory'}
                </td></tr>
              )}
            </tbody>
          </table>
        )}
      </div>
      {truncated && <p className="text-xs text-text-tertiary">Showing the first matches only. Narrow the search or open a deeper folder.</p>}

      {upload && (
        <UploadFlow
          dir={upload.dir}
          files={upload.files}
          existing={new Set(upload.dir === cwd ? (listed || []).map((e) => e.name) : [])}
          onFinished={refresh}
          onClose={() => { setUpload(null); refresh() }}
        />
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

      <Modal open={!!editor} onClose={closeEditor} size="xl" title={<span className="flex items-center gap-2 font-mono text-sm">{editor?.path}{dirty && <span className="h-2 w-2 rounded-full bg-warning" title="Unsaved changes" />}</span>}>
        {editor?.image && (
          <div className="flex max-h-[70vh] items-center justify-center overflow-auto rounded-md bg-[#1e1f22] p-4">
            <img src={api.downloadURL(editor.path)} alt={editor.path} className="max-h-[64vh] max-w-full object-contain" />
          </div>
        )}
        {editor && !editor.image && (
          <div className="space-y-3">
            <CodeEditor autoFocus path={editor.path} value={editor.content} onChange={(v) => setEditor((cur) => ({ ...cur, content: v }))} onSave={saveEditor} />
            <div className="flex items-center justify-between gap-2">
              <span className="text-xs text-text-tertiary">Tab indents · Ctrl/Cmd+S saves</span>
              <div className="flex gap-2">
                <Button variant="secondary" onClick={closeEditor}>Close</Button>
                <Button disabled={busy || !dirty} onClick={saveEditor}>Save</Button>
              </div>
            </div>
          </div>
        )}
      </Modal>
    </div>
  )
}

export default FileManager
