import { useState, useMemo } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { Plus, Pencil, Trash2, GripVertical } from 'lucide-react'
import ConfirmDialog from '../../components/ui/ConfirmDialog'
import EmptyState from '../../components/ui/EmptyState'
import Skeleton from '../../components/ui/Skeleton'
import { Menubar, MenubarAction, MenubarLabel, MenubarSeparator } from '../../components/ui/Menubar'
import MenuSectionFormModal from './MenuSectionFormModal'
import {
  getMenuSections,
  createMenuSection,
  updateMenuSection,
  deleteMenuSection,
  reorderMenuSections,
} from '../../api/menus'
import usePermission from '../../hooks/usePermission'
import usePageActions from '../../hooks/usePageActions'

function SectionList() {
  const { canCreate, canEdit, canDelete } = usePermission()
  const queryClient = useQueryClient()
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState(null)
  const [deleting, setDeleting] = useState(null)
  const [dragIndex, setDragIndex] = useState(null)
  const [overIndex, setOverIndex] = useState(null)

  const { data, isLoading } = useQuery({ queryKey: ['menus'], queryFn: () => getMenuSections({ limit: 100 }) })

  const saveMutation = useMutation({
    mutationFn: (payload) => (editing ? updateMenuSection(editing.uuid, payload) : createMenuSection(payload)),
    onSuccess: () => {
      toast.success(editing ? 'Section updated' : 'Section created')
      queryClient.invalidateQueries({ queryKey: ['menus'] })
      setFormOpen(false)
      setEditing(null)
    },
    onError: (err) => toast.error(err.response?.data?.message || 'Save failed'),
  })

  const deleteMutation = useMutation({
    mutationFn: (uuid) => deleteMenuSection(uuid),
    onSuccess: () => {
      toast.success('Section deleted')
      queryClient.invalidateQueries({ queryKey: ['menus'] })
      setDeleting(null)
    },
    onError: (err) => toast.error(err.response?.data?.message || 'Delete failed'),
  })

  const reorderMutation = useMutation({
    mutationFn: (payload) => reorderMenuSections(payload),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['menus'] }),
    onError: (err) => toast.error(err.response?.data?.message || 'Reorder failed'),
  })

  const sections = data?.data || []

  const dropReorder = (fromIndex, toIndex) => {
    if (fromIndex === toIndex) return
    const orders = sections.map((s) => s.order)
    const items = [...sections]
    const [moved] = items.splice(fromIndex, 1)
    items.splice(toIndex, 0, moved)
    reorderMutation.mutate(items.map((s, i) => ({ uuid: s.uuid, order: orders[i] })))
  }

  const resetDrag = () => {
    setDragIndex(null)
    setOverIndex(null)
  }

  usePageActions(
    useMemo(
      () => (
        <Menubar>
          <MenubarLabel>Menus</MenubarLabel>
          {canCreate('menus') && (
            <>
              <MenubarSeparator />
              <MenubarAction
                label="Add New"
                icon={Plus}
                onClick={() => {
                  setEditing(null)
                  setFormOpen(true)
                }}
              />
            </>
          )}
        </Menubar>
      ),
      [canCreate],
    ),
  )

  return (
    <div className="space-y-4">
      {isLoading ? (
        <Skeleton className="h-64 w-full" />
      ) : sections.length === 0 ? (
        <EmptyState title="No menu sections found" />
      ) : (
        <div className="rounded-lg border border-surface-border bg-surface-card">
          <ul className="divide-y divide-surface-border">
            {sections.map((row, i) => {
              const draggable = canEdit('menus')
              const isDragging = dragIndex === i
              const isOver = overIndex === i && dragIndex !== i
              return (
                <li
                  key={row.uuid}
                  draggable={draggable}
                  onDragStart={() => setDragIndex(i)}
                  onDragOver={(e) => {
                    e.preventDefault()
                    setOverIndex(i)
                  }}
                  onDrop={(e) => {
                    e.preventDefault()
                    if (dragIndex !== null) dropReorder(dragIndex, i)
                    resetDrag()
                  }}
                  onDragEnd={resetDrag}
                  className={`flex items-center justify-between gap-3 px-4 py-2.5 ${isDragging ? 'opacity-40' : ''} ${isOver ? 'border-t-2 border-primary' : ''}`}
                >
                  <div className="flex min-w-0 items-center gap-3 text-sm">
                    {draggable && (
                      <GripVertical size={14} className="shrink-0 cursor-grab text-text-secondary active:cursor-grabbing" />
                    )}
                    <span className="min-w-0 flex-1 truncate font-medium text-text-primary">{row.name}</span>
                    {row.icon && <span className="shrink-0 text-text-secondary">{row.icon}</span>}
                    <span className="shrink-0 text-text-tertiary">{row.menu_count ?? (row.menus?.length || 0)} menus</span>
                  </div>
                  <div className="flex shrink-0 items-center gap-1">
                    {canEdit('menus') && (
                      <button
                        onClick={() => {
                          setEditing(row)
                          setFormOpen(true)
                        }}
                        className="rounded p-1.5 hover:bg-surface-hover"
                      >
                        <Pencil size={14} />
                      </button>
                    )}
                    {canDelete('menus') && (
                      <button onClick={() => setDeleting(row)} className="rounded p-1.5 text-danger hover:bg-danger/10">
                        <Trash2 size={14} />
                      </button>
                    )}
                  </div>
                </li>
              )
            })}
          </ul>
        </div>
      )}

      <MenuSectionFormModal
        open={formOpen}
        onClose={() => setFormOpen(false)}
        onSubmit={(values) => saveMutation.mutate(values)}
        initialData={editing}
        loading={saveMutation.isPending}
      />

      <ConfirmDialog
        open={!!deleting}
        onClose={() => setDeleting(null)}
        onConfirm={() => deleteMutation.mutate(deleting.uuid)}
        loading={deleteMutation.isPending}
        message={`Delete section "${deleting?.name}"? This cannot be undone.`}
      />
    </div>
  )
}

export default SectionList
