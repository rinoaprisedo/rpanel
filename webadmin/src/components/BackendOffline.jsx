import { Unplug } from 'lucide-react'

function BackendOffline() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-3 bg-surface-bg text-center">
      <Unplug size={48} className="text-danger" />
      <h1 className="text-3xl font-bold text-text-primary">Backend not connected</h1>
      <p className="text-text-secondary">Can't reach the server. Retrying automatically...</p>
    </div>
  )
}

export default BackendOffline
