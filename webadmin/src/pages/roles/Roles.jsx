import { useState, useMemo } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import toast from 'react-hot-toast'
import { Plus, Pencil, Trash2, ShieldCheck } from 'lucide-react'
import Table from '../../components/ui/Table'
import Badge from '../../components/ui/Badge'
import ConfirmDialog from '../../components/ui/ConfirmDialog'
import { Menubar, MenubarAction, MenubarLabel, MenubarSeparator } from '../../components/ui/Menubar'
import RoleFormModal from './RoleFormModal'
import { getRoles, createRole, updateRole, deleteRole } from '../../api/roles'
import usePermission from '../../hooks/usePermission'
import usePageActions from '../../hooks/usePageActions'

function Roles() {
  const { canCreate, canEdit, canDelete } = usePermission()
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const [page, setPage] = useState(1)
  const [limit, setLimit] = useState(10)
  const [search, setSearch] = useState('')
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState(null)
  const [deleting, setDeleting] = useState(null)

  const { data, isLoading } = useQuery({
    queryKey: ['roles', { page, limit, search }],
    queryFn: () => getRoles({ page, limit, search }),
  })

  const saveMutation = useMutation({
    mutationFn: (payload) => (editing ? updateRole(editing.uuid, payload) : createRole(payload)),
    onSuccess: () => {
      toast.success(editing ? 'Role updated' : 'Role created')
      queryClient.invalidateQueries({ queryKey: ['roles'] })
      setFormOpen(false)
      setEditing(null)
    },
    onError: (err) => toast.error(err.response?.data?.message || 'Save failed'),
  })

  const deleteMutation = useMutation({
    mutationFn: (uuid) => deleteRole(uuid),
    onSuccess: () => {
      toast.success('Role deleted')
      queryClient.invalidateQueries({ queryKey: ['roles'] })
      setDeleting(null)
    },
    onError: (err) => toast.error(err.response?.data?.message || 'Delete failed'),
  })

  const columns = [
    {
      key: 'name',
      label: 'Name',
      render: (row) => (
        <div className="flex items-center gap-2">
          {row.name}
          {row.is_superadmin && <Badge variant="warning">Superadmin</Badge>}
        </div>
      ),
    },
    { key: 'description', label: 'Description' },
    {
      key: 'actions',
      label: '',
      render: (row) => (
        <div className="flex justify-end gap-1">
          {!row.is_superadmin && (
            <button
              onClick={() => navigate(`/roles/${row.uuid}/permissions`)}
              className="rounded p-1.5 hover:bg-surface-hover"
              title="Permissions"
            >
              <ShieldCheck size={16} />
            </button>
          )}
          {canEdit('roles') && !row.is_superadmin && (
            <button
              onClick={() => {
                setEditing(row)
                setFormOpen(true)
              }}
              className="rounded p-1.5 hover:bg-surface-hover"
              title="Edit"
            >
              <Pencil size={16} />
            </button>
          )}
          {canDelete('roles') && !row.is_superadmin && (
            <button onClick={() => setDeleting(row)} className="rounded p-1.5 text-danger hover:bg-danger/10" title="Delete">
              <Trash2 size={16} />
            </button>
          )}
        </div>
      ),
    },
  ]

  usePageActions(
    useMemo(
      () => (
        <Menubar>
          <MenubarLabel>Roles</MenubarLabel>
          {canCreate('roles') && (
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
      <Table
        columns={columns}
        data={data?.data || []}
        loading={isLoading}
        search={search}
        onSearchChange={(v) => {
          setSearch(v)
          setPage(1)
        }}
        page={page}
        limit={limit}
        total={data?.meta?.total || 0}
        onPageChange={setPage}
        onLimitChange={(l) => {
          setLimit(l)
          setPage(1)
        }}
      />

      <RoleFormModal
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
        message={`Delete role "${deleting?.name}"? This cannot be undone.`}
      />
    </div>
  )
}

export default Roles
