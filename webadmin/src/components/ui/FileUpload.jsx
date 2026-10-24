import { useRef, useState } from 'react'
import { UploadCloud, X } from 'lucide-react'
import { cn } from '../../utils/cn'

function FileUpload({
  label,
  hint,
  preview,
  accept = 'image/*',
  onFileSelect,
  onClear,
  previewClassName = 'h-16 w-16',
  className,
}) {
  const inputRef = useRef(null)
  const [isDragging, setIsDragging] = useState(false)

  const handleFiles = (files) => {
    const file = files?.[0]
    if (file) onFileSelect(file)
  }

  return (
    <div className={cn('flex flex-col gap-1', className)}>
      {label && <p className="text-xs font-medium uppercase tracking-[.05em] text-text-secondary">{label}</p>}

      <div
        role="button"
        tabIndex={0}
        onClick={() => inputRef.current?.click()}
        onKeyDown={(e) => e.key === 'Enter' && inputRef.current?.click()}
        onDragOver={(e) => {
          e.preventDefault()
          setIsDragging(true)
        }}
        onDragLeave={() => setIsDragging(false)}
        onDrop={(e) => {
          e.preventDefault()
          setIsDragging(false)
          handleFiles(e.dataTransfer.files)
        }}
        className={cn(
          'group relative flex cursor-pointer items-center gap-4 rounded-lg border-2 border-dashed border-surface-border bg-white px-4 py-4 transition-colors duration-150 hover:border-primary/60 hover:bg-primary/5',
          isDragging && 'border-primary bg-primary/5',
        )}
      >
        <input
          ref={inputRef}
          type="file"
          accept={accept}
          className="hidden"
          onChange={(e) => {
            handleFiles(e.target.files)
            e.target.value = ''
          }}
        />

        {preview ? (
          <div className={cn('shrink-0 overflow-hidden rounded-md border border-surface-border bg-surface-card', previewClassName)}>
            <img src={preview} alt={label || 'preview'} className="h-full w-full object-contain" />
          </div>
        ) : (
          <div className={cn('flex shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary', previewClassName)}>
            <UploadCloud className="h-5 w-5" />
          </div>
        )}

        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium text-text-primary">
            <span className="text-primary">Click to upload</span> or drag and drop
          </p>
          {hint && <p className="mt-0.5 truncate text-xs text-text-secondary">{hint}</p>}
        </div>

        {preview && onClear && (
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation()
              onClear()
            }}
            className="shrink-0 rounded-full p-1.5 text-text-secondary transition-colors hover:bg-danger/10 hover:text-danger"
            aria-label="Remove file"
          >
            <X className="h-4 w-4" />
          </button>
        )}
      </div>
    </div>
  )
}

export default FileUpload
