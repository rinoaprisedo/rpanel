import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { ChevronDown } from 'lucide-react'

const link = 'whitespace-nowrap text-[13px] font-medium text-primary transition-opacity hover:opacity-70'

// The "More" list is portalled with fixed coordinates: the file table scrolls
// horizontally, and that would clip a normally positioned dropdown on the last rows.
function MoreMenu({ items, anchor, onClose }) {
  const ref = useRef(null)
  const [pos, setPos] = useState({ top: 0, left: 0 })

  useLayoutEffect(() => {
    const r = anchor.getBoundingClientRect()
    const h = ref.current?.offsetHeight || 0
    const top = r.bottom + h + 8 > window.innerHeight ? Math.max(8, r.top - h - 4) : r.bottom + 4
    setPos({ top, left: Math.max(8, r.right - 176) })
  }, [anchor])

  useEffect(() => {
    const close = (e) => {
      if (e.type === 'keydown' ? e.key === 'Escape' : !ref.current?.contains(e.target) && !anchor.contains(e.target)) onClose()
    }
    document.addEventListener('mousedown', close)
    document.addEventListener('keydown', close)
    window.addEventListener('scroll', onClose, true)
    return () => {
      document.removeEventListener('mousedown', close)
      document.removeEventListener('keydown', close)
      window.removeEventListener('scroll', onClose, true)
    }
  }, [onClose, anchor])

  return createPortal(
    <div ref={ref} style={{ top: pos.top, left: pos.left }} className="fixed z-[70] w-44 rounded-md border border-surface-border bg-surface-card py-1 shadow-md">
      {items.map((it) => (
        <button key={it.label} className="block w-full px-3 py-1.5 text-left text-[13px] text-text-primary hover:bg-surface-hover" onClick={() => { onClose(); it.run() }}>
          {it.label}
        </button>
      ))}
    </div>,
    document.body,
  )
}

// Inline action links that appear while the row is hovered or focused.
function RowActions({ actions, more, open, onToggleMore }) {
  const moreBtn = useRef(null)
  return (
    <div className={`absolute right-3 top-1/2 z-10 -translate-y-1/2 items-center gap-3.5 rounded-md border border-surface-border bg-surface-card px-3 py-1.5 shadow-md group-focus-within:flex group-hover:flex ${open ? 'flex' : 'hidden'}`}>
      {actions.map((a) => (
        <button key={a.label} className={`${link} ${a.danger ? '!text-danger' : ''}`} onClick={a.run}>{a.label}</button>
      ))}
      {more.length > 0 && (
        <>
          <button ref={moreBtn} className={`${link} inline-flex items-center gap-0.5`} onClick={onToggleMore}>More<ChevronDown size={13} /></button>
          {open && moreBtn.current && <MoreMenu items={more} anchor={moreBtn.current} onClose={onToggleMore} />}
        </>
      )}
    </div>
  )
}

export default RowActions
