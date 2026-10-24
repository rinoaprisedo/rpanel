import { useEffect, useState, useMemo } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useQuery, useMutation } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { ArrowLeft, Save } from 'lucide-react'
import Skeleton from '../../components/ui/Skeleton'
import { Menubar, MenubarAction, MenubarLabel, MenubarSeparator } from '../../components/ui/Menubar'
import { getRole, getRolePermissions, updateRolePermissions } from '../../api/roles'
import { getMenus } from '../../api/menus'
import usePageActions from '../../hooks/usePageActions'

const fields = [
  { key: 'can_view', label: 'View' },
  { key: 'can_create', label: 'Create' },
  { key: 'can_edit', label: 'Edit' },
  { key: 'can_delete', label: 'Delete' },
]

function RolePermissions() {
  const { uuid } = useParams()
  const navigate = useNavigate()
  const [matrix, setMatrix] = useState({})

  const { data: role } = useQuery({ queryKey: ['roles', uuid], queryFn: () => getRole(uuid) })
  const roleName = role?.data?.role?.name

  // Full menu catalog — the matrix always shows every menu, not just ones
  // that already have a saved permission row.
  const { data: menusData, isLoading: menusLoading } = useQuery({
    queryKey: ['menus', 'tree'],
    queryFn: () => getMenus(),
  })
  const { data: permsData, isLoading: permsLoading } = useQuery({
    queryKey: ['roles', uuid, 'permissions'],
    queryFn: () => getRolePermissions(uuid),
  })

  const isLoading = menusLoading || permsLoading

  useEffect(() => {
    if (!menusData?.data) return

    const savedByMenu = {}
    ;(permsData?.data || []).forEach((row) => {
      savedByMenu[row.menu_uuid] = row
    })

    const initial = {}
    menusData.data.forEach((section) => {
      ;(section.menus || []).forEach((menu) => {
        const saved = savedByMenu[menu.uuid]
        initial[menu.uuid] = {
          menu_uuid: menu.uuid,
          menu_name: menu.name,
          can_view: !!saved?.can_view,
          can_create: !!saved?.can_create,
          can_edit: !!saved?.can_edit,
          can_delete: !!saved?.can_delete,
        }
      })
    })
    setMatrix(initial)
  }, [menusData, permsData])

  const saveMutation = useMutation({
    mutationFn: (payload) => updateRolePermissions(uuid, { permissions: payload }),
    onSuccess: () => toast.success('Permissions updated'),
    onError: (err) => toast.error(err.response?.data?.message || 'Save failed'),
  })

  const toggle = (menuUuid, field) => {
    setMatrix((prev) => ({
      ...prev,
      [menuUuid]: { ...prev[menuUuid], [field]: !prev[menuUuid][field] },
    }))
  }

  const toggleRow = (menuUuid) => {
    setMatrix((prev) => {
      const row = prev[menuUuid]
      const allOn = fields.every((f) => row[f.key])
      const next = {}
      fields.forEach((f) => {
        next[f.key] = !allOn
      })
      return { ...prev, [menuUuid]: { ...row, ...next } }
    })
  }

  const handleSave = () => {
    const payload = Object.values(matrix).map((row) => ({
      menu_uuid: row.menu_uuid,
      can_view: row.can_view,
      can_create: row.can_create,
      can_edit: row.can_edit,
      can_delete: row.can_delete,
    }))
    saveMutation.mutate(payload)
  }

  const rows = Object.entries(matrix)

  usePageActions(
    useMemo(
      () => (
        <Menubar>
          <MenubarAction label="Back" icon={ArrowLeft} onClick={() => navigate('/roles')} />
          <MenubarSeparator />
          <MenubarLabel>Permissions{roleName ? ` - ${roleName}` : ''}</MenubarLabel>
          <MenubarSeparator />
          <MenubarAction
            label={saveMutation.isPending ? 'Saving...' : 'Save Changes'}
            icon={Save}
            onClick={handleSave}
          />
        </Menubar>
      ),
      [roleName, saveMutation.isPending, matrix],
    ),
  )

  return (
    <div className="space-y-4">
      <div className="overflow-x-auto rounded-lg border border-surface-border bg-surface-card">
        {isLoading ? (
          <div className="p-4 space-y-2">
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="h-6 w-full" />
            ))}
          </div>
        ) : (
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-surface-border text-text-secondary">
                <th className="px-4 py-3 font-medium">Menu</th>
                {fields.map((f) => (
                  <th key={f.key} className="px-4 py-3 text-center font-medium">
                    {f.label}
                  </th>
                ))}
                <th className="px-4 py-3 text-center font-medium">All</th>
              </tr>
            </thead>
            <tbody>
              {rows.map(([menuUuid, row]) => (
                <tr key={menuUuid} className="border-b border-surface-border last:border-0">
                  <td className="px-4 py-3 font-medium text-text-primary">{row.menu_name}</td>
                  {fields.map((f) => (
                    <td key={f.key} className="px-4 py-3 text-center">
                      <input
                        type="checkbox"
                        checked={!!row[f.key]}
                        onChange={() => toggle(menuUuid, f.key)}
                        className="h-4 w-4 accent-primary"
                      />
                    </td>
                  ))}
                  <td className="px-4 py-3 text-center">
                    <input
                      type="checkbox"
                      checked={fields.every((f) => row[f.key])}
                      onChange={() => toggleRow(menuUuid)}
                      className="h-4 w-4 accent-primary"
                    />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}

export default RolePermissions
