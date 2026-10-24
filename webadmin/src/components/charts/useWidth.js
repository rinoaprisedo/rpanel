import { useEffect, useRef, useState } from 'react'

// Tracks the rendered width of an element so SVG charts can draw at 1:1 pixels.
export default function useWidth(initial = 600) {
  const ref = useRef(null)
  const [width, setWidth] = useState(initial)
  useEffect(() => {
    const el = ref.current
    if (!el) return undefined
    const ro = new ResizeObserver(([e]) => setWidth(Math.max(200, Math.round(e.contentRect.width))))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  return [ref, width]
}
