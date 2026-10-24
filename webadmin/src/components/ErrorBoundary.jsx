import { Component } from 'react'
import { AlertTriangle } from 'lucide-react'

// Without this, any uncaught render error (e.g. a page assuming the wrong
// API response shape) unmounts the whole React tree and leaves a blank
// white screen with no clue what happened.
class ErrorBoundary extends Component {
  state = { error: null }

  static getDerivedStateFromError(error) {
    return { error }
  }

  componentDidCatch(error, info) {
    console.error('Uncaught render error:', error, info)
  }

  render() {
    if (this.state.error) {
      return (
        <div className="flex min-h-screen flex-col items-center justify-center gap-3 bg-surface-bg px-4 text-center">
          <AlertTriangle size={40} className="text-danger" />
          <p className="text-lg font-semibold text-text-primary">Something went wrong</p>
          <p className="max-w-md text-sm text-text-secondary">{this.state.error.message}</p>
          <button
            onClick={() => window.location.reload()}
            className="mt-2 h-[38px] rounded-md bg-primary px-4 text-[13px] font-medium text-white hover:opacity-[.88]"
          >
            Reload page
          </button>
        </div>
      )
    }
    return this.props.children
  }
}

export default ErrorBoundary
