import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { format } from 'date-fns'
import { getVersion } from '../../api/system'
import useApiTimingStore from '../../store/apiTimingStore'

function measureLoadTime() {
  const nav = performance.getEntriesByType('navigation')[0]
  return Math.round(nav ? nav.loadEventEnd - nav.startTime : performance.now())
}

function Footer() {
  const [now, setNow] = useState(new Date())
  const [loadTime, setLoadTime] = useState(document.readyState === 'complete' ? measureLoadTime() : null)
  const { data } = useQuery({ queryKey: ['version'], queryFn: getVersion, staleTime: Infinity, retry: false })
  const apiLoadTime = useApiTimingStore((s) => s.lastDuration)

  useEffect(() => {
    const timer = setInterval(() => setNow(new Date()), 1000)
    return () => clearInterval(timer)
  }, [])

  useEffect(() => {
    if (loadTime !== null) return
    const onLoad = () => setLoadTime(measureLoadTime())
    window.addEventListener('load', onLoad, { once: true })
    return () => window.removeEventListener('load', onLoad)
  }, [loadTime])

  return (
    <footer className="fixed inset-x-0 bottom-0 z-20 flex flex-col items-center justify-center gap-y-0.5 border-t border-surface-border bg-surface-card px-4 py-1.5 text-[11px] text-text-tertiary sm:flex-row sm:items-center sm:justify-between sm:gap-x-4 sm:overflow-x-auto sm:whitespace-nowrap sm:py-3 lg:left-[240px]">
      <div className="order-2 flex flex-wrap items-center justify-center gap-x-4 gap-y-0.5 sm:contents">
        <div className="flex items-center gap-x-4 sm:order-1 sm:flex-1 sm:justify-start">
          <span>BE v{data?.data?.version || '-'}</span>
          <span>FE v{__APP_VERSION__}</span>
        </div>
        <div className="flex items-center gap-x-4 sm:order-2 sm:flex-1 sm:justify-center">
          <span>Load Page {loadTime ?? '-'}ms</span>
          <span>Load Api {apiLoadTime ?? '-'}ms</span>
        </div>
      </div>
      <span className="order-1 sm:order-3 sm:flex-1 sm:text-right">{format(now, 'PP HH:mm:ss')}</span>
    </footer>
  )
}

export default Footer
