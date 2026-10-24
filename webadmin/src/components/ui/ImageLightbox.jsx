import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import { ChevronLeft, ChevronRight, ExternalLink, ImageOff, X } from 'lucide-react'

// Full-screen photo viewer. images = [{ src, label }]; arrows/keyboard step
// through them. Portal to body like Modal, and one level above it so it can
// open from inside a modal.
function ImageLightbox({ images, index = 0, onClose }) {
  const [current, setCurrent] = useState(index)
  const [broken, setBroken] = useState({})
  const count = images.length
  const img = images[current]

  useEffect(() => {
    const onKey = (e) => {
      if (e.key === 'Escape') onClose()
      if (e.key === 'ArrowLeft') setCurrent((i) => (i - 1 + count) % count)
      if (e.key === 'ArrowRight') setCurrent((i) => (i + 1) % count)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [count, onClose])

  if (!img) return null
  const step = (d) => (e) => {
    e.stopPropagation()
    setCurrent((i) => (i + d + count) % count)
  }
  const navBtn = 'absolute top-1/2 -translate-y-1/2 rounded-full bg-white/10 p-2.5 text-white hover:bg-white/20'

  return createPortal(
    // React events bubble through portals to the opener's ancestors (e.g. a
    // clickable table row), so a click here must stop at the overlay.
    <div
      className="fixed inset-0 z-[60] flex flex-col bg-black/85 backdrop-blur-sm"
      onClick={(e) => {
        e.stopPropagation()
        onClose()
      }}
    >
      <div className="flex items-center justify-between px-5 py-4 text-white" onClick={(e) => e.stopPropagation()}>
        <div className="text-sm font-medium">
          {img.label}
          {count > 1 && <span className="ml-2 text-white/60">{current + 1} / {count}</span>}
        </div>
        <div className="flex items-center gap-1">
          <a href={img.src} target="_blank" rel="noreferrer" className="rounded-md p-2 hover:bg-white/10" title="Buka di tab baru">
            <ExternalLink size={18} />
          </a>
          <button onClick={onClose} className="rounded-md p-2 hover:bg-white/10" title="Tutup">
            <X size={20} />
          </button>
        </div>
      </div>
      <div className="relative flex min-h-0 flex-1 items-center justify-center px-16 pb-8">
        {broken[current] ? (
          <div className="flex flex-col items-center gap-2 text-white/70" onClick={(e) => e.stopPropagation()}>
            <ImageOff size={32} />
            <span className="text-sm">Foto tidak dapat dimuat</span>
          </div>
        ) : (
          <img
            key={img.src}
            src={img.src}
            alt={img.label}
            onClick={(e) => e.stopPropagation()}
            onError={() => setBroken((b) => ({ ...b, [current]: true }))}
            className="max-h-full max-w-full rounded-md object-contain shadow-2xl"
          />
        )}
        {count > 1 && (
          <>
            <button onClick={step(-1)} className={`${navBtn} left-4`} title="Sebelumnya">
              <ChevronLeft size={22} />
            </button>
            <button onClick={step(1)} className={`${navBtn} right-4`} title="Berikutnya">
              <ChevronRight size={22} />
            </button>
          </>
        )}
      </div>
    </div>,
    document.body,
  )
}

export default ImageLightbox
