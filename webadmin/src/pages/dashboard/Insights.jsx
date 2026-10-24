import { useState } from 'react'
import Modal from '../../components/ui/Modal'
import { Bell } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { OctagonAlert, TriangleAlert, Info, CircleCheck, ArrowRight } from 'lucide-react'
import { cn } from '../../utils/cn'

// Severity is never color alone: every level has its own icon and a text label.
const LEVEL = {
  critical: { icon: OctagonAlert, label: 'Critical', tone: 'text-danger', wash: 'bg-danger-bg' },
  warning: { icon: TriangleAlert, label: 'Warning', tone: 'text-warning', wash: 'bg-warning-bg' },
  info: { icon: Info, label: 'Notice', tone: 'text-info', wash: 'bg-info-bg' },
  ok: { icon: CircleCheck, label: 'Healthy', tone: 'text-success', wash: 'bg-success-bg' },
}

const SHOWN = 5

function Insights({ items, bare }) {
  const navigate = useNavigate()
  const [all, setAll] = useState(false)
  const list = all ? items : items.slice(0, SHOWN)
  const counts = items.reduce((m, i) => ({ ...m, [i.severity]: (m[i.severity] || 0) + 1 }), {})
  const problems = (counts.critical || 0) + (counts.warning || 0)

  return (
    <section className={bare ? '' : 'rounded-lg border border-surface-border bg-surface-card'}>
      <header className={cn('flex flex-wrap items-center justify-between gap-2 px-4 py-3', bare ? '' : 'border-b border-surface-border')}>
        {!bare && <h2 className="text-sm font-semibold text-text-primary">Needs attention</h2>}
        <p className="text-xs text-text-tertiary">
          {items[0]?.severity === 'ok' ? 'No issues found' : `${problems} to fix${counts.info ? ` · ${counts.info} notice${counts.info > 1 ? 's' : ''}` : ''}`}
        </p>
      </header>
      <ul className="divide-y divide-surface-border">
        {list.map((it) => {
          const L = LEVEL[it.severity] || LEVEL.info
          const Icon = L.icon
          return (
            <li key={it.id} className="flex items-start gap-3 px-4 py-3">
              <span className={cn('mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-md', L.wash, L.tone)}><Icon size={15} strokeWidth={2} /></span>
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium text-text-primary">
                  <span className={cn('mr-2 text-[11px] font-semibold uppercase tracking-wide', L.tone)}>{L.label}</span>{it.title}
                </p>
                <p className="mt-0.5 text-xs leading-5 text-text-secondary">{it.detail}</p>
              </div>
              {it.link && (
                <button onClick={() => navigate(it.link)} className="mt-0.5 inline-flex shrink-0 items-center gap-1 text-xs font-medium text-primary hover:underline">
                  Open<ArrowRight size={12} />
                </button>
              )}
            </li>
          )
        })}
      </ul>
      {items.length > SHOWN && (
        <button onClick={() => setAll((v) => !v)} className="w-full border-t border-surface-border py-2 text-xs font-medium text-primary hover:bg-surface-hover">
          {all ? 'Show fewer' : `Show ${items.length - SHOWN} more`}
        </button>
      )}
    </section>
  )
}

// Compact trigger: shows only a count until clicked, then opens the full list.
export function AttentionButton({ items }) {
  const [open, setOpen] = useState(false)
  const real = (items || []).filter((i) => i.severity !== 'ok')
  const worst = real.some((i) => i.severity === 'critical') ? 'critical' : real.some((i) => i.severity === 'warning') ? 'warning' : real.length ? 'info' : 'ok'
  const L = LEVEL[worst]
  return (
    <>
      <button
        onClick={() => setOpen(true)}
        className="inline-flex h-9 items-center gap-2 rounded-full border border-surface-border bg-surface-card px-3.5 text-xs font-medium text-text-primary shadow-card transition-colors hover:border-surface-border-hover hover:bg-surface-hover"
        aria-label={real.length ? `${real.length} items need attention` : 'No items need attention'}
      >
        {real.length ? <Bell size={14} className={L.tone} /> : <CircleCheck size={14} className="text-success" />}
        Needs attention
        {real.length > 0 && <span className={cn('flex h-5 min-w-[20px] items-center justify-center rounded-full px-1.5 text-[11px] font-semibold', worst === 'critical' ? 'bg-danger text-white' : worst === 'warning' ? 'bg-warning text-white' : 'bg-info text-white')}>{real.length}</span>}
      </button>
      <Modal open={open} onClose={() => setOpen(false)} title="Needs attention" size="lg">
        <div className="-mx-[26px] -my-6"><Insights items={items || []} bare /></div>
      </Modal>
    </>
  )
}

export default Insights
