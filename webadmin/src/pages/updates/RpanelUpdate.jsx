import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { RefreshCw, CircleCheck, ArrowUpCircle, ChevronDown, ExternalLink, Clock } from 'lucide-react'
import Button from '../../components/ui/Button'
import Badge from '../../components/ui/Badge'
import Markdown from '../../components/ui/Markdown'
import Skeleton from '../../components/ui/Skeleton'
import UpdateDialog, { fmtDate } from '../../components/layout/UpdateDialog'
import useAuthStore from '../../store/authStore'
import { cn } from '../../utils/cn'
import { getSelfUpdate } from '../../api/selfUpdate'

function ago(ts) {
  if (!ts) return 'never'
  const m = Math.round((Date.now() / 1000 - ts) / 60)
  if (m < 1) return 'just now'
  if (m < 60) return `${m} min ago`
  return `${Math.round(m / 60)} h ago`
}

function ReleaseItem({ r, tag, defaultOpen }) {
  const [open, setOpen] = useState(!!defaultOpen)
  return (
    <li className="rounded-lg border border-surface-border">
      <button onClick={() => setOpen((v) => !v)} aria-expanded={open} className="flex w-full items-center gap-3 px-4 py-3 text-left">
        <span className="font-mono text-sm font-semibold text-text-primary">v{r.tag}</span>
        {tag}
        <span className="text-xs text-text-tertiary">{fmtDate(r.published_at)}</span>
        {r.name && r.name !== `RPanel ${r.tag}` && <span className="hidden truncate text-xs text-text-secondary sm:block">{r.name.replace(/^RPanel\s*[\d.]+\s*[—-]?\s*/, '')}</span>}
        <ChevronDown size={16} className={cn('ml-auto shrink-0 text-text-tertiary transition-transform', open && 'rotate-180')} />
      </button>
      {open && (
        <div className="border-t border-surface-border px-4 pb-4 pt-1">
          {r.body?.trim() ? <Markdown text={r.body} /> : <p className="py-2 text-sm text-text-tertiary">No release notes.</p>}
          {r.url && <a href={r.url} target="_blank" rel="noopener noreferrer" className="mt-1 inline-flex items-center gap-1 text-xs text-primary hover:underline">View on GitHub <ExternalLink size={12} /></a>}
        </div>
      )}
    </li>
  )
}

function RpanelUpdate() {
  const qc = useQueryClient()
  const isSuper = useAuthStore((s) => !!s.user?.is_superadmin)
  const { data, isLoading, isError } = useQuery({ queryKey: ['self-update'], queryFn: () => getSelfUpdate(), staleTime: 10 * 60 * 1000, retry: false })
  const [checking, setChecking] = useState(false)
  const [dialog, setDialog] = useState(false)
  const info = data?.data

  const check = async () => {
    setChecking(true)
    try {
      const res = await getSelfUpdate(true)
      qc.setQueryData(['self-update'], res) // also refreshes the top-bar badge
      if (res.data.available) toast(`RPanel ${res.data.latest} is available`, { icon: '⬆️' })
      else toast.success('RPanel is up to date')
    } catch (e) {
      toast.error(e.response?.data?.message || 'Could not reach GitHub. Try again in a moment.')
    } finally {
      setChecking(false)
    }
  }

  if (isLoading) return <Skeleton className="h-56 w-full" />

  const available = !!info?.available
  const history = info?.history || []

  return (
    <div className="max-w-3xl space-y-6">
      <section className={cn('flex flex-wrap items-center justify-between gap-4 rounded-xl border p-5', available ? 'border-primary/40 bg-primary/5' : 'border-success/30 bg-success-bg')}>
        <div className="flex items-center gap-4">
          <span className={cn('flex h-12 w-12 items-center justify-center rounded-2xl text-white', available ? 'bg-primary' : 'bg-success')}>
            {available ? <ArrowUpCircle size={24} /> : <CircleCheck size={24} />}
          </span>
          <div>
            <h3 className="text-base font-semibold text-text-primary">
              {isError || !info ? 'Update status unavailable' : available ? `RPanel ${info.latest} is available` : 'RPanel is up to date'}
            </h3>
            <p className="text-sm text-text-secondary">
              Installed <span className="font-mono">v{info?.current || '—'}</span>
              {info?.checked_at ? <span className="ml-2 inline-flex items-center gap-1 text-xs text-text-tertiary"><Clock size={11} />checked {ago(info.checked_at)}</span> : null}
            </p>
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="secondary" onClick={check} disabled={checking}><RefreshCw size={14} className={checking ? 'animate-spin' : ''} />{checking ? 'Checking…' : 'Check for updates'}</Button>
          {available && isSuper && <Button onClick={() => setDialog(true)}>View changes &amp; update</Button>}
        </div>
      </section>

      {isError && <p className="text-sm text-danger">Could not reach GitHub to look for new releases. Check the server&apos;s internet access.</p>}

      {available && (
        <section>
          <h3 className="mb-2 text-sm font-semibold text-text-primary">What&apos;s new</h3>
          <ul className="space-y-2">
            {info.releases.map((r, i) => <ReleaseItem key={r.tag} r={r} defaultOpen={i === 0} tag={<Badge variant="primary">new</Badge>} />)}
          </ul>
          {!isSuper && <p className="mt-2 text-xs text-text-tertiary">Only a superadmin can install updates.</p>}
        </section>
      )}

      {history.length > 0 && (
        <section>
          <h3 className="mb-2 text-sm font-semibold text-text-primary">Update history</h3>
          <ul className="space-y-2">
            {history.map((r, i) => (
              <ReleaseItem key={r.tag} r={r} defaultOpen={!available && i === 0} tag={r.tag === info.current ? <Badge variant="success">installed</Badge> : null} />
            ))}
          </ul>
        </section>
      )}

      {dialog && info && <UpdateDialog info={info} onClose={() => setDialog(false)} />}
    </div>
  )
}

export default RpanelUpdate
