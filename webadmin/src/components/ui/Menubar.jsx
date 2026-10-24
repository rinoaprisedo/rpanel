import { createContext, useContext, useState, useRef, useEffect, useLayoutEffect } from 'react'
import { createPortal } from 'react-dom'
import { cn } from '../../utils/cn'

// shadcn-style menubar: a single grouped bar of triggers (actions + dropdown
// menus) instead of separate free-standing buttons. Only one menu is open at a
// time, and hovering another trigger switches to it once a menu is open.
const MenubarContext = createContext(null)

const triggerCls =
  'inline-flex items-center gap-1.5 whitespace-nowrap rounded-md px-2 py-1.5 text-[13px] font-medium text-text-secondary transition-colors duration-100 hover:bg-surface-hover sm:px-3'

export function Menubar({ children, className }) {
  const [openId, setOpenId] = useState(null)
  const ref = useRef(null)

  useEffect(() => {
    if (openId === null) return
    const onClickOutside = (e) => {
      if (ref.current?.contains(e.target)) return
      if (e.target.closest?.('[data-menubar-portal]')) return
      setOpenId(null)
    }
    document.addEventListener('mousedown', onClickOutside)
    return () => document.removeEventListener('mousedown', onClickOutside)
  }, [openId])

  return (
    <MenubarContext.Provider value={{ openId, setOpenId }}>
      <div
        ref={ref}
        className={cn(
          'flex min-w-0 items-center gap-0.5 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden',
          className,
        )}
      >
        {children}
      </div>
    </MenubarContext.Provider>
  )
}

// Static label inside the bar, e.g. the page title.
export function MenubarLabel({ children }) {
  return <span className="whitespace-nowrap px-2 text-[17px] font-bold text-text-primary sm:px-3">{children}</span>
}

// Vertical divider between groups of items.
export function MenubarSeparator() {
  return <span className="mx-0.5 h-5 w-px bg-surface-border sm:mx-1" />
}

// Plain action trigger (no dropdown), e.g. "Add New".
export function MenubarAction({ label, icon: Icon, onClick }) {
  const { setOpenId } = useContext(MenubarContext)
  return (
    <button
      onClick={() => {
        setOpenId(null)
        onClick?.()
      }}
      className={cn(triggerCls, 'text-text-primary')}
    >
      {Icon && <Icon size={15} />}
      {label}
    </button>
  )
}

// On/off switch styled as a trigger, for page-level feature toggles.
export function MenubarToggle({ label, icon: Icon, checked, onChange, disabled, title }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      disabled={disabled}
      title={title}
      onClick={() => onChange(!checked)}
      className={cn(triggerCls, 'text-text-primary disabled:cursor-not-allowed disabled:opacity-50')}
    >
      {Icon && <Icon size={15} className={checked ? 'text-primary' : undefined} />}
      {label}
      <span className={cn('relative h-4 w-7 rounded-full transition-colors', checked ? 'bg-primary' : 'bg-surface-border')}>
        <span className={cn('absolute top-0.5 h-3 w-3 rounded-full bg-white shadow transition-all', checked ? 'left-3.5' : 'left-0.5')} />
      </span>
    </button>
  )
}

// Trigger that opens a dropdown panel, rendered via a portal to <body> and
// positioned from the trigger's on-screen rect (recomputed on open/resize).
// A portal is required because the Menubar row uses `overflow-x-auto` for
// small screens, and CSS clips vertical overflow too once an axis is scrolled
// — an `absolute`-positioned panel nested inside it would get invisibly cut
// off instead of dropping below the bar.
export function MenubarMenu({ id, label, icon: Icon, active, align = 'left', children }) {
  const { openId, setOpenId } = useContext(MenubarContext)
  const open = openId === id
  const close = () => setOpenId(null)
  const triggerRef = useRef(null)
  const [pos, setPos] = useState(null)

  useLayoutEffect(() => {
    if (!open) return
    const place = () => {
      const rect = triggerRef.current.getBoundingClientRect()
      const mobile = window.innerWidth < 640 && align === 'sheet'
      if (mobile) {
        setPos({ top: 64, left: 12, right: 12 })
      } else if (align === 'right') {
        setPos({ top: rect.bottom + 8, right: window.innerWidth - rect.right })
      } else {
        setPos({ top: rect.bottom + 8, left: rect.left })
      }
    }
    place()
    window.addEventListener('resize', place)
    window.addEventListener('scroll', place, true)
    return () => {
      window.removeEventListener('resize', place)
      window.removeEventListener('scroll', place, true)
    }
  }, [open, align])

  return (
    <div className="relative">
      <button
        ref={triggerRef}
        onClick={() => setOpenId(open ? null : id)}
        onMouseEnter={() => setOpenId((prev) => (prev !== null ? id : prev))}
        className={cn(triggerCls, open && 'bg-surface-hover', active && 'text-primary')}
      >
        {Icon && <Icon size={15} />}
        {label}
        {active && <span className="h-1.5 w-1.5 rounded-full bg-primary" />}
      </button>
      {open &&
        pos &&
        createPortal(
          <div
            data-menubar-portal
            className="fixed z-50 min-w-[12rem] rounded-md border border-surface-border bg-surface-card p-1 shadow-md"
            style={{ top: pos.top, left: pos.left, right: pos.right }}
          >
            {typeof children === 'function' ? children(close) : children}
          </div>,
          document.body,
        )}
    </div>
  )
}

// A row inside a MenubarMenu dropdown (closes the menu on click).
export function MenubarItem({ label, icon: Icon, onClick }) {
  const { setOpenId } = useContext(MenubarContext)
  return (
    <button
      onClick={() => {
        setOpenId(null)
        onClick?.()
      }}
      className="flex w-full items-center gap-2 rounded-sm px-3 py-2 text-left text-[13px] text-text-primary hover:bg-surface-hover"
    >
      {Icon && <Icon size={14} />}
      {label}
    </button>
  )
}
