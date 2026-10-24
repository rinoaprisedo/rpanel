import { useEffect, useState } from 'react'

const R = 30
const C = 2 * Math.PI * R

// Ring that fills to `used` percent; color carries the verdict. Starts empty and
// animates to its value on mount and on every change.
function Donut({ used, color, children }) {
  const [shown, setShown] = useState(0)
  useEffect(() => {
    const id = requestAnimationFrame(() => setShown(Math.max(0, Math.min(100, used || 0))))
    return () => cancelAnimationFrame(id)
  }, [used])
  const min = shown > 0 ? Math.max(shown, 1.5) : 0 // keep a sliver visible for tiny values
  return (
    <div className="relative aspect-square w-full max-w-[84px]">
      <svg viewBox="0 0 80 80" className="h-full w-full -rotate-90">
        <circle cx="40" cy="40" r={R} fill="none" stroke={color} strokeOpacity=".18" strokeWidth="8" />
        <circle cx="40" cy="40" r={R} fill="none" stroke={color} strokeWidth="8" strokeLinecap="round"
          strokeDasharray={C} strokeDashoffset={C * (1 - min / 100)}
          style={{ transition: 'stroke-dashoffset 1s cubic-bezier(.4,0,.2,1)' }} />
      </svg>
      <div className="absolute inset-0 flex items-center justify-center">{children}</div>
    </div>
  )
}

export default Donut
