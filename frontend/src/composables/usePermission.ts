import { useUserStore } from '@/stores'
import type { Permission } from '@/types'

type PermissionInput = Permission | string

interface RouteMetaLike {
  permission?: PermissionInput | PermissionInput[]
}

export function usePermission() {
  const userStore = useUserStore()
  const getPermissions = (): PermissionInput[] => userStore.permissions || []

  const isSuperAdmin = () => userStore.user?.role === 'super_admin'
  const hasWildcard = () => isSuperAdmin()

  const hasPermission = (permission?: PermissionInput | null) => {
    if (!permission) return true
    const permissions = getPermissions()
    return permissions.includes(permission) || hasWildcard()
  }

  const hasAnyPermission = (perms?: PermissionInput[] | null) => {
    if (!Array.isArray(perms) || perms.length === 0) return true
    if (hasWildcard()) return true
    const permissions = getPermissions()
    return perms.some(p => permissions.includes(p))
  }

  const hasAllPermissions = (perms?: PermissionInput[] | null) => {
    if (!Array.isArray(perms) || perms.length === 0) return true
    if (hasWildcard()) return true
    const permissions = getPermissions()
    return perms.every(p => permissions.includes(p))
  }

  const canAccessRoute = (routeMeta?: RouteMetaLike | null) => {
    const required = routeMeta?.permission
    if (!required) return true
    return Array.isArray(required) ? hasAnyPermission(required) : hasPermission(required)
  }

  return { hasPermission, hasAnyPermission, hasAllPermissions, canAccessRoute }
}
