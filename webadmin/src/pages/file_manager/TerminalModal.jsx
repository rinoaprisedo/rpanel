import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { Terminal as XTerm } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { RotateCw, Play, CornerDownLeft, Search, BookMarked } from 'lucide-react'
import Modal from '../../components/ui/Modal'
import Skeleton from '../../components/ui/Skeleton'
import { cn } from '../../utils/cn'
import * as api from '../../api/terminal'

const apiBase = import.meta.env.VITE_API_URL || 'http://localhost:8800/api/v1'
const wsURL = (cwd) => {
  const base = apiBase.startsWith('/')
    ? `${window.location.protocol === 'https:' ? 'wss' : 'ws'}://${window.location.host}${apiBase}/terminal/ws`
    : apiBase.replace(/^http/, 'ws') + '/terminal/ws'
  return cwd ? `${base}?cwd=${encodeURIComponent(cwd)}` : base
}

// Shell opened in a popup, starting in the given folder (same websocket as the Terminal page).
function TerminalModal({ cwd, onClose }) {
  const box = useRef(null)
  const [status, setStatus] = useState('connecting')
  const [attempt, setAttempt] = useState(0)
  const wsRef = useRef(null)
  const termRef = useRef(null)
  const [q, setQ] = useState('')
  const { data, isLoading } = useQuery({ queryKey: ['terminal-templates'], queryFn: api.getTemplates })
  const templates = data?.data || []
  const filtered = useMemo(() => {
    const s = q.trim().toLowerCase()
    return s ? templates.filter((t) => `${t.name} ${t.description} ${t.command}`.toLowerCase().includes(s)) : templates
  }, [templates, q])

  // Same as the Terminal page: templates are typed into the shell like keystrokes.
  const send = (cmd, run) => {
    const ws = wsRef.current
    if (!ws || ws.readyState !== WebSocket.OPEN) return toast.error('Terminal is not connected')
    const body = cmd.includes('\n') ? `\x1b[200~${cmd}\x1b[201~` : cmd
    ws.send(new TextEncoder().encode(body + (run ? '\r' : '')))
    termRef.current?.focus()
  }

  useEffect(() => {
    // Modal renders through a portal; wait a tick so the container has its final size.
    const term = new XTerm({
      cursorBlink: true, cursorStyle: 'bar', fontSize: 13, lineHeight: 1.2, scrollback: 5000,
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

    const enc = new TextEncoder()
    const ws = new WebSocket(wsURL(cwd))
    ws.binaryType = 'arraybuffer'
    wsRef.current = ws
    termRef.current = term
    const sendSize = () => ws.readyState === WebSocket.OPEN && ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
    ws.onopen = () => { setStatus('connected'); sendSize(); term.focus() }
    ws.onmessage = (e) => term.write(typeof e.data === 'string' ? e.data : new Uint8Array(e.data))
    ws.onclose = (e) => { setStatus('closed'); term.write(`\r\n\x1b[31m[disconnected${e.reason ? ': ' + e.reason : ''}]\x1b[0m\r\n`) }
    ws.onerror = () => setStatus('closed')
    const onData = term.onData((d) => ws.readyState === WebSocket.OPEN && ws.send(enc.encode(d)))
    const onBin = term.onBinary((d) => ws.readyState === WebSocket.OPEN && ws.send(Uint8Array.from(d, (c) => c.charCodeAt(0))))
    const onResize = term.onResize(sendSize)
    const ro = new ResizeObserver(() => fit.fit())
    ro.observe(box.current)

    return () => { ro.disconnect(); onData.dispose(); onBin.dispose(); onResize.dispose(); ws.close(); term.dispose() }
  }, [attempt, cwd])

  const dot = { connected: 'bg-success', connecting: 'bg-warning animate-pulse', closed: 'bg-danger' }[status]
  const title = (
    <span className="flex items-center gap-2 text-sm">
      <span className={cn('h-2 w-2 rounded-full', dot)} />
      Terminal
      <span className="max-w-[40vw] truncate font-mono text-xs text-text-secondary" title={cwd}>{cwd}</span>
      <button title="Reconnect" onClick={() => { setStatus('connecting'); setAttempt((n) => n + 1) }} className="rounded-md p-1 text-text-secondary hover:bg-surface-hover"><RotateCw size={14} /></button>
    </span>
  )

  return (
    <Modal open onClose={onClose} size="2xl" title={title}>
      <div className="flex h-[60vh] min-h-[320px] gap-4">
        <div className="min-w-0 flex-1 rounded-lg bg-[#14130f] p-3"><div ref={box} className="h-full w-full" /></div>
        <aside className="hidden w-72 shrink-0 flex-col overflow-hidden rounded-lg border border-surface-border md:flex">
          <div className="flex items-center gap-2 border-b border-surface-border px-3 py-2.5 text-sm font-semibold text-text-primary"><BookMarked size={15} />Command templates</div>
          <div className="relative border-b border-surface-border p-2">
            <Search size={14} className="pointer-events-none absolute left-4 top-1/2 -translate-y-1/2 text-text-tertiary" />
            <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Search templates…"
              className="h-8 w-full rounded-md border-[1.5px] border-surface-border bg-field pl-8 pr-3 text-sm outline-none focus:border-primary" />
          </div>
          <div className="min-h-0 flex-1 space-y-2 overflow-y-auto p-2">
            {isLoading && <Skeleton className="h-20 w-full" />}
            {filtered.map((t) => (
              <div key={t.uuid} className="rounded-lg border border-surface-border p-2.5 transition-colors hover:border-primary/50">
                <div className="truncate text-sm font-medium text-text-primary">{t.name}</div>
                {t.description && <div className="truncate text-xs text-text-secondary">{t.description}</div>}
                <pre className="mt-1.5 max-h-16 overflow-hidden whitespace-pre-wrap break-all rounded-md bg-[#14130f] px-2 py-1 font-mono text-[11px] leading-snug text-gray-300">{t.command}</pre>
                <div className="mt-2 flex gap-2">
                  <button onClick={() => send(t.command, true)} disabled={status !== 'connected'} className="flex flex-1 items-center justify-center gap-1 rounded-md bg-primary px-2 py-1 text-xs font-medium text-white disabled:opacity-40"><Play size={12} />Run</button>
                  <button onClick={() => send(t.command, false)} disabled={status !== 'connected'} title="Type into the terminal without pressing Enter" className="flex flex-1 items-center justify-center gap-1 rounded-md border border-surface-border px-2 py-1 text-xs font-medium text-text-primary hover:bg-surface-hover disabled:opacity-40"><CornerDownLeft size={12} />Insert</button>
                </div>
              </div>
            ))}
            {!isLoading && !filtered.length && <p className="p-4 text-center text-xs text-text-secondary">{templates.length ? 'No match' : 'No templates yet. Add them on the Terminal page.'}</p>}
          </div>
        </aside>
      </div>
    </Modal>
  )
}

export default TerminalModal
