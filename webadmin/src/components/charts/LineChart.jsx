import { useMemo, useState } from 'react'
import useWidth from './useWidth'
import Tooltip from './Tooltip'
import { niceTicks, axisTime, tooltipTime, labelIndices } from './scale'

const H = 170
const BASE = { l: 44, r: 10, t: 10, b: 24 }

// points: [{ t, ...values }]; series: [{ key, label, color }]; fmt formats a value for ticks and tooltip.
export default function LineChart({ points, series, fmt, yMax, area, refLine, ariaLabel }) {
  const [ref, width] = useWidth()
  const [hover, setHover] = useState(null)
  const n = points.length

  const { ticks, top } = useMemo(() => {
    if (yMax) return { ticks: [0, yMax / 2, yMax], top: yMax }
    const max = Math.max(0, ...points.flatMap((p) => series.map((s) => p[s.key] || 0)), refLine?.value || 0)
    return niceTicks(max * 1.05)
  }, [points, series, yMax, refLine])

  // Room for the widest tick label ("892.5 KB/s" needs more than "50%").
  const M = { ...BASE, l: Math.max(BASE.l, Math.max(...ticks.map((t) => fmt(t).length)) * 6.4 + 16) }
  const plotW = width - M.l - M.r
  const plotH = H - M.t - M.b

  const t0 = n ? points[0].t : 0
  const span = n > 1 ? points[n - 1].t - t0 : 1
  const X = (t) => M.l + ((t - t0) / span) * plotW
  const Y = (v) => M.t + plotH - (Math.min(v, top) / top) * plotH
  const path = (k) => points.map((p, i) => `${i ? 'L' : 'M'}${X(p.t).toFixed(1)},${Y(p[k] || 0).toFixed(1)}`).join('')

  const nearest = (clientX, left) => {
    const t = t0 + ((clientX - left - M.l) / plotW) * span
    let best = 0
    for (let i = 1; i < n; i++) if (Math.abs(points[i].t - t) < Math.abs(points[best].t - t)) best = i
    return best
  }
  const move = (e) => {
    const r = e.currentTarget.getBoundingClientRect()
    setHover({ i: nearest(e.clientX, r.left), x: e.clientX - r.left, y: e.clientY - r.top })
  }
  const key = (e) => {
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
    e.preventDefault()
    const i = Math.min(n - 1, Math.max(0, (hover?.i ?? n) + (e.key === 'ArrowRight' ? 1 : -1)))
    setHover({ i, x: X(points[i].t), y: 24 })
  }

  if (n < 2) {
    return (
      <div ref={ref} className="flex items-center justify-center text-center text-xs text-text-tertiary" style={{ height: H }}>
        Collecting data… history builds up while the panel is running.
      </div>
    )
  }
  const hp = hover ? points[hover.i] : null

  return (
    <div ref={ref} className="relative" onPointerLeave={() => setHover(null)}>
      <svg width={width} height={H} role="img" aria-label={ariaLabel} tabIndex={0} onPointerMove={move} onKeyDown={key} onBlur={() => setHover(null)} className="block touch-pan-y outline-none focus-visible:ring-1 focus-visible:ring-primary">
        {ticks.map((t) => (
          <g key={t}>
            <line x1={M.l} x2={width - M.r} y1={Y(t)} y2={Y(t)} stroke="rgb(var(--c-border))" strokeWidth="1" />
            <text x={M.l - 8} y={Y(t) + 3.5} textAnchor="end" fontSize="11" fill="rgb(var(--c-text-2))">{fmt(t)}</text>
          </g>
        ))}
        {refLine && refLine.value <= top && (
          <g>
            <line x1={M.l} x2={width - M.r} y1={Y(refLine.value)} y2={Y(refLine.value)} stroke="rgb(var(--c-text-3))" strokeWidth="1" />
            <text x={width - M.r} y={Y(refLine.value) - 4} textAnchor="end" fontSize="10" fill="rgb(var(--c-text-2))">{refLine.label}</text>
          </g>
        )}
        {area && series.length === 1 && <path d={`${path(series[0].key)}L${X(points[n - 1].t)},${M.t + plotH}L${M.l},${M.t + plotH}Z`} fill={series[0].color} opacity="0.1" />}
        {series.map((s) => <path key={s.key} d={path(s.key)} fill="none" stroke={s.color} strokeWidth="2" strokeLinejoin="round" strokeLinecap="round" />)}
        <line x1={M.l} x2={width - M.r} y1={M.t + plotH} y2={M.t + plotH} stroke="rgb(var(--c-border-hover))" strokeWidth="1" />
        {labelIndices(n, Math.max(2, Math.floor(plotW / 78))).map((i) => (
          <text key={i} x={X(points[i].t)} y={H - 6} textAnchor={i === 0 ? 'start' : i === n - 1 ? 'end' : 'middle'} fontSize="11" fill="rgb(var(--c-text-2))">{axisTime(points[i].t, span)}</text>
        ))}
        {hp && <line x1={X(hp.t)} x2={X(hp.t)} y1={M.t} y2={M.t + plotH} stroke="rgb(var(--c-text-3))" strokeWidth="1" />}
        {/* end markers: 8px dots with a 2px surface ring, so they stay legible over the line */}
        {series.map((s) => {
          const p = hp || points[n - 1]
          return <circle key={s.key} cx={X(p.t)} cy={Y(p[s.key] || 0)} r="4" fill={s.color} stroke="rgb(var(--c-card))" strokeWidth="2" />
        })}
      </svg>
      {hp && (
        <Tooltip
          x={hover.x} y={hover.y} width={width}
          title={tooltipTime(hp.t, span)}
          rows={series.map((s) => ({ label: s.label, color: s.color, value: fmt(hp[s.key] || 0) }))}
        />
      )}
    </div>
  )
}
