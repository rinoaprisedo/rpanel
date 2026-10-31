import { useEffect } from 'react'
import { Outlet } from 'react-router-dom'
import useSettingsStore from '../../store/settingsStore'
import logoLandscape from '../../assets/logo-landscape-rpanel.webp'
import NetworkBackground from './NetworkBackground'
import BackendOffline from '../BackendOffline'

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
    <div className="auth-scene dark">
      <div className="auth-glow left-1/2 top-1/3 h-[30rem] w-[30rem] -translate-x-1/2 -translate-y-1/2 bg-sky-500/25" />
      <div className="auth-glow -bottom-40 -right-20 h-[26rem] w-[26rem] bg-primary/25" style={{ animationDelay: '-6s' }} />
      <NetworkBackground />

      <div className="relative z-10 flex min-h-screen items-center justify-center px-4 py-24">
        <div className="w-full max-w-sm">
          <div className="auth-card relative rounded-2xl p-8 sm:p-10">
            <div className="mb-6 flex flex-col items-center gap-4">
              <img src={logo || logoLandscape} alt={appName} className="h-14 w-auto max-w-full object-contain" />
              <div className="flex items-center gap-2 font-mono text-[11px] uppercase tracking-[.2em] text-emerald-400">
                <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-emerald-400" /> secure access
              </div>
            </div>
            <Outlet />
          </div>
          <p className="mt-5 text-center font-mono text-[11px] text-slate-600">Authorized personnel only</p>
        </div>
      </div>
    </div>
  )
}

export default AuthLayout
