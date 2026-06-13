// src/composables/usePermission.js
import { useUserStore } from '@/stores'

export function usePermission() {
  const userStore = useUserStore()
  const getPermissions = () => userStore.permissions || []

  const isSuperAdmin = () => userStore.user?.role === 'super_admin'
  const hasWildcard = () => isSuperAdmin()

  const hasPermission = (permission) => {
    if (!permission) return true
    const permissions = getPermissions()
    return permissions.includes(permission) || hasWildcard()
  }

  const hasAnyPermission = (perms) => {
    if (!Array.isArray(perms) || perms.length === 0) return true
    if (hasWildcard()) return true
    const permissions = getPermissions()
    return perms.some(p => permissions.includes(p))
  }

  const hasAllPermissions = (perms) => {
    if (!Array.isArray(perms) || perms.length === 0) return true
    if (hasWildcard()) return true
    const permissions = getPermissions()
    return perms.every(p => permissions.includes(p))
  }

  const canAccessRoute = (routeMeta) => {
    const required = routeMeta?.permission
    if (!required) return true
    return Array.isArray(required) ? hasAnyPermission(required) : hasPermission(required)
  }

  return { hasPermission, hasAnyPermission, hasAllPermissions, canAccessRoute }
}
