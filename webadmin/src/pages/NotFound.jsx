import { Link } from 'react-router-dom'
import { FileQuestion } from 'lucide-react'

function NotFound() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-3 bg-surface-bg text-center">
      <FileQuestion size={48} className="text-text-secondary" />
      <h1 className="text-3xl font-bold text-text-primary">404 - Page not found</h1>
      <p className="text-text-secondary">The page you're looking for doesn't exist.</p>
      <Link to="/" className="text-primary hover:underline">
        Back to dashboard
      </Link>
    </div>
  )
}

export default NotFound
