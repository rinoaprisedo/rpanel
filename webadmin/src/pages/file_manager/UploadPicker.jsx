import { useRef, useState } from 'react'
import { UploadCloud } from 'lucide-react'
import Modal from '../../components/ui/Modal'
import { cn } from '../../utils/cn'

// Popup with a drop zone; hands the chosen files to the upload flow.
function UploadPicker({ dir, onPick, onClose }) {
  const input = useRef(null)
  const [over, setOver] = useState(false)

  const pick = (list) => {
    const files = [...list]
    if (files.length) onPick(files)
  }

  return (
    <Modal open onClose={onClose} title="Upload files" size="lg">
      <div className="space-y-4">
        <p className="text-sm text-text-secondary">
          Destination <code className="rounded bg-surface-hover px-1.5 py-0.5 font-mono text-xs text-text-primary">{dir}</code>
        </p>
        <div
          role="button" tabIndex={0}
          onClick={() => input.current?.click()}
          onKeyDown={(e) => (e.key === 'Enter' || e.key === ' ') && input.current?.click()}
          onDragOver={(e) => { e.preventDefault(); setOver(true) }}
          onDragLeave={() => setOver(false)}
          onDrop={(e) => { e.preventDefault(); setOver(false); pick(e.dataTransfer.files) }}
          className={cn(
            'flex cursor-pointer flex-col items-center justify-center gap-2 rounded-xl border-2 border-dashed px-6 py-14 text-center transition-colors',
            over ? 'border-primary bg-primary/10' : 'border-surface-border hover:border-primary/60 hover:bg-surface-hover',
          )}
        >
          <UploadCloud size={36} className={over ? 'text-primary' : 'text-text-tertiary'} />
          <p className="text-sm font-medium text-text-primary">{over ? 'Drop to upload' : 'Drag & drop files here'}</p>
          <p className="text-xs text-text-secondary">or <span className="text-primary">click to browse</span></p>
        </div>
        <input ref={input} type="file" multiple hidden onChange={(e) => { const f = e.target.files; pick(f); e.target.value = '' }} />
      </div>
    </Modal>
  )
}

export default UploadPicker
