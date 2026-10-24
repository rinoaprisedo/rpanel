// Minimal dependency-free line chart. values: number[]; max: fixed y-max (e.g. 100 for %).
function Sparkline({ series = [], max, height = 60 }) {
  const w = 300
  const all = series.flatMap((s) => s.values)
  const top = max || Math.max(1, ...all) * 1.1
  const path = (values) =>
    values
      .map((v, i) => {
        const x = values.length > 1 ? (i / (values.length - 1)) * w : 0
        const y = height - Math.min(v / top, 1) * height
        return `${i ? 'L' : 'M'}${x.toFixed(1)},${y.toFixed(1)}`
      })
      .join(' ')
  return (
    <svg viewBox={`0 0 ${w} ${height}`} preserveAspectRatio="none" className="h-16 w-full">
      {series.map((s) => (
        <path key={s.label} d={path(s.values)} fill="none" stroke={s.color} strokeWidth="1.5" vectorEffect="non-scaling-stroke" />
      ))}
    </svg>
  )
}

export default Sparkline
