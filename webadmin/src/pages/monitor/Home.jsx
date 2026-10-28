import { Navigate } from 'react-router-dom'
import usePermission from '../../hooks/usePermission'

// "/" sends people to the first monitoring page their role may open.
function Home() {
  const { canView } = usePermission()
  if (canView('monitor-server')) return <Navigate to="/monitor-server" replace />
  if (canView('monitor-traffic')) return <Navigate to="/monitor-traffic" replace />
  if (canView('websites')) return <Navigate to="/websites" replace />
  return <Navigate to="/403" replace />
}

export default Home
