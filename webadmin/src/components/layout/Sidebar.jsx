import { NavLink } from 'react-router-dom'
import logoLandscape from '../../assets/logo-landscape-rpanel.webp'
import { useQuery } from '@tanstack/react-query'
import * as Icons from 'lucide-react'
import { LayoutGrid, X, FileCode2 } from 'lucide-react'
import { getMenus } from '../../api/menus'
import useSidebarStore from '../../store/sidebarStore'
import useSettingsStore from '../../store/settingsStore'
import useAuthStore from '../../store/authStore'
import usePermission from '../../hooks/usePermission'

function iconFor(name) {
  return Icons[name] || LayoutGrid
}

const apiBase = import.meta.env.VITE_API_URL || 'http://localhost:8800/api/v1'
const swaggerURL = apiBase.replace(/\/api\/v1\/?$/, '') + '/swagger/index.html'

function Sidebar() {
  const isCollapsed = useSidebarStore((s) => s.isCollapsed)
  const toggle = useSidebarStore((s) => s.toggle)
  const close = useSidebarStore((s) => s.close)
  const appName = useSettingsStore((s) => s.appName)
  const logo = useSettingsStore((s) => s.logo)
  const isSuperadmin = useAuthStore((s) => !!s.user?.is_superadmin)
  const { canView } = usePermission()

  const { data } = useQuery({
    queryKey: ['menus', 'tree'],
    queryFn: () => getMenus({ is_active: true }),
    staleTime: 5 * 60 * 1000,
  })

  // /menus returns sections with a nested `menus` array already grouped server-side
  const sections = (() => {
    const raw = data?.data
    if (!raw) return []
    if (Array.isArray(raw)) return raw
    return raw.sections || []
  })()

  return (
    <>
      <aside
        className={`fixed inset-y-0 left-0 z-40 flex w-[240px] flex-col border-r border-surface-border bg-sidebar-bg transition-transform duration-200 ${
          isCollapsed ? '-translate-x-full lg:translate-x-0' : 'translate-x-0'
        }`}
      >
        <div className="relative flex h-[60px] items-center justify-center border-b border-surface-border px-4">
          <img src={logo || logoLandscape} alt={appName} className="h-11 max-w-full object-contain" />
          <button onClick={toggle} className="absolute right-4 text-text-secondary lg:hidden">
            <X size={20} />
          </button>
        </div>
        <nav className="flex-1 overflow-y-auto px-3 py-3">
          {sections.map((section) => {
            const visibleMenus = (section.menus || []).filter((menu) => canView(menu.path) || canView(menu.name))
            const showSwagger = section.name === 'System' && isSuperadmin
            if (visibleMenus.length === 0 && !showSwagger) return null

            return (
              <div key={section.id || section.uuid || section.name} className="mb-4">
                <div className="mb-1 flex items-center gap-2 px-2 py-2 text-[10px] font-semibold uppercase tracking-[.08em] text-text-tertiary">
                  {section.name}
                </div>
                {visibleMenus.map((menu) => {
                  const Icon = iconFor(menu.icon)
                  return (
                    <NavLink
                      key={menu.id || menu.uuid}
                      to={menu.path}
                      onClick={close}
                      className={({ isActive }) =>
                        `mb-1 flex h-[38px] items-center gap-3 rounded-lg px-[10px] text-[13px] transition-colors duration-100 ${
                          isActive
                            ? 'bg-sidebar-active font-medium text-white'
                            : 'text-sidebar-text hover:bg-surface-hover hover:text-text-primary'
                        }`
                      }
                    >
                      <Icon size={16} strokeWidth={1.7} />
                      {menu.name}
                    </NavLink>
                  )
                })}
                {showSwagger && (
                  <a
                    href={swaggerURL}
                    target="_blank"
                    rel="noopener noreferrer"
                    onClick={close}
                    className="mb-1 flex h-[38px] items-center gap-3 rounded-lg px-[10px] text-[13px] text-sidebar-text transition-colors duration-100 hover:bg-surface-hover hover:text-text-primary"
                  >
                    <FileCode2 size={16} strokeWidth={1.7} />
                    Swagger
                  </a>
                )}
              </div>
            )
          })}
        </nav>
        <div className="border-t border-surface-border px-4 py-3 text-center text-[11px] text-text-tertiary">
          Develop by{' '}
          <a href="https://rinoaprisedo.com" target="_blank" rel="noopener noreferrer" className="font-medium hover:text-text-primary hover:underline">
            rinoaprisedo.com
          </a>
        </div>
      </aside>
      {!isCollapsed && (
        <div className="fixed inset-0 z-30 bg-black/40 lg:hidden" onClick={toggle} />
      )}
    </>
  )
}

export default Sidebar
