import { useEffect, useMemo, useRef, useState } from 'react'
import { CircleCheck, CircleX, Loader2, SkipForward, TriangleAlert, Clock, Ban } from 'lucide-react'
import Modal from '../../components/ui/Modal'
import Button from '../../components/ui/Button'
import { cn } from '../../utils/cn'
import { formatBytes } from '../../utils/format'
import * as api from '../../api/fileManager'

const errMsg = (e) => (e.response?.status === 409 ? 'Already exists' : e.response?.data?.message || e.message || 'Upload failed')

function Bar({ value, tone = 'bg-primary' }) {
  return (
    <div className="h-2 w-full overflow-hidden rounded-full bg-surface-hover">
      <div className={cn('h-full rounded-full transition-[width] duration-200', tone)} style={{ width: `${Math.max(0, Math.min(100, value))}%` }} />
    </div>
  )
}

const STATUS = {
  waiting: { icon: Clock, tone: 'text-text-tertiary', label: 'Waiting' },
  uploading: { icon: Loader2, tone: 'text-primary', label: 'Uploading' },
  done: { icon: CircleCheck, tone: 'text-success', label: 'Uploaded' },
  skipped: { icon: SkipForward, tone: 'text-text-tertiary', label: 'Skipped' },
  failed: { icon: CircleX, tone: 'text-danger', label: 'Failed' },
  cancelled: { icon: Ban, tone: 'text-text-tertiary', label: 'Cancelled' },
}

// Confirm → upload with live progress → summary. Files are sent one by one so each
// has its own progress, and existing names can be replaced or skipped individually.
function UploadFlow({ dir, files, existing, onFinished, onClose }) {
  const [phase, setPhase] = useState('confirm') // confirm | uploading | done
  const [items, setItems] = useState(() => files.map((file) => ({
    file, exists: existing.has(file.name), action: existing.has(file.name) ? 'skip' : 'upload', status: 'waiting', loaded: 0, error: '',
  })))
  const [startedAt, setStartedAt] = useState(0)
  const [now, setNow] = useState(0)
  const abort = useRef(null)
  const cancelled = useRef(false)

  const conflicts = items.filter((i) => i.exists).length
  const toSend = items.filter((i) => i.action !== 'skip')
  const totalBytes = useMemo(() => toSend.reduce((n, i) => n + i.file.size, 0), [items]) // eslint-disable-line react-hooks/exhaustive-deps
  const sentBytes = items.reduce((n, i) => n + (i.action === 'skip' ? 0 : i.status === 'done' ? i.file.size : i.loaded), 0)
  const pct = totalBytes ? (sentBytes / totalBytes) * 100 : 100
  const current = items.find((i) => i.status === 'uploading')

  useEffect(() => {
    if (phase !== 'uploading') return
    const t = setInterval(() => setNow(Date.now()), 500)
    return () => clearInterval(t)
  }, [phase])

  const setAll = (action) => setItems((list) => list.map((i) => (i.exists ? { ...i, action } : i)))
  const setOne = (idx, patch) => setItems((list) => list.map((i, n) => (n === idx ? { ...i, ...patch } : i)))

  const start = async () => {
    setPhase('uploading')
    setStartedAt(Date.now())
    cancelled.current = false
    // Skipped files are only marked, never sent.
    setItems((list) => list.map((i) => (i.action === 'skip' ? { ...i, status: 'skipped' } : i)))
    for (let idx = 0; idx < items.length; idx++) {
      const it = items[idx]
      if (it.action === 'skip') continue
      if (cancelled.current) { setOne(idx, { status: 'cancelled' }); continue }
      setOne(idx, { status: 'uploading', loaded: 0 })
      abort.current = new AbortController()
      try {
        await api.uploadFile(dir, it.file, {
          overwrite: it.action === 'replace',
          signal: abort.current.signal,
          onProgress: (loaded) => setOne(idx, { loaded: Math.min(loaded, it.file.size) }),
        })
        setOne(idx, { status: 'done', loaded: it.file.size })
      } catch (e) {
        setOne(idx, { status: cancelled.current ? 'cancelled' : 'failed', error: cancelled.current ? '' : errMsg(e) })
      }
    }
    abort.current = null
    setPhase('done')
    onFinished?.()
  }

  const cancel = () => {
    cancelled.current = true
    abort.current?.abort()
  }

  const counts = items.reduce((m, i) => ({ ...m, [i.status]: (m[i.status] || 0) + 1 }), {})
  const doneCount = (counts.done || 0) + (counts.failed || 0)
  const elapsed = Math.max(1, (now - startedAt) / 1000)
  const speed = sentBytes / elapsed
  const eta = speed > 0 && phase === 'uploading' ? Math.max(0, Math.round((totalBytes - sentBytes) / speed)) : null

  const busy = phase === 'uploading'
  const title = phase === 'confirm' ? 'Upload files?' : phase === 'uploading' ? 'Uploading…' : 'Upload finished'

  return (
    <Modal open onClose={busy ? () => {} : onClose} title={title} size="lg">
      <div className="space-y-4">
        <p className="text-sm text-text-secondary">
          {phase === 'confirm'
            ? <>Upload <b className="text-text-primary">{items.length} file{items.length > 1 ? 's' : ''}</b> ({formatBytes(items.reduce((n, i) => n + i.file.size, 0))}) to </>
            : <>Destination </>}
          <code className="rounded bg-surface-hover px-1.5 py-0.5 font-mono text-xs text-text-primary">{dir}</code>
        </p>

        {phase === 'confirm' && conflicts > 0 && (
          <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-warning/40 bg-warning-bg px-3 py-2 text-sm">
            <span className="flex items-center gap-2 text-warning"><TriangleAlert size={15} />{conflicts} file{conflicts > 1 ? 's' : ''} already exist{conflicts === 1 ? 's' : ''} in this folder</span>
            <span className="flex gap-3 text-xs font-medium">
              <button className="text-primary hover:underline" onClick={() => setAll('replace')}>Replace all</button>
              <button className="text-primary hover:underline" onClick={() => setAll('skip')}>Skip all</button>
            </span>
          </div>
        )}

        {phase !== 'confirm' && (
          <div className="space-y-1.5">
            <div className="flex items-baseline justify-between text-xs text-text-secondary">
              <span className="text-sm font-semibold text-text-primary">{Math.round(pct)}%</span>
              <span>
                {formatBytes(sentBytes)} of {formatBytes(totalBytes)}
                {busy && speed > 0 && <> · {formatBytes(speed)}/s{eta !== null && <> · {eta < 60 ? `${eta}s` : `${Math.floor(eta / 60)}m ${eta % 60}s`} left</>}</>}
              </span>
            </div>
            <Bar value={pct} tone={phase === 'done' && counts.failed ? 'bg-warning' : 'bg-primary'} />
            <p className="text-xs text-text-tertiary">
              {busy ? `${Math.min(doneCount + 1, toSend.length)} of ${toSend.length}${current ? ` · ${current.file.name}` : ''}`
                : `${counts.done || 0} uploaded${counts.skipped ? ` · ${counts.skipped} skipped` : ''}${counts.failed ? ` · ${counts.failed} failed` : ''}${counts.cancelled ? ` · ${counts.cancelled} cancelled` : ''}`}
            </p>
          </div>
        )}

        <ul className="max-h-72 divide-y divide-surface-border overflow-y-auto rounded-lg border border-surface-border">
          {items.map((i, idx) => {
            const S = STATUS[i.status]
            const Icon = S.icon
            return (
              <li key={idx} className="px-3 py-2.5 text-sm">
                <div className="flex items-center gap-3">
                  {phase !== 'confirm' && <Icon size={15} className={cn('shrink-0', S.tone, i.status === 'uploading' && 'animate-spin')} />}
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-medium text-text-primary" title={i.file.name}>{i.file.name}</p>
                    <p className="text-xs text-text-tertiary">
                      {formatBytes(i.file.size)}
                      {phase !== 'confirm' && <> · <span className={S.tone}>{i.status === 'failed' ? i.error : S.label}</span></>}
                      {phase === 'confirm' && i.exists && <> · <span className="text-warning">already exists</span></>}
                    </p>
                  </div>
                  {phase === 'confirm' && i.exists && (
                    <div className="flex shrink-0 gap-1 rounded-lg bg-surface-hover p-0.5 text-xs font-medium">
                      {[['replace', 'Replace'], ['skip', 'Skip']].map(([k, l]) => (
                        <button key={k} onClick={() => setOne(idx, { action: k })}
                          className={cn('rounded-md px-2.5 py-1 transition-colors', i.action === k ? (k === 'replace' ? 'bg-danger text-white' : 'bg-surface-card text-text-primary shadow-sm') : 'text-text-secondary hover:text-text-primary')}>{l}</button>
                      ))}
                    </div>
                  )}
                </div>
                {i.status === 'uploading' && <div className="mt-2 pl-7"><Bar value={(i.loaded / Math.max(1, i.file.size)) * 100} /></div>}
              </li>
            )
          })}
        </ul>

        <div className="flex justify-end gap-2">
          {phase === 'confirm' && (
            <>
              <Button variant="secondary" onClick={onClose}>Cancel</Button>
              <Button onClick={start} disabled={!toSend.length}>Upload{toSend.length ? ` ${toSend.length} file${toSend.length > 1 ? 's' : ''}` : ''}</Button>
            </>
          )}
          {phase === 'uploading' && <Button variant="secondary" onClick={cancel}>Cancel upload</Button>}
          {phase === 'done' && <Button onClick={onClose}>Close</Button>}
        </div>
      </div>
    </Modal>
  )
}

export default UploadFlow
