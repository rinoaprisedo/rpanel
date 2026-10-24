import { useMemo, useState } from 'react'
import useWidth from './useWidth'
import Tooltip from './Tooltip'
import { niceTicks, compact, axisTime, tooltipTime, labelIndices } from './scale'

const H = 210
const M = { l: 40, r: 6, t: 8, b: 24 }
const GAP = 2 // surface-colored gap between touching segments
const MIN_SEG = 3 // a real but tiny count (a handful of 5xx) must stay visible

// Rounds only the top corners: the data end is round, the baseline end is square.
function topRounded(x, y, w, h, r) {
  const rr = Math.min(r, h, w / 2)
  return `M${x},${y + h}V${y + rr}Q${x},${y} ${x + rr},${y}H${x + w - rr}Q${x + w},${y} ${x + w},${y + rr}V${y + h}Z`
}

// buckets: [{ t, ...counts }]; series (bottom → top): [{ key, label, color }].
export default function StackedColumns({ buckets, series, bucketSec, daily, ariaLabel }) {
  const [ref, width] = useWidth()
  const [hover, setHover] = useState(null)
  const plotW = width - M.l - M.r
  const plotH = H - M.t - M.b

  const { ticks, top } = useMemo(() => niceTicks(Math.max(0, ...buckets.map((b) => series.reduce((s, k) => s + (b[k.key] || 0), 0)))), [buckets, series])
  const n = buckets.length
  const band = n ? plotW / n : plotW
  const barW = Math.min(24, Math.max(3, band * 0.72))
  const span = n > 1 ? buckets[n - 1].t - buckets[0].t : 0
  const y = (v) => M.t + plotH - (v / top) * plotH

  const move = (e) => {
    const r = e.currentTarget.getBoundingClientRect()
    const i = Math.floor((e.clientX - r.left - M.l) / band)
    setHover(i >= 0 && i < n ? { i, x: e.clientX - r.left, y: e.clientY - r.top } : null)
  }
  const key = (e) => {
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
    e.preventDefault()
    const i = Math.min(n - 1, Math.max(0, (hover?.i ?? (e.key === 'ArrowRight' ? -1 : n)) + (e.key === 'ArrowRight' ? 1 : -1)))
    setHover({ i, x: M.l + band * (i + 0.5), y: 24 })
  }

  const hb = hover ? buckets[hover.i] : null
  const total = hb ? series.reduce((s, k) => s + (hb[k.key] || 0), 0) : 0

  return (
    <div ref={ref} className="relative" onPointerLeave={() => setHover(null)}>
      <svg
        width={width} height={H} role="img" aria-label={ariaLabel} tabIndex={0}
        onPointerMove={move} onKeyDown={key} onBlur={() => setHover(null)} className="block touch-pan-y outline-none focus-visible:ring-1 focus-visible:ring-primary"
      >
        {ticks.map((t) => (
          <g key={t}>
            <line x1={M.l} x2={width - M.r} y1={y(t)} y2={y(t)} stroke="rgb(var(--c-border))" strokeWidth="1" />
            <text x={M.l - 8} y={y(t) + 3.5} textAnchor="end" fontSize="11" fill="rgb(var(--c-text-2))">{compact(t)}</text>
          </g>
        ))}
        {hover && <rect x={M.l + band * hover.i} y={M.t} width={band} height={plotH} fill="rgb(var(--c-text))" opacity="0.06" />}
        {buckets.map((b, i) => {
          const cx = M.l + band * i + (band - barW) / 2
          let floor = M.t + plotH
          return (
            <g key={b.t}>
              {series.map((s, k) => {
                const v = b[s.key] || 0
                if (!v) return null
                const h = Math.max(MIN_SEG, (v / top) * plotH)
                const yy = floor - h
                const isTop = series.slice(k + 1).every((q) => !(b[q.key] || 0))
                const el = isTop
                  ? <path key={s.key} d={topRounded(cx, yy, barW, h, 4)} fill={s.color} />
                  : <rect key={s.key} x={cx} y={yy} width={barW} height={h} fill={s.color} />
                floor = yy - GAP
                return el
              })}
            </g>
          )
        })}
        <line x1={M.l} x2={width - M.r} y1={M.t + plotH} y2={M.t + plotH} stroke="rgb(var(--c-border-hover))" strokeWidth="1" />
        {labelIndices(n, Math.max(2, Math.floor(plotW / 78))).map((i) => (
          <text key={i} x={M.l + band * (i + 0.5)} y={H - 6} textAnchor="middle" fontSize="11" fill="rgb(var(--c-text-2))">{axisTime(buckets[i].t, span)}</text>
        ))}
      </svg>
      {hb && (
        <Tooltip
          x={hover.x} y={hover.y} width={width}
          title={tooltipTime(hb.t, span, daily)}
          rows={[...series].reverse().map((s) => ({ label: s.label, color: s.color, value: (hb[s.key] || 0).toLocaleString() }))}
          footer={`${total.toLocaleString()} requests · ${bucketSec >= 3600 ? `${bucketSec / 3600}h` : `${bucketSec / 60}m`} window`}
        />
      )}
    </div>
  )
}
