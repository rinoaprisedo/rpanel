import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { Terminal as XTerm } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { Plus, Pencil, Trash2, Play, CornerDownLeft, Search, RotateCw, Eraser, Maximize2, Minimize2, TerminalSquare, BookMarked } from 'lucide-react'
import Button from '../../components/ui/Button'
import Input from '../../components/ui/Input'
import Modal from '../../components/ui/Modal'
import ConfirmDialog from '../../components/ui/ConfirmDialog'
import Skeleton from '../../components/ui/Skeleton'
import usePermission from '../../hooks/usePermission'
import { cn } from '../../utils/cn'
import * as api from '../../api/terminal'

const apiBase = import.meta.env.VITE_API_URL || 'http://localhost:8800/api/v1'
const wsURL = () => {
  // VITE_API_URL may be absolute (dev) or relative like /api/v1 (behind the panel's nginx).
  if (apiBase.startsWith('/')) return `${window.location.protocol === 'https:' ? 'wss' : 'ws'}://${window.location.host}${apiBase}/terminal/ws`
  return apiBase.replace(/^http/, 'ws') + '/terminal/ws'
}

const errMsg = (e) => e.response?.data?.message || 'Operation failed'
const emptyForm = { name: '', description: '', command: '' }
const statusStyle = {
  connected: { dot: 'bg-success', text: 'Connected' },
  connecting: { dot: 'bg-warning animate-pulse', text: 'Connecting…' },
  closed: { dot: 'bg-danger', text: 'Disconnected' },
}

function Terminal() {
  const box = useRef(null)
  const termRef = useRef(null)
  const wsRef = useRef(null)
  const [status, setStatus] = useState('connecting')
  const [attempt, setAttempt] = useState(0) // bump to reconnect
  const [full, setFull] = useState(false)

  const qc = useQueryClient()
  const { canCreate, canEdit, canDelete } = usePermission()
  const { data, isLoading } = useQuery({ queryKey: ['terminal-templates'], queryFn: api.getTemplates })
  const templates = data?.data || []
  const [q, setQ] = useState('')
  const [form, setForm] = useState(null) // { uuid?, ...fields }
  const [del, setDel] = useState(null)

  const filtered = useMemo(() => {
    const s = q.trim().toLowerCase()
    if (!s) return templates
    return templates.filter((t) => `${t.name} ${t.description} ${t.command}`.toLowerCase().includes(s))
  }, [templates, q])

  const refresh = () => qc.invalidateQueries({ queryKey: ['terminal-templates'] })
  const m = (fn, ok, after) => ({ mutationFn: fn, onSuccess: () => { toast.success(ok); refresh(); after?.() }, onError: (e) => toast.error(errMsg(e)) })
  const save = useMutation(m((f) => {
    const body = { name: f.name, description: f.description, command: f.command }
    return f.uuid ? api.updateTemplate(f.uuid, body) : api.createTemplate(body)
  }, 'Template saved', () => setForm(null)))
  const remove = useMutation(m((t) => api.deleteTemplate(t.uuid), 'Template deleted', () => setDel(null)))

  useEffect(() => {
    const term = new XTerm({
      cursorBlink: true,
      cursorStyle: 'bar',
      fontSize: 13,
      lineHeight: 1.2,
      scrollback: 5000,
      fontFamily: '"JetBrains Mono", "Fira Code", Menlo, Consolas, "DejaVu Sans Mono", monospace',
      theme: {
        background: '#14130f', foreground: '#e6e2d9', cursor: '#f0a36b', selectionBackground: '#3d3a33',
        black: '#14130f', red: '#e5645a', green: '#8fc77a', yellow: '#e5b65a', blue: '#6aa6e8', magenta: '#c490e0', cyan: '#5cc4c4', white: '#e6e2d9',
        brightBlack: '#6b675c', brightRed: '#f07a70', brightGreen: '#a6dc92', brightYellow: '#f0c878', brightBlue: '#84baf5', brightMagenta: '#d6a8ee', brightCyan: '#78d8d8', brightWhite: '#ffffff',
      },
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(box.current)
    fit.fit()
    termRef.current = term

    const enc = new TextEncoder()
    const ws = new WebSocket(wsURL())
    wsRef.current = ws
    ws.binaryType = 'arraybuffer'
    const sendSize = () => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
      }
    }
    ws.onopen = () => {
      setStatus('connected')
      sendSize()
      term.focus()
    }
    ws.onmessage = (e) => term.write(typeof e.data === 'string' ? e.data : new Uint8Array(e.data))
    ws.onclose = (e) => {
      setStatus('closed')
      term.write(`\r\n\x1b[31m[disconnected${e.reason ? ': ' + e.reason : ''}]\x1b[0m\r\n`)
    }
    ws.onerror = () => setStatus('closed')

    const onData = term.onData((d) => ws.readyState === WebSocket.OPEN && ws.send(enc.encode(d)))
    const onBin = term.onBinary((d) => ws.readyState === WebSocket.OPEN && ws.send(Uint8Array.from(d, (c) => c.charCodeAt(0))))
    const onResize = term.onResize(sendSize)
    const ro = new ResizeObserver(() => fit.fit())
    ro.observe(box.current)

    return () => {
      ro.disconnect()
      onData.dispose()
      onBin.dispose()
      onResize.dispose()
      ws.close()
      term.dispose()
      termRef.current = null
      wsRef.current = null
    }
  }, [attempt])

  // The terminal pane changes size when toggling fullscreen; ResizeObserver refits it.
  useEffect(() => {
    if (!full) return
    const onKey = (e) => e.key === 'Escape' && setFull(false)
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [full])

  const reconnect = () => { setStatus('connecting'); setAttempt((n) => n + 1) }

  // Templates are typed into the user's own shell, so they pass through the same
  // WebSocket as keystrokes; `run` appends Enter, plain insert leaves it for review.
  const send = (cmd, run) => {
    const ws = wsRef.current
    if (!ws || ws.readyState !== WebSocket.OPEN) return toast.error('Terminal is not connected')
    // Bracketed paste keeps multi-line snippets from executing line by line before Enter.
    const body = cmd.includes('\n') ? `\x1b[200~${cmd}\x1b[201~` : cmd
    ws.send(new TextEncoder().encode(body + (run ? '\r' : '')))
    termRef.current?.focus()
  }

  const set = (k) => (e) => setForm({ ...form, [k]: e.target.value })
  const st = statusStyle[status]

  return (
    <div className={cn('flex gap-4', full ? 'fixed inset-0 z-40 bg-surface-bg p-4' : 'h-[calc(100vh-150px)] min-h-[480px]')}>
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden rounded-xl border border-surface-border bg-[#14130f] shadow-md">
        <div className="flex items-center gap-3 border-b border-white/10 bg-[#1d1b17] px-4 py-2.5">
          <div className="flex gap-1.5">
            <span className="h-3 w-3 rounded-full bg-[#ff5f57]" />
            <span className="h-3 w-3 rounded-full bg-[#febc2e]" />
            <span className="h-3 w-3 rounded-full bg-[#28c840]" />
          </div>
          <div className="flex items-center gap-2 text-xs text-gray-300">
            <TerminalSquare size={14} className="text-gray-400" />
            <span className="font-medium">Shell</span>
            <span className="text-gray-500">·</span>
            <span className={cn('h-2 w-2 rounded-full', st.dot)} />
            <span>{st.text}</span>
          </div>
          <div className="ml-auto flex items-center gap-1">
            <button title="Clear" onClick={() => termRef.current?.clear()} className="rounded-md p-1.5 text-gray-400 hover:bg-white/10 hover:text-white"><Eraser size={15} /></button>
            <button title="Reconnect" onClick={reconnect} className="rounded-md p-1.5 text-gray-400 hover:bg-white/10 hover:text-white"><RotateCw size={15} /></button>
            <button title={full ? 'Exit full screen (Esc)' : 'Full screen'} onClick={() => setFull(!full)} className="rounded-md p-1.5 text-gray-400 hover:bg-white/10 hover:text-white">
              {full ? <Minimize2 size={15} /> : <Maximize2 size={15} />}
            </button>
          </div>
        </div>
        <div className="min-h-0 flex-1 p-3"><div ref={box} className="h-full w-full" /></div>
        <div className="border-t border-white/10 bg-[#1d1b17] px-4 py-1.5 text-[11px] text-gray-500">
          Sessions are recorded in the activity log and close after idle timeout.
        </div>
      </div>

      <aside className="hidden w-80 shrink-0 flex-col overflow-hidden rounded-xl border border-surface-border bg-surface-card lg:flex">
        <div className="flex items-center justify-between border-b border-surface-border px-4 py-3">
          <div className="flex items-center gap-2 text-sm font-semibold text-text-primary"><BookMarked size={16} />Command templates</div>
          {canCreate('terminal') && <Button onClick={() => setForm(emptyForm)}><Plus size={14} />New</Button>}
        </div>
        <div className="relative border-b border-surface-border p-3">
          <Search size={14} className="pointer-events-none absolute left-5 top-1/2 -translate-y-1/2 text-text-tertiary" />
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Search templates…"
            className="h-9 w-full rounded-md border-[1.5px] border-surface-border bg-field pl-8 pr-3 text-sm outline-none focus:border-primary" />
        </div>
        <div className="min-h-0 flex-1 space-y-2 overflow-y-auto p-3">
          {isLoading && <Skeleton className="h-24 w-full" />}
          {filtered.map((t) => (
            <div key={t.uuid} className="group rounded-lg border border-surface-border p-3 transition-colors hover:border-primary/50">
              <div className="flex items-start justify-between gap-2">
                <div className="min-w-0">
                  <div className="truncate text-sm font-medium text-text-primary">{t.name}</div>
                  {t.description && <div className="truncate text-xs text-text-secondary">{t.description}</div>}
                </div>
                <div className="flex shrink-0 gap-0.5 opacity-0 transition-opacity group-hover:opacity-100">
                  {canEdit('terminal') && <button title="Edit" onClick={() => setForm({ ...emptyForm, ...t })} className="rounded p-1 text-text-secondary hover:bg-surface-hover"><Pencil size={13} /></button>}
                  {canDelete('terminal') && <button title="Delete" onClick={() => setDel(t)} className="rounded p-1 text-danger hover:bg-surface-hover"><Trash2 size={13} /></button>}
                </div>
              </div>
              <pre className="mt-2 max-h-20 overflow-hidden whitespace-pre-wrap break-all rounded-md bg-[#14130f] px-2 py-1.5 font-mono text-[11px] leading-snug text-gray-300">{t.command}</pre>
              <div className="mt-2 flex gap-2">
                <button onClick={() => send(t.command, true)} disabled={status !== 'connected'} className="flex flex-1 items-center justify-center gap-1 rounded-md bg-primary px-2 py-1.5 text-xs font-medium text-white disabled:opacity-40"><Play size={12} />Run</button>
                <button onClick={() => send(t.command, false)} disabled={status !== 'connected'} title="Type into the terminal without pressing Enter" className="flex flex-1 items-center justify-center gap-1 rounded-md border border-surface-border px-2 py-1.5 text-xs font-medium text-text-primary hover:bg-surface-hover disabled:opacity-40"><CornerDownLeft size={12} />Insert</button>
              </div>
            </div>
          ))}
          {!isLoading && !filtered.length && (
            <p className="p-6 text-center text-sm text-text-secondary">{templates.length ? 'No match' : 'No templates yet. Save frequently used commands here.'}</p>
          )}
        </div>
      </aside>

      <Modal open={!!form} onClose={() => setForm(null)} title={form?.uuid ? 'Edit template' : 'New template'} size="lg">
        {form && (
          <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); save.mutate(form) }}>
            <Input label="Name" placeholder="Disk usage" maxLength={100} value={form.name} onChange={set('name')} />
            <Input label="Description (optional)" placeholder="Show free space per mount" maxLength={255} value={form.description} onChange={set('description')} />
            <div className="flex flex-col gap-1">
              <label className="text-xs font-medium uppercase tracking-[.05em] text-text-secondary">Command</label>
              <textarea rows={6} spellCheck={false} placeholder="df -h" value={form.command} onChange={set('command')}
                className="w-full rounded-md border-[1.5px] border-surface-border bg-field p-3 font-mono text-xs outline-none focus:border-primary" />
            </div>
            <div className="flex justify-end gap-2">
              <Button type="button" variant="secondary" onClick={() => setForm(null)}>Cancel</Button>
              <Button type="submit" disabled={save.isPending || !form.name.trim() || !form.command.trim()}>{save.isPending ? 'Saving…' : 'Save'}</Button>
            </div>
          </form>
        )}
      </Modal>

      <ConfirmDialog open={!!del} onClose={() => setDel(null)} loading={remove.isPending} title={`Delete ${del?.name}`}
        message="This template is removed for everyone with terminal access." onConfirm={() => remove.mutate(del)} />
    </div>
  )
}

export default Terminal
