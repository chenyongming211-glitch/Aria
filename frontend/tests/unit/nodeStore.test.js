import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import useNodeStore from '@/stores/node'

vi.mock('@/composables/useApi', () => ({
  default: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn()
  }
}))

vi.mock('@/config/api', async () => {
  const actual = await vi.importActual('@/config/api')
  return {
    ...actual,
    requireCurrentTenantId: vi.fn(() => 'tenant-1')
  }
})

import api from '@/composables/useApi'

describe('node store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('deletes nodes through the tenant-scoped backend API', async () => {
    api.delete.mockResolvedValue({ data: { success: true } })
    const store = useNodeStore()
    store.nodes = [{ id: 'node-1' }, { id: 'node-2' }]

    await store.deleteNodeRemote('node-1')

    expect(api.delete).toHaveBeenCalledWith('/v2/tenants/tenant-1/nodes/node-1')
    expect(store.nodes).toEqual([{ id: 'node-2' }])
  })
})
