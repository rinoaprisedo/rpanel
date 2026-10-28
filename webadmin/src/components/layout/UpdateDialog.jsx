import { useEffect, useRef, useState } from 'react'
import { LoaderCircle, Check, X, ExternalLink } from 'lucide-react'
import toast from 'react-hot-toast'
import Modal from '../ui/Modal'
import Button from '../ui/Button'
import Markdown from '../ui/Markdown'
import { getSelfUpdateJob, startSelfUpdate } from '../../api/selfUpdate'
import { getVersion } from '../../api/system'
import { cn } from '../../utils/cn'

export function fmtDate(s) {
  if (!s) return ''
  const d = new Date(s)
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleDateString(undefined, { day: '2-digit', month: 'short', year: 'numeric' })
}

// While the update runs the panel restarts: wait for it to answer again with the new version, then reload.
function useUpdateRun(target, active) {
  const [phase, setPhase] = useState('running') // running | restarting | done | failed
  const [log, setLog] = useState([])
  const [secs, setSecs] = useState(0)
  useEffect(() => {
    if (!active) return undefined
    let stopped = false
    let wentDown = false
    const t0 = Date.now()
    const tick = async () => {
      if (stopped) return
      setSecs(Math.floor((Date.now() - t0) / 1000))
      try {
        const v = await getVersion()
        const now = v?.data?.version
        if (now === target) {
          setPhase('done')
          setTimeout(() => window.location.reload(), 1800)
          return
        }
        // Same old version answering again: the update either has not begun, or rolled back.
        const j = await getSelfUpdateJob().catch(() => null)
        if (j?.data?.job) {
          setLog(j.data.job.log || [])
          if (j.data.job.state === 'failed' || (wentDown && j.data.job.state !== 'running')) { setPhase('failed'); return }
          setPhase(wentDown ? 'restarting' : 'running')
        }
      } catch {
        wentDown = true
        setPhase('restarting')
      }
      if (Date.now() - t0 > 10 * 60 * 1000) { setPhase('failed'); return }
      setTimeout(tick, 2000)
    }
    const first = setTimeout(tick, 1500)
    return () => { stopped = true; clearTimeout(first) }
  }, [active, target])
  return { phase, log, secs }
}

function UpdateDialog({ info, onClose, onSkip }) {
  const [started, setStarted] = useState(false)
  const [starting, setStarting] = useState(false)
  const { phase, log, secs } = useUpdateRun(info.latest, started)
  const logRef = useRef(null)
  useEffect(() => { if (logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight }, [log.length])

  const start = async () => {
    setStarting(true)
    try {
      await startSelfUpdate()
      setStarted(true)
    } catch (e) {
      toast.error(e.response?.data?.message || 'Could not start the update', { duration: 8000 })
    } finally {
      setStarting(false)
    }
  }
  const busy = started && phase !== 'failed' && phase !== 'done'

  return (
    <Modal open onClose={busy ? () => {} : onClose} title={started ? `Updating to RPanel ${info.latest}` : `RPanel ${info.latest} is available`} size="lg">
      {!started ? (
        <div className="space-y-4">
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <span className="rounded-md bg-surface-hover px-2 py-1 font-mono text-xs text-text-secondary">v{info.current}</span>
            <span className="text-text-tertiary">→</span>
            <span className="rounded-md bg-primary/10 px-2 py-1 font-mono text-xs font-semibold text-primary">v{info.latest}</span>
            {info.published_at && <span className="text-xs text-text-tertiary">· released {fmtDate(info.published_at)}</span>}
            {info.url && <a href={info.url} target="_blank" rel="noopener noreferrer" className="ml-auto inline-flex items-center gap-1 text-xs text-primary hover:underline">GitHub <ExternalLink size={12} /></a>}
          </div>
          <div className="max-h-[48vh] overflow-y-auto rounded-lg border border-surface-border p-4">
            {info.releases.map((r, i) => (
              <section key={r.tag} className={cn(i > 0 && 'mt-5 border-t border-surface-border pt-4')}>
                {info.releases.length > 1 && <p className="mb-1 flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-text-tertiary">v{r.tag}{r.published_at && <span className="font-normal normal-case">{fmtDate(r.published_at)}</span>}</p>}
                {r.body?.trim() ? <Markdown text={r.body} /> : <p className="text-sm text-text-tertiary">No release notes.</p>}
              </section>
            ))}
          </div>
          {!info.can_update && (
            <p className="rounded-md bg-warning-bg p-3 text-xs text-warning">
              One-click update is not available on this server. Run <code className="font-mono">sudo rpanel-update</code> in a terminal once; later updates can be done from here.
            </p>
          )}
          <p className="text-xs text-text-tertiary">The panel restarts for a few seconds. Your websites keep running. If anything fails the previous version is restored automatically.</p>
          <div className="flex justify-end gap-2">
            {onSkip && <Button variant="secondary" onClick={onSkip}>Skip this version</Button>}
            {!onSkip && <Button variant="secondary" onClick={onClose}>Close</Button>}
            <Button onClick={start} disabled={starting || !info.can_update}>{starting ? 'Starting…' : 'Update now'}</Button>
          </div>
        </div>
      ) : (
        <div className="space-y-4">
          <div className="flex items-center gap-3">
            {phase === 'done' ? <span className="flex h-9 w-9 items-center justify-center rounded-full bg-success text-white"><Check size={18} /></span>
              : phase === 'failed' ? <span className="flex h-9 w-9 items-center justify-center rounded-full bg-danger text-white"><X size={18} /></span>
                : <LoaderCircle size={30} className="animate-spin text-primary" />}
            <div>
              <p className="text-sm font-medium text-text-primary">
                {phase === 'done' ? `RPanel ${info.latest} is running. Reloading…` : phase === 'failed' ? 'The update did not complete' : phase === 'restarting' ? 'Restarting the panel…' : 'Downloading and installing…'}
              </p>
              <p className="text-xs text-text-tertiary">{secs}s{phase === 'restarting' ? ' · this page reloads automatically' : ''}</p>
            </div>
          </div>
          <div className="h-2.5 overflow-hidden rounded-full bg-surface-hover">
            <div className={cn('h-full rounded-full transition-all duration-700', phase === 'failed' ? 'bg-danger' : phase === 'done' ? 'bg-success' : 'animate-pulse bg-primary')}
              style={{ width: phase === 'done' ? '100%' : phase === 'failed' ? '100%' : phase === 'restarting' ? '85%' : `${Math.min(70, 8 + secs)}%` }} />
          </div>
          {phase === 'failed' && (
            <div className="rounded-lg border border-danger/30 bg-danger-bg p-3 text-sm text-danger">
              The panel is back on v{info.current} (the previous version was restored). Details below; you can try again later.
            </div>
          )}
          {log.length > 0 && (
            <pre ref={logRef} className="max-h-56 overflow-auto rounded-md bg-[#0b0f14] p-3 font-mono text-[11px] leading-5 text-slate-300">
              {log.map((l, i) => <div key={i} className={/error|failed|warning/i.test(l) ? 'text-warning' : undefined}>{l}</div>)}
            </pre>
          )}
          {phase === 'failed' && <div className="flex justify-end"><Button onClick={onClose}>Close</Button></div>}
        </div>
      )}
    </Modal>
  )
}

export default UpdateDialog
