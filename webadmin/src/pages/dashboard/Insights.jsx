import { useState } from 'react'
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

function Insights({ items }) {
  const navigate = useNavigate()
  const [all, setAll] = useState(false)
  const list = all ? items : items.slice(0, SHOWN)
  const counts = items.reduce((m, i) => ({ ...m, [i.severity]: (m[i.severity] || 0) + 1 }), {})
  const problems = (counts.critical || 0) + (counts.warning || 0)

  return (
    <section className="rounded-lg border border-surface-border bg-surface-card">
      <header className="flex flex-wrap items-center justify-between gap-2 border-b border-surface-border px-4 py-3">
        <h2 className="text-sm font-semibold text-text-primary">Needs attention</h2>
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

export default Insights
