import { useEffect } from 'react'
import { Outlet } from 'react-router-dom'
import useSettingsStore from '../../store/settingsStore'
import BackendOffline from '../BackendOffline'
import loginBackground from '../../assets/background-login.webp'

function AuthLayout() {
  const appName = useSettingsStore((s) => s.appName)
  const logo = useSettingsStore((s) => s.logo)
  const backendOnline = useSettingsStore((s) => s.backendOnline)
  const fetchSettings = useSettingsStore((s) => s.fetchSettings)

  useEffect(() => {
    if (backendOnline) return
    const interval = setInterval(fetchSettings, 5000)
    return () => clearInterval(interval)
  }, [backendOnline, fetchSettings])

  if (!backendOnline) return <BackendOffline />

  return (
    <div className="flex min-h-screen">
      <div
        className="hidden bg-cover bg-center md:block md:w-1/2"
        style={{ backgroundImage: `url(${loginBackground})` }}
      />
      <div
        className="flex w-full items-center justify-center px-4 md:w-1/2"
        style={{ backgroundColor: 'rgb(162 228 228 / 4%)' }}
      >
        <div className="w-full max-w-sm">
          <div className="mb-6 flex flex-col items-center gap-2">
            {logo ? (
              <img src={logo} alt={appName} className="h-12 w-12 object-contain" />
            ) : (
              <div className="flex h-12 w-12 items-center justify-center rounded-lg bg-primary text-lg font-bold text-white">
                {appName.charAt(0)}
              </div>
            )}
            <h1 className="text-[22px] font-semibold tracking-[-.02em] text-text-primary">{appName}</h1>
          </div>
          <div className="rounded-xl border border-surface-border bg-surface-card p-[44px_48px] shadow-md">
            <Outlet />
          </div>
        </div>
      </div>
    </div>
  )
}

export default AuthLayout
