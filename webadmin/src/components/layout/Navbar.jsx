import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Menu, ChevronDown, LogOut, User, ShieldCheck, Sun, Moon } from 'lucide-react'
import toast from 'react-hot-toast'
import useSidebarStore from '../../store/sidebarStore'
import useAuthStore from '../../store/authStore'
import usePageActionsStore from '../../store/pageActionsStore'
import useThemeStore from '../../store/themeStore'
import { logout as logoutApi } from '../../api/auth'
import UpdateNotice from './UpdateNotice'

function Navbar() {
  const toggleSidebar = useSidebarStore((s) => s.toggle)
  const [menuOpen, setMenuOpen] = useState(false)
  const user = useAuthStore((s) => s.user)
  const logout = useAuthStore((s) => s.logout)
  const pageActions = usePageActionsStore((s) => s.actions)
  const navigate = useNavigate()
  const theme = useThemeStore((s) => s.theme)
  const toggleTheme = useThemeStore((s) => s.toggle)

  const handleLogout = async () => {
    try {
      await logoutApi()
    } catch {
      // ignore network failure on logout
    }
    logout()
    toast.success('Logged out')
    navigate('/login')
  }

  return (
    <header className="sticky top-0 z-20 flex h-[60px] items-center justify-between border-b border-surface-border bg-surface-card px-4 md:px-[28px]">
      <div className="flex min-w-0 flex-1 items-center gap-3">
        <button onClick={toggleSidebar} className="shrink-0 rounded-md p-2 text-text-secondary transition-colors duration-100 hover:bg-surface-hover lg:hidden">
          <Menu size={20} />
        </button>
        {pageActions}
      </div>
      <div className="flex shrink-0 items-center gap-3">
        <UpdateNotice />
        <button
          onClick={toggleTheme}
          title={theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
          aria-label="Toggle dark mode"
          className="rounded-md p-2 text-text-secondary transition-colors duration-100 hover:bg-surface-hover"
        >
          {theme === 'dark' ? <Sun size={17} /> : <Moon size={17} />}
        </button>
        <div className="relative">
          <button
            onClick={() => setMenuOpen((v) => !v)}
            className="flex items-center gap-2 rounded-md px-2 py-1.5 transition-colors duration-100 hover:bg-surface-hover"
          >
            <div className="flex h-[30px] w-[30px] items-center justify-center rounded-full bg-primary text-xs font-semibold text-white">
              {(user?.name || 'U').charAt(0).toUpperCase()}
            </div>
            <span className="hidden text-sm font-medium sm:block">{user?.name || 'User'}</span>
            <ChevronDown size={14} />
          </button>
          {menuOpen && (
            <div className="absolute right-0 mt-2 w-48 rounded-md border border-surface-border bg-surface-card py-1 shadow-md">
              <div className="flex items-center gap-2 px-3 py-2 text-sm text-text-secondary">
                <User size={14} /> {user?.email}
              </div>
              <button
                onClick={() => { setMenuOpen(false); navigate('/security') }}
                className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm text-text-primary hover:bg-surface-hover"
              >
                <ShieldCheck size={14} /> Security
              </button>
              <button
                onClick={handleLogout}
                className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm text-danger hover:bg-surface-hover"
              >
                <LogOut size={14} /> Logout
              </button>
            </div>
          )}
        </div>
      </div>
    </header>
  )
}

export default Navbar
