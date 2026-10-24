import { useState } from 'react'
import { ImageOff } from 'lucide-react'
import ImageLightbox from './ImageLightbox'
import { fileURL, privateFileURL } from '../../utils/url'
import { cn } from '../../utils/cn'

// Legacy-imported photos may still point at the old server's relative path
// (when LEGACY_UPLOAD_URL wasn't set during migrate-legacy), so a 404 here is
// expected rather than exceptional — fall back to a placeholder.
// Clicking opens a lightbox; pass gallery=[{ path, label }] (including this
// photo) to page through related photos, e.g. a member's face/full/KTP.
// thumb asks /files for a small server-made copy (private files only); the
// lightbox still shows the full photo.
function Thumb({ path, alt, className, wrapperClassName, iconSize = 14, emptyLabel, isPrivate = false, thumb = false, gallery }) {
  const [broken, setBroken] = useState(false)
  const [open, setOpen] = useState(false)
  const toURL = (p) => (isPrivate ? privateFileURL(p) : fileURL(p))
  const src = toURL(path)

  if (!src || broken) {
    return (
      <div
        title={path || 'No file'}
        className={cn('flex items-center justify-center rounded bg-surface-hover text-text-tertiary', className)}
      >
        <div className="flex flex-col items-center gap-2">
          <ImageOff size={iconSize} />
          {emptyLabel && <span className="text-xs">{path ? 'Foto tidak dapat dimuat' : emptyLabel}</span>}
        </div>
      </div>
    )
  }

  const images = (gallery || [{ path, label: alt }])
    .map((g) => ({ src: toURL(g.path), label: g.label }))
    .filter((g) => g.src)
  const index = Math.max(0, images.findIndex((g) => g.src === src))

  return (
    <>
      <button
        type="button"
        onClick={(e) => {
          e.stopPropagation()
          setOpen(true)
        }}
        className={cn('shrink-0 cursor-zoom-in', wrapperClassName)}
        title="Lihat foto"
      >
        <img
          src={thumb && isPrivate && !/^https?:\/\//.test(path) ? `${src}?thumb=1` : src}
          alt={alt}
          loading="lazy"
          decoding="async"
          onError={() => setBroken(true)}
          className={cn('rounded object-cover', className)}
        />
      </button>
      {open && <ImageLightbox images={images} index={index} onClose={() => setOpen(false)} />}
    </>
  )
}

export default Thumb
