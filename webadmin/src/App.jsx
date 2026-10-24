import { useEffect, useState } from 'react'
import AppRouter from './router'
import useAuthStore from './store/authStore'
import useSettingsStore from './store/settingsStore'
import { me } from './api/auth'

function App() {
  const setUser = useAuthStore((s) => s.setUser)
  const fetchSettings = useSettingsStore((s) => s.fetchSettings)
  const appName = useSettingsStore((s) => s.appName)
  const favicon = useSettingsStore((s) => s.favicon)
  const [ready, setReady] = useState(false)

  useEffect(() => {
    document.title = appName
  }, [appName])

  useEffect(() => {
    if (!favicon) return
    const link = document.getElementById('favicon')
    if (link) link.href = favicon
  }, [favicon])

  useEffect(() => {
    Promise.allSettled([
      fetchSettings(),
      me()
        .then((res) => setUser(res.data))
        .catch(() => {}),
    ]).finally(() => setReady(true))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  if (!ready) return null

  return <AppRouter />
}

export default App
