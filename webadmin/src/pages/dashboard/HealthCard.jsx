import { Gauge, Zap } from 'lucide-react'
import HealthGauge, { levelColor, REVERSED_ZONES, zoneColor } from './HealthGauge'

// The one-glance answer to "is this server running well?", with the measurements behind it.
function HealthCard({ health, traffic, hostname, subtitle, right, children }) {
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
      <div className="grid items-center gap-6 px-5 pb-5 pt-2 lg:grid-cols-[auto_1fr]">
        <div className="flex flex-wrap items-start justify-center gap-4">
          {traffic && (
            <div className="w-[260px] max-w-full" title="Data coming in over the network right now, on a log scale. Not part of the server score.">
              <HealthGauge score={traffic.score} label={traffic.label} leftLabel="Quiet" rightLabel="Busy" valueText={traffic.value} zones={REVERSED_ZONES} color={zoneColor(REVERSED_ZONES, traffic.score)} ariaLabel={`Incoming traffic ${traffic.value}: ${traffic.label}`} />
              <p className="mt-0.5 text-center text-xs text-text-tertiary">Incoming traffic</p>
            </div>
          )}
          <div className="w-[260px] max-w-full">
            <HealthGauge score={health.score} level={health.level} label={health.label} />
            <p className="mt-0.5 text-center text-xs text-text-tertiary">Server health</p>
          </div>
        </div>
        <div>
          <p className="mb-3 flex items-center gap-2 text-sm text-text-secondary">
            {worst ? <><Zap size={14} className="text-warning" />Slowest part right now: <b className="text-text-primary">{worst.label}</b> <span className="text-text-tertiary">({worst.value})</span></>
              : <><Gauge size={14} className="text-success" />Nothing is holding the server back.</>}
          </p>
          {children}
          <p className="mt-3 text-[11px] text-text-tertiary">Score blends CPU, load per core, memory &amp; swap, disk and stopped services (server errors have their own gauge below).</p>
        </div>
      </div>
    </section>
  )
}

export default HealthCard
