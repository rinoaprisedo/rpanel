import { useEffect, useRef, useState } from 'react'
import { Terminal as XTerm } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import Button from '../../components/ui/Button'

const apiBase = import.meta.env.VITE_API_URL || 'http://localhost:8800/api/v1'
const wsURL = () => {
  // VITE_API_URL may be absolute (dev) or relative like /api/v1 (behind the panel's nginx).
  if (apiBase.startsWith('/')) return `${window.location.protocol === 'https:' ? 'wss' : 'ws'}://${window.location.host}${apiBase}/terminal/ws`
  return apiBase.replace(/^http/, 'ws') + '/terminal/ws'
}

function Terminal() {
  const box = useRef(null)
  const [status, setStatus] = useState('connecting')
  const [attempt, setAttempt] = useState(0) // bump to reconnect

  useEffect(() => {
    const term = new XTerm({
      cursorBlink: true,
      fontSize: 13,
      fontFamily: 'Menlo, Consolas, "DejaVu Sans Mono", monospace',
      theme: { background: '#1a1917' },
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(box.current)
    fit.fit()

    const enc = new TextEncoder()
    const ws = new WebSocket(wsURL())
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
    }
  }, [attempt])

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between rounded-md border border-surface-border bg-surface-card px-3 py-2 text-sm">
        <span className="text-text-secondary">
          Status: <span className={status === 'connected' ? 'text-success' : 'text-danger'}>{status}</span>
          <span className="ml-3 text-xs">Sessions are recorded in the activity log and close after idle timeout.</span>
        </span>
        {status === 'closed' && <Button onClick={() => { setStatus('connecting'); setAttempt((n) => n + 1) }}>Reconnect</Button>}
      </div>
      <div className="rounded-lg bg-[#1a1917] p-2">
        <div ref={box} className="h-[65vh] w-full" />
      </div>
    </div>
  )
}

export default Terminal
