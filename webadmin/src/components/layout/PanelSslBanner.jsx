import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { ShieldAlert } from 'lucide-react'
import { getPanelSSL } from '../../api/panelSsl'

// Shown on panels that still run on an IP address with a self-signed certificate (installed before 1.3.3).
function PanelSslBanner() {
  const { data } = useQuery({ queryKey: ['panel-ssl'], queryFn: getPanelSSL, retry: false, staleTime: 5 * 60 * 1000 })
  if (!data?.data?.needs_domain) return null
  return (
    <div className="flex items-center gap-3 border-b border-surface-border bg-warning-bg px-4 py-2 text-sm text-warning md:px-[28px]">
      <ShieldAlert size={16} className="shrink-0" />
      <span className="min-w-0 flex-1">This panel has no domain: it is served over a self-signed certificate.</span>
      <Link to="/settings?tab=panel-ssl" className="shrink-0 font-medium underline">
        Set a domain
      </Link>
    </div>
  )
}

export default PanelSslBanner
