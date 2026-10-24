import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Folder, FolderOpen, CornerLeftUp, ChevronRight } from 'lucide-react'
import Modal from '../../components/ui/Modal'
import Button from '../../components/ui/Button'
import Skeleton from '../../components/ui/Skeleton'
import * as api from '../../api/websites'

// Lets the user pick an existing folder inside the web root instead of typing a path.
function FolderPicker({ open, initial, onClose, onSelect }) {
  const [dir, setDir] = useState(undefined) // undefined = web root

  useEffect(() => { if (open) setDir(initial || undefined) }, [open, initial])

  const { data, isLoading, isError } = useQuery({
    queryKey: ['site-dirs', dir],
    queryFn: () => api.getWebsiteDirs(dir),
    enabled: open,
    retry: false,
  })
  // A typed path that does not exist (yet) falls back to the web root.
  useEffect(() => { if (isError && dir) setDir(undefined) }, [isError, dir])

  const list = data?.data
  const rel = list ? list.path.slice(list.root.length).split('/').filter(Boolean) : []

  return (
    <Modal open={open} onClose={onClose} title="Choose root folder">
      <div className="space-y-3">
        <div className="flex flex-wrap items-center gap-1 rounded-md bg-surface-hover px-3 py-2 font-mono text-xs">
          <button type="button" className="text-primary hover:underline" onClick={() => setDir(undefined)}>{list?.root || '/www/wwwroot'}</button>
          {rel.map((seg, i) => (
            <span key={i} className="flex items-center gap-1">
              <ChevronRight size={12} className="text-text-tertiary" />
              <button type="button" className="text-primary hover:underline" onClick={() => setDir(`${list.root}/${rel.slice(0, i + 1).join('/')}`)}>{seg}</button>
            </span>
          ))}
        </div>

        <div className="h-64 overflow-y-auto rounded-md border border-surface-border">
          {isLoading ? <Skeleton className="h-full w-full" /> : (
            <ul className="divide-y divide-surface-border text-sm">
              {list?.parent && (
                <li>
                  <button type="button" className="flex w-full items-center gap-2 px-3 py-2 text-left text-text-secondary hover:bg-surface-hover" onClick={() => setDir(list.parent)}>
                    <CornerLeftUp size={15} />..
                  </button>
                </li>
              )}
              {list?.dirs.map((d) => (
                <li key={d.path}>
                  <button type="button" className="flex w-full items-center gap-2 px-3 py-2 text-left text-text-primary hover:bg-surface-hover" onClick={() => setDir(d.path)} onDoubleClick={() => onSelect(d.path)}>
                    <Folder size={15} className="text-amber-500" />{d.name}
                  </button>
                </li>
              ))}
              {list && !list.dirs.length && <li className="px-3 py-8 text-center text-text-secondary">No sub-folders here</li>}
            </ul>
          )}
        </div>

        <p className="flex items-center gap-2 text-xs text-text-secondary"><FolderOpen size={14} />Selected: <span className="truncate font-mono text-text-primary">{list?.path || '…'}</span></p>
        <div className="flex justify-end gap-2">
          <Button type="button" variant="secondary" onClick={onClose}>Cancel</Button>
          <Button type="button" disabled={!list} onClick={() => onSelect(list.path)}>Select this folder</Button>
        </div>
      </div>
    </Modal>
  )
}

export default FolderPicker
