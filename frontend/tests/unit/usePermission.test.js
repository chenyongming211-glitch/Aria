import { describe, it, expect, vi, beforeEach } from 'vitest'

const mockUserStore = {
  permissions: []
}

vi.mock('@/stores', () => ({
  useUserStore: () => mockUserStore
}))

import { usePermission } from '@/composables/usePermission'

describe('usePermission', () => {
  beforeEach(() => {
    mockUserStore.permissions = []
  })

  it('grants exact permission matches', () => {
    mockUserStore.permissions = ['nodes:read']
    const { hasPermission } = usePermission()
    expect(hasPermission('nodes:read')).toBe(true)
    expect(hasPermission('nodes:write')).toBe(false)
  })

  it('grants wildcard permissions', () => {
    mockUserStore.permissions = ['*']
    const { hasPermission, hasAllPermissions } = usePermission()
    expect(hasPermission('tokens:write')).toBe(true)
    expect(hasAllPermissions(['nodes:read', 'settings:write'])).toBe(true)
  })

  it('supports route meta permission checks', () => {
    mockUserStore.permissions = ['monitoring:read']
    const { canAccessRoute } = usePermission()
    expect(canAccessRoute({ permission: 'monitoring:read' })).toBe(true)
    expect(canAccessRoute({ permission: 'monitoring:write' })).toBe(false)
    expect(canAccessRoute({ permission: ['tokens:read', 'monitoring:read'] })).toBe(true)
  })
})
