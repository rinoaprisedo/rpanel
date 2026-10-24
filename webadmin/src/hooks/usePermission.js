import { useCallback } from 'react'
import useAuthStore from '../store/authStore'

// permissions shape: [{ path, can_view, can_create, can_edit, can_delete }]
function usePermission() {
  const permissions = useAuthStore((s) => s.permissions)
  const user = useAuthStore((s) => s.user)
  const isSuperadmin = !!user?.is_superadmin

  const check = useCallback(
    (moduleName, field) => {
      if (isSuperadmin) return true
      const path = String(moduleName || '').replace(/^\//, '')
      const perm = permissions?.find((p) => String(p.path || '').replace(/^\//, '') === path)
      return !!perm?.[field]
    },
    [permissions, isSuperadmin],
  )

  return {
    canView: (moduleName) => check(moduleName, 'can_view'),
    canCreate: (moduleName) => check(moduleName, 'can_create'),
    canEdit: (moduleName) => check(moduleName, 'can_edit'),
    canDelete: (moduleName) => check(moduleName, 'can_delete'),
  }
}

export default usePermission
