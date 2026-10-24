import { Navigate, useLocation } from 'react-router-dom'
import useAuthStore from '../store/authStore'
import usePermission from '../hooks/usePermission'

// menuKey: the permission module key this route requires can_view for. Omit to just require auth.
function ProtectedRoute({ children, menuKey }) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  const { canView } = usePermission()
  const location = useLocation()

  if (!isAuthenticated) {
    return <Navigate to="/login" state={{ from: location }} replace />
  }

  if (menuKey && !canView(menuKey)) {
    return <Navigate to="/403" replace />
  }

  return children
}

export default ProtectedRoute
