import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

vi.mock('@/composables/useApi', () => ({
  default: {
    get: vi.fn()
  }
}))

import useTenantStore from '@/stores/tenant'
import useUserStore from '@/stores/user'
import api from '@/composables/useApi'

describe('tenant store', () => {
  let storage

  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    storage = new Map()
    globalThis.localStorage = {
      getItem: (key) => (storage.has(key) ? storage.get(key) : null),
      setItem: (key, value) => storage.set(key, String(value)),
      removeItem: (key) => storage.delete(key),
      clear: () => storage.clear()
    }
  })

  it('reloads permissions and notifies views when switching tenant', async () => {
    const userStore = useUserStore()
    userStore.user = {
      id: 'user-1',
      username: 'admin',
      role: 'admin',
      tenant_id: 'tenant-1'
    }
    const loadPermissions = vi.spyOn(userStore, 'loadPermissions').mockResolvedValue(['nodes:read'])
    const tenantChanged = vi.fn()
    window.addEventListener('tenantChanged', tenantChanged)

    const tenantStore = useTenantStore()
    await tenantStore.switchTenant({ id: 'tenant-2', name: 'Tenant 2' })

    expect(loadPermissions).toHaveBeenCalledWith('tenant-2', 'admin')
    expect(JSON.parse(localStorage.getItem('aria-current-tenant'))).toEqual({
      id: 'tenant-2',
      name: 'Tenant 2'
    })
    expect(tenantChanged).toHaveBeenCalledTimes(1)
  })

  it('only selects active tenants for the global tenant context', async () => {
    api.get.mockResolvedValueOnce({
      data: {
        data: [
          { id: 'tenant-deleted', name: 'Deleted', status: 'deleted' },
          { id: 'tenant-suspended', name: 'Suspended', status: 'suspended' },
          { id: 'tenant-active', name: 'Active', status: 'active' }
        ]
      }
    })
    localStorage.setItem('aria-current-tenant', JSON.stringify({
      id: 'tenant-deleted',
      name: 'Deleted',
      status: 'deleted'
    }))

    const tenantStore = useTenantStore()
    await tenantStore.loadTenants()

    expect(tenantStore.tenants).toEqual([
      { id: 'tenant-active', name: 'Active', status: 'active' }
    ])
    expect(tenantStore.currentTenant).toEqual({
      id: 'tenant-active',
      name: 'Active',
      status: 'active'
    })
    expect(JSON.parse(localStorage.getItem('aria-current-tenant'))).toEqual({
      id: 'tenant-active',
      name: 'Active',
      status: 'active'
    })
  })
})
