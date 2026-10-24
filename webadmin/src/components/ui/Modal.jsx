import { createPortal } from 'react-dom'
import { X } from 'lucide-react'

function Modal({ open, onClose, title, children, size = 'md' }) {
  if (!open) return null
  const sizes = { sm: 'max-w-sm', md: 'max-w-lg', lg: 'max-w-2xl', xl: 'max-w-4xl' }

  // Rendered via portal into document.body so the fixed-position overlay
  // always covers the true viewport, regardless of any ancestor establishing
  // its own stacking/containing context (transform, filter, etc.).
  return createPortal(
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/25 p-4 backdrop-blur-[2px]"
      onClick={onClose}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className={`w-full ${sizes[size]} max-h-[90vh] overflow-y-auto rounded-xl bg-surface-card shadow-md`}
      >
        <div className="flex items-center justify-between border-b border-surface-border px-[26px] py-[18px]">
          <h3 className="text-base font-semibold text-text-primary">{title}</h3>
          <button
            onClick={onClose}
            className="rounded-md p-1 text-text-secondary hover:bg-surface-hover"
          >
            <X size={18} />
          </button>
        </div>
        <div className="px-[26px] py-6">{children}</div>
      </div>
    </div>,
    document.body,
  )
}

export default Modal
