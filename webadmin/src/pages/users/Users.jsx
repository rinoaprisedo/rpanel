import { useState, useMemo } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { Plus, Pencil, Trash2, MoreHorizontal, FileSpreadsheet, FileText, Filter } from 'lucide-react'
import { format } from 'date-fns'
import Table from '../../components/ui/Table'
import Button from '../../components/ui/Button'
import Badge from '../../components/ui/Badge'
import { Menubar, MenubarAction, MenubarMenu, MenubarItem, MenubarLabel, MenubarSeparator } from '../../components/ui/Menubar'
import Select from '../../components/ui/Select'
import ConfirmDialog from '../../components/ui/ConfirmDialog'
import UserFormModal from './UserFormModal'
import { getUsers, createUser, updateUser, deleteUser, exportUsersCsv, exportUsersExcel } from '../../api/users'
import { getRoles } from '../../api/roles'
import useAuthStore from '../../store/authStore'
import usePermission from '../../hooks/usePermission'
import usePageActions from '../../hooks/usePageActions'

const statusVariant = { active: 'success', inactive: 'neutral', suspended: 'danger' }

function Users() {
  const { canCreate, canEdit, canDelete } = usePermission()
  const currentUser = useAuthStore((s) => s.user)
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [limit, setLimit] = useState(10)
  const [search, setSearch] = useState('')
  const [sortBy, setSortBy] = useState('created_at')
  const [sortDir, setSortDir] = useState('desc')
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState(null)
  const [deleting, setDeleting] = useState(null)
  const [status, setStatus] = useState('')
  const [roleFilter, setRoleFilter] = useState('')
  const [draftStatus, setDraftStatus] = useState('')
  const [draftRole, setDraftRole] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['users', { page, limit, search, sortBy, sortDir, status, roleFilter }],
    queryFn: () =>
      getUsers({ page, limit, search, sort_by: sortBy, sort_dir: sortDir, status, role: roleFilter }),
  })

  const { data: rolesData } = useQuery({ queryKey: ['roles', 'all'], queryFn: () => getRoles({ limit: 100 }) })

  const saveMutation = useMutation({
    mutationFn: (payload) => (editing ? updateUser(editing.uuid, payload) : createUser(payload)),
    onSuccess: () => {
      toast.success(editing ? 'User updated' : 'User created')
      queryClient.invalidateQueries({ queryKey: ['users'] })
      setFormOpen(false)
      setEditing(null)
    },
    onError: (err) => toast.error(err.response?.data?.message || 'Save failed'),
  })

  const deleteMutation = useMutation({
    mutationFn: (uuid) => deleteUser(uuid),
    onSuccess: () => {
      toast.success('User deleted')
      queryClient.invalidateQueries({ queryKey: ['users'] })
      setDeleting(null)
    },
    onError: (err) => toast.error(err.response?.data?.message || 'Delete failed'),
  })

  const columns = [
    {
      key: 'name',
      label: 'Name',
      sortable: true,
      render: (row) => (
        <div className="flex items-center gap-2">
          <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary/10 text-xs font-semibold text-primary">
            {row.avatar ? (
              <img src={row.avatar} alt={row.name} className="h-8 w-8 rounded-full object-cover" />
            ) : (
              row.name?.charAt(0).toUpperCase()
            )}
          </div>
          <span className="font-medium">{row.name}</span>
        </div>
      ),
    },
    { key: 'email', label: 'Email', sortable: true },
    {
      key: 'status',
      label: 'Status',
      sortable: true,
      render: (row) => <Badge variant={statusVariant[row.status] || 'neutral'}>{row.status}</Badge>,
    },
    {
      key: 'role',
      label: 'Role',
      render: (row) => row.role?.name || '-',
    },
    {
      key: 'last_login_at',
      label: 'Last Login',
      sortable: true,
      render: (row) => (row.last_login_at ? format(new Date(row.last_login_at), 'PP p') : 'Never'),
    },
    {
      key: 'actions',
      label: '',
      render: (row) => (
        <div className="flex justify-end gap-1">
          {canEdit('users') && (
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
          {canDelete('users') && row.uuid !== currentUser?.uuid && (
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
        <div className="flex min-w-0 items-center gap-3">
          <Menubar>
            <MenubarLabel>Users</MenubarLabel>
            <MenubarSeparator />
            {canCreate('users') && (
              <MenubarAction
                label="Add New"
                icon={Plus}
                onClick={() => {
                  setEditing(null)
                  setFormOpen(true)
                }}
              />
            )}
            <MenubarMenu id="filter" label="Filter" icon={Filter} align="sheet" active={!!(status || roleFilter)}>
              {(close) => (
                <div className="w-full p-2 sm:w-60">
                  <div className="flex flex-col gap-3">
                    <Select label="Status" value={draftStatus} onChange={(e) => setDraftStatus(e.target.value)}>
                      <option value="">All</option>
                      <option value="active">Active</option>
                      <option value="inactive">Inactive</option>
                      <option value="suspended">Suspended</option>
                    </Select>
                    <Select label="Role" value={draftRole} onChange={(e) => setDraftRole(e.target.value)}>
                      <option value="">All</option>
                      {(rolesData?.data || []).map((r) => (
                        <option key={r.uuid} value={r.uuid}>
                          {r.name}
                        </option>
                      ))}
                    </Select>
                  </div>
                  <div className="mt-3 flex items-center justify-end gap-2 border-t border-surface-border pt-3">
                    <Button
                      variant="secondary"
                      className="h-[32px] px-3"
                      onClick={() => {
                        setDraftStatus('')
                        setDraftRole('')
                        setStatus('')
                        setRoleFilter('')
                        setPage(1)
                        close()
                      }}
                    >
                      Reset
                    </Button>
                    <Button
                      className="h-[32px] px-3"
                      onClick={() => {
                        setStatus(draftStatus)
                        setRoleFilter(draftRole)
                        setPage(1)
                        close()
                      }}
                    >
                      Apply
                    </Button>
                  </div>
                </div>
              )}
            </MenubarMenu>
            <MenubarMenu id="others" label="Others" icon={MoreHorizontal} align="right">
              <MenubarItem
                label="Export Excel"
                icon={FileSpreadsheet}
                onClick={() =>
                  toast.promise(exportUsersExcel({ search, status, role: roleFilter }), {
                    loading: 'Exporting...',
                    success: 'Excel file downloaded',
                    error: 'Export failed',
                  })
                }
              />
              <MenubarItem
                label="Export CSV"
                icon={FileText}
                onClick={() =>
                  toast.promise(exportUsersCsv({ search, status, role: roleFilter }), {
                    loading: 'Exporting...',
                    success: 'CSV file downloaded',
                    error: 'Export failed',
                  })
                }
              />
            </MenubarMenu>
          </Menubar>
        </div>
      ),
      [canCreate, status, roleFilter, draftStatus, draftRole, rolesData, search],
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
        sortBy={sortBy}
        sortDir={sortDir}
        onSortChange={(k, d) => {
          setSortBy(k)
          setSortDir(d)
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

      <UserFormModal
        open={formOpen}
        onClose={() => setFormOpen(false)}
        onSubmit={(values) => saveMutation.mutate(values)}
        initialData={editing}
        roles={rolesData?.data || []}
        loading={saveMutation.isPending}
      />


      <ConfirmDialog
        open={!!deleting}
        onClose={() => setDeleting(null)}
        onConfirm={() => deleteMutation.mutate(deleting.uuid)}
        loading={deleteMutation.isPending}
        message={`Delete user "${deleting?.name}"? This cannot be undone.`}
      />
    </div>
  )
}

export default Users
