import { Link } from 'react-router-dom'
import { ShieldAlert } from 'lucide-react'

function Forbidden() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-3 bg-surface-bg text-center">
      <ShieldAlert size={48} className="text-danger" />
      <h1 className="text-3xl font-bold text-text-primary">403 - Forbidden</h1>
      <p className="text-text-secondary">You don't have permission to access this page.</p>
      <Link to="/" className="text-primary hover:underline">
        Back to dashboard
      </Link>
    </div>
  )
}

export default Forbidden
