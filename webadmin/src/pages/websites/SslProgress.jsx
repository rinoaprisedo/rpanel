import { useEffect, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Check, LoaderCircle, TriangleAlert, X, Circle, Lock } from 'lucide-react'
import Modal from '../../components/ui/Modal'
import Button from '../../components/ui/Button'
import { cn } from '../../utils/cn'
import * as api from '../../api/websites'

function StepIcon({ status }) {
  if (status === 'done') return <span className="flex h-6 w-6 items-center justify-center rounded-full bg-success text-white"><Check size={14} strokeWidth={3} /></span>
  if (status === 'warn') return <span className="flex h-6 w-6 items-center justify-center rounded-full bg-warning text-white"><TriangleAlert size={13} /></span>
  if (status === 'failed') return <span className="flex h-6 w-6 items-center justify-center rounded-full bg-danger text-white"><X size={14} strokeWidth={3} /></span>
  if (status === 'running') return <span className="flex h-6 w-6 items-center justify-center rounded-full bg-primary/15 text-primary"><LoaderCircle size={15} className="animate-spin" /></span>
  return <span className="flex h-6 w-6 items-center justify-center text-text-tertiary"><Circle size={14} /></span>
}

// Live progress of a Let's Encrypt request: bar, steps and the certbot log.
// `target` is { uuid, domain } or null; onFinished fires once when the job ends (either way).
function SslProgress({ target, onClose, onFinished, onRetry }) {
  const uuid = target?.uuid
  const { data } = useQuery({
    queryKey: ['ssl-progress', uuid],
    queryFn: () => api.getSSLProgress(uuid),
    enabled: !!uuid,
    refetchInterval: (q) => (q.state.data?.data?.state === 'running' || !q.state.data ? 800 : false),
    retry: 3,
    gcTime: 0,
  })
  const p = data?.data
  const running = !p || p.state === 'running'
  const logRef = useRef(null)
  const notified = useRef(null)

  useEffect(() => { if (logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight }, [p?.log?.length])
  useEffect(() => {
    if (p && p.state !== 'running' && notified.current !== uuid) { notified.current = uuid; onFinished?.(p) }
  }, [p, uuid, onFinished])
  useEffect(() => { if (!uuid) notified.current = null }, [uuid])

  const percent = p?.percent ?? 0
  const failed = p?.state === 'failed'
  const done = p?.state === 'done'

  return (
    <Modal open={!!target} onClose={running ? () => {} : onClose} title={`HTTPS for ${target?.domain || ''}`} size="lg">
      <div className="space-y-5">
        <div>
          <div className="mb-1.5 flex items-center justify-between text-xs">
            <span className={cn('font-medium', failed ? 'text-danger' : done ? 'text-success' : 'text-text-secondary')}>
              {failed ? 'Failed' : done ? 'HTTPS is enabled' : 'Installing the Let’s Encrypt certificate…'}
            </span>
            <span className="font-mono text-text-tertiary">{percent}%{p ? ` · ${p.elapsed_seconds}s` : ''}</span>
          </div>
          <div className="h-2.5 overflow-hidden rounded-full bg-surface-hover" role="progressbar" aria-valuenow={percent} aria-valuemin={0} aria-valuemax={100}>
            <div
              className={cn('h-full rounded-full transition-[width] duration-500', failed ? 'bg-danger' : done ? 'bg-success' : 'bg-primary', running && 'animate-pulse')}
              style={{ width: `${Math.max(percent, 2)}%` }}
            />
          </div>
        </div>

        <ol className="space-y-2.5">
          {(p?.steps || []).map((s) => (
            <li key={s.key} className="flex items-start gap-3">
              <StepIcon status={s.status} />
              <div className="min-w-0 pt-0.5">
                <p className={cn('text-sm', s.status === 'pending' ? 'text-text-tertiary' : 'font-medium text-text-primary')}>{s.label}</p>
                {s.detail && <p className={cn('text-xs', s.status === 'failed' ? 'text-danger' : s.status === 'warn' ? 'text-warning' : 'text-text-tertiary')}>{s.detail}</p>}
              </div>
            </li>
          ))}
        </ol>

        {failed && (
          <div className="rounded-lg border border-danger/30 bg-danger-bg p-3 text-sm text-danger">
            <p className="mb-1 font-medium">The certificate could not be issued. The site keeps working over HTTP.</p>
            <pre className="whitespace-pre-wrap break-words font-mono text-xs">{p.error}</pre>
            <p className="mt-2 text-xs text-text-secondary">Check that the domain&apos;s A record points at this server and that port 80 is open, then try again.</p>
          </div>
        )}
        {done && (
          <div className="flex items-center gap-2 rounded-lg border border-success/30 bg-success-bg p-3 text-sm text-success">
            <Lock size={16} />https://{target?.domain} now uses a valid certificate and HTTP redirects to HTTPS.
          </div>
        )}

        <details className="group" open={failed}>
          <summary className="cursor-pointer text-xs font-medium text-text-secondary hover:text-text-primary">Installation log</summary>
          <pre ref={logRef} className="mt-2 max-h-48 overflow-auto rounded-md bg-[#0b0f14] p-3 font-mono text-[11px] leading-5 text-slate-300">
            {(p?.log || []).length ? p.log.map((l, i) => <div key={i} className={/^WARNING|failed|error/i.test(l) ? 'text-warning' : undefined}>{l}</div>) : 'Waiting…'}
          </pre>
        </details>

        <div className="flex justify-end gap-2">
          {failed && onRetry && <Button variant="secondary" onClick={onRetry}>Try again</Button>}
          <Button onClick={onClose} disabled={running}>{running ? 'Working…' : done ? 'Done' : 'Close'}</Button>
        </div>
      </div>
    </Modal>
  )
}

export default SslProgress
