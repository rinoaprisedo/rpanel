import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Folder, FolderOpen, CornerLeftUp, ChevronRight } from 'lucide-react'
import Modal from '../../components/ui/Modal'
import Button from '../../components/ui/Button'
import Skeleton from '../../components/ui/Skeleton'
import * as fm from '../../api/fileManager'

const parentOf = (p) => (p.replace(/\/+$/, '').split('/').slice(0, -1).join('/') || '/')

// Browse the server's folders (same access as the File Manager) and pick one.
function FolderBrowser({ open, onClose, onSelect }) {
  const [dir, setDir] = useState('')
  const { data: rootsRes } = useQuery({ queryKey: ['fm-roots'], queryFn: fm.getRoots, enabled: open })
  const roots = rootsRes?.data || []
  useEffect(() => { if (open && !dir && roots.length) setDir(roots[0]) }, [open, dir, roots])
  useEffect(() => { if (!open) setDir('') }, [open])

  const { data, isLoading, isError } = useQuery({ queryKey: ['fm-list', dir], queryFn: () => fm.listDir(dir), enabled: open && !!dir, retry: false })
  const dirs = (data?.data?.entries || []).filter((e) => e.is_dir).sort((a, b) => a.name.localeCompare(b.name))
  const isRoot = roots.includes(dir)
  const crumbs = dir.split('/').filter(Boolean)

  return (
    <Modal open={open} onClose={onClose} title="Choose a folder">
      <div className="space-y-3">
        <div className="flex flex-wrap items-center gap-1 rounded-md bg-surface-hover px-3 py-2 font-mono text-xs">
          <button type="button" className="text-primary hover:underline" onClick={() => roots[0] && setDir(roots[0])}>/</button>
          {crumbs.map((seg, i) => (
            <span key={i} className="flex items-center gap-1">
              <ChevronRight size={12} className="text-text-tertiary" />
              <button type="button" className="text-primary hover:underline" onClick={() => setDir(`/${crumbs.slice(0, i + 1).join('/')}`)}>{seg}</button>
            </span>
          ))}
        </div>

        <div className="h-64 overflow-y-auto rounded-md border border-surface-border">
          {isLoading || !dir ? <Skeleton className="h-full w-full" /> : isError ? (
            <p className="px-3 py-8 text-center text-sm text-danger">This folder cannot be opened</p>
          ) : (
            <ul className="divide-y divide-surface-border text-sm">
              {!isRoot && dir !== '/' && (
                <li><button type="button" className="flex w-full items-center gap-2 px-3 py-2 text-left text-text-secondary hover:bg-surface-hover" onClick={() => setDir(parentOf(dir))}><CornerLeftUp size={15} />..</button></li>
              )}
              {dirs.map((d) => (
                <li key={d.path}>
                  <button type="button" className="flex w-full items-center gap-2 px-3 py-2 text-left text-text-primary hover:bg-surface-hover" onClick={() => setDir(d.path)} onDoubleClick={() => onSelect(d.path)}>
                    <Folder size={15} className="text-amber-500" />{d.name}
                  </button>
                </li>
              ))}
              {!dirs.length && <li className="px-3 py-8 text-center text-text-secondary">No sub-folders here</li>}
            </ul>
          )}
        </div>

        <p className="flex items-center gap-2 text-xs text-text-secondary"><FolderOpen size={14} />Selected: <span className="truncate font-mono text-text-primary">{dir || '…'}</span></p>
        <div className="flex justify-end gap-2">
          <Button type="button" variant="secondary" onClick={onClose}>Cancel</Button>
          <Button type="button" disabled={!dir || isError || dir === '/'} onClick={() => onSelect(dir)}>Select this folder</Button>
        </div>
      </div>
    </Modal>
  )
}

export default FolderBrowser
