import { Gauge, Zap } from 'lucide-react'
import HealthGauge, { levelColor } from './HealthGauge'
import Donut from './Donut'
import { scoreColor } from '../../components/charts/severity'

const barColor = scoreColor

// The one-glance answer to "is this server running well?", with the measurements behind it.
function HealthCard({ health, hostname, subtitle, right }) {
  const worst = health.factors.find((f) => f.id === health.bottleneck)
  const color = levelColor(health.level)
  return (
    <section
      className="overflow-hidden rounded-xl border border-surface-border bg-surface-card"
      style={{ backgroundImage: `linear-gradient(135deg, color-mix(in srgb, ${color} 9%, transparent), transparent 55%)` }}
    >
      <div className="flex flex-wrap items-start justify-between gap-3 px-5 pt-4">
        <div className="min-w-0">
          <h1 className="truncate text-lg font-semibold text-text-primary">{hostname}</h1>
          <p className="truncate text-xs text-text-tertiary">{subtitle}</p>
        </div>
        {right}
      </div>
      <div className="grid items-center gap-6 px-5 pb-5 pt-2 lg:grid-cols-[minmax(260px,340px)_1fr]">
        <HealthGauge score={health.score} level={health.level} label={health.label} />
        <div>
          <p className="mb-3 flex items-center gap-2 text-sm text-text-secondary">
            {worst ? <><Zap size={14} className="text-warning" />Slowest part right now: <b className="text-text-primary">{worst.label}</b> <span className="text-text-tertiary">({worst.value})</span></>
              : <><Gauge size={14} className="text-success" />Nothing is holding the server back.</>}
          </p>
          <ul className="grid gap-2 sm:gap-3" style={{ gridTemplateColumns: `repeat(${health.factors.length}, minmax(0, 1fr))` }}>
            {[...health.factors].sort((a, b) => b.weight - a.weight).map((f) => (
              <li key={f.id} title={f.hint} className="flex min-w-0 flex-col items-center rounded-xl border border-surface-border bg-surface-card/60 px-1.5 py-3 text-center">
                <Donut used={f.used} color={barColor(f.score)}>
                  <span className="text-xs font-semibold tabular-nums text-text-primary sm:text-sm">{Math.round(f.used)}<span className="text-[10px] font-medium text-text-tertiary">%</span></span>
                </Donut>
                <p className="mt-2 max-w-full truncate text-xs font-semibold text-text-primary">{f.label}</p>
                <p className="max-w-full truncate text-[11px] text-text-secondary">{f.value}</p>
              </li>
            ))}
          </ul>
          <p className="mt-3 text-[11px] text-text-tertiary">Score blends CPU, load per core, memory &amp; swap, disk, server errors and stopped services. Ring = how much is in use, color = how healthy that is. Hover a ring for how it is read.</p>
        </div>
      </div>
    </section>
  )
}

export default HealthCard
