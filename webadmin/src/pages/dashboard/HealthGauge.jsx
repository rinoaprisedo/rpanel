import { useId } from 'react'
import { cn } from '../../utils/cn'
import { SEV } from '../../components/charts/severity'

const CX = 100
const CY = 100
const R = 78
const pt = (t, r = R) => [CX - r * Math.cos(Math.PI * t), CY - r * Math.sin(Math.PI * t)]
const arc = (a, b) => {
  const [x0, y0] = pt(a)
  const [x1, y1] = pt(b)
  return `M${x0.toFixed(2)},${y0.toFixed(2)} A${R},${R} 0 0 1 ${x1.toFixed(2)},${y1.toFixed(2)}`
}

// Zones follow the verdict levels (struggling < 40 <= slow < 65 <= fair < 85 <= smooth).
const ZONES = [[0, 0.4, SEV.red], [0.4, 0.65, SEV.orange], [0.65, 0.85, SEV.yellow], [0.85, 1, SEV.green]]
// Same four colours the other way round, for things where more is worse (traffic): green at the left, red at the right.
export const REVERSED_ZONES = [[0, 0.4, SEV.green], [0.4, 0.65, SEV.yellow], [0.65, 0.85, SEV.orange], [0.85, 1, SEV.red]]
export const zoneColor = (zones, score) => (zones.find(([a, b]) => score / 100 >= a && score / 100 < b) || zones[zones.length - 1])[2]

const LEVEL_COLOR = { smooth: SEV.green, fair: SEV.yellow, slow: SEV.orange, struggle: SEV.red }

export const levelColor = (level) => LEVEL_COLOR[level] || LEVEL_COLOR.smooth

// Speedometer: the needle sweeps from "struggling" (left) to "smooth" (right).
function HealthGauge({ score, level, label, className, compact = false, leftLabel = 'Slow', rightLabel = 'Smooth', valueText, ariaLabel, zones = ZONES, color: colorOverride }) {
  const s = Math.max(0, Math.min(100, score ?? 0))
  const color = colorOverride || levelColor(level)
  const shadow = `needle-shadow-${useId().replace(/:/g, '')}`
  return (
    <div className={cn('flex flex-col items-center', className)} role="img" aria-label={ariaLabel || `Server health ${s} out of 100: ${label}`}>
      <svg viewBox="0 0 200 118" className={cn('w-full overflow-visible', compact ? 'max-w-[150px]' : 'max-w-[320px]')}>
        <defs>
          <filter id={shadow} x="-50%" y="-50%" width="200%" height="200%">
            <feDropShadow dx="0" dy="1" stdDeviation="1.5" floodOpacity=".3" />
          </filter>
        </defs>
        {zones.map(([a, b, c]) => (
          <path key={a} d={arc(a + 0.005, b - 0.005)} fill="none" stroke={c} strokeWidth="14" opacity=".22" />
        ))}
        {zones.map(([a, b, c]) => {
          const end = Math.min(b, s / 100)
          return end > a ? <path key={a} d={arc(a + 0.005, Math.max(a + 0.006, end - (end === b ? 0.005 : 0)))} fill="none" stroke={c} strokeWidth="14" /> : null
        })}
        {[0, 0.25, 0.5, 0.75, 1].map((t) => {
          const [x0, y0] = pt(t, R + 11)
          const [x1, y1] = pt(t, R + 16)
          return <line key={t} x1={x0} y1={y0} x2={x1} y2={y1} stroke="rgb(var(--c-text-3))" strokeWidth="1.5" strokeLinecap="round" />
        })}
        <g style={{ transform: `rotate(${-90 + (180 * s) / 100}deg)`, transformOrigin: `${CX}px ${CY}px`, transition: 'transform 1s cubic-bezier(.34,1.3,.5,1)' }} filter={`url(#${shadow})`}>
          <path d={`M${CX - 4},${CY} L${CX},${CY - 66} L${CX + 4},${CY} Z`} fill="rgb(var(--c-text))" />
        </g>
        <circle cx={CX} cy={CY} r="9" fill="rgb(var(--c-card))" stroke="rgb(var(--c-text))" strokeWidth="3" />
        <circle cx={CX} cy={CY} r="3" fill={color} />
        <text x={CX - R} y="116" textAnchor="middle" className="fill-text-tertiary text-[9px] font-medium">{leftLabel}</text>
        <text x={CX + R} y="116" textAnchor="middle" className="fill-text-tertiary text-[9px] font-medium">{rightLabel}</text>
      </svg>
      <div className="-mt-1 text-center">
        <div className={cn('flex items-baseline justify-center gap-1', !compact && 'h-[40px]')}>
          <span className={cn('font-semibold leading-none tabular-nums', compact ? 'text-xl' : valueText && valueText.length > 6 ? 'text-[28px]' : 'text-[40px]')} style={{ color }}>{valueText ?? s}</span>
          {valueText === undefined && <span className="text-sm text-text-tertiary">/ 100</span>}
        </div>
        <p className={cn('mt-1 font-semibold text-text-primary', compact ? 'text-xs' : 'text-sm')}>{label}</p>
      </div>
    </div>
  )
}

export default HealthGauge
