// src/composables/usePermission.js
import { useUserStore } from '@/stores'

export function usePermission() {
  const userStore = useUserStore()

  const hasPermission = (permission) => {
    const permissions = userStore.permissions || []
    return permissions.includes(permission)
  }

  const hasAnyPermission = (perms) => {
    const permissions = userStore.permissions || []
    return perms.some(p => permissions.includes(p))
  }

  const hasAllPermissions = (perms) => {
    const permissions = userStore.permissions || []
    return perms.every(p => permissions.includes(p))
  }

  return { hasPermission, hasAnyPermission, hasAllPermissions }
}
