import { describe, it, expect, vi, beforeEach } from 'vitest'

const mockUserStore = {
  user: null,
  permissions: []
}

vi.mock('@/stores', () => ({
  useUserStore: () => mockUserStore
}))

import { usePermission } from '@/composables/usePermission'

describe('usePermission', () => {
  beforeEach(() => {
    mockUserStore.user = null
    mockUserStore.permissions = []
  })

  it('grants exact permission matches', () => {
    mockUserStore.permissions = ['nodes:read']
    const { hasPermission } = usePermission()
    expect(hasPermission('nodes:read')).toBe(true)
    expect(hasPermission('nodes:write')).toBe(false)
  })

  it('ignores cached wildcard permissions for non-super_admin users', () => {
    mockUserStore.user = { role: 'admin' }
    mockUserStore.permissions = ['*']

    const { hasPermission, hasAnyPermission } = usePermission()

    expect(hasPermission('tokens:write')).toBe(false)
    expect(hasAnyPermission(['nodes:read', 'settings:write'])).toBe(false)
  })

  it('grants wildcard permissions', () => {
    mockUserStore.permissions = ['*']
    const { hasPermission, hasAllPermissions } = usePermission()
    expect(hasPermission('tokens:write')).toBe(true)
    expect(hasAllPermissions(['nodes:read', 'settings:write'])).toBe(true)
  })

  it('grants super_admin permissions even before cached permissions are restored', () => {
    mockUserStore.user = { role: 'super_admin' }
    mockUserStore.permissions = []

    const { hasPermission, hasAnyPermission } = usePermission()

    expect(hasPermission('nodes:read')).toBe(true)
    expect(hasAnyPermission(['tokens:read', 'roles:read'])).toBe(true)
  })

  it('supports route meta permission checks', () => {
    mockUserStore.permissions = ['monitoring:read']
    const { canAccessRoute } = usePermission()
    expect(canAccessRoute({ permission: 'monitoring:read' })).toBe(true)
    expect(canAccessRoute({ permission: 'monitoring:write' })).toBe(false)
    expect(canAccessRoute({ permission: ['tokens:read', 'monitoring:read'] })).toBe(true)
  })
})
