import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useTenantApi } from '@/composables/useTenantApi'

vi.mock('@/composables/useApi', () => ({
  default: {
    get: vi.fn()
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

describe('useTenantApi', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('unwraps paginated tenant node responses', async () => {
    vi.mocked(api.get).mockResolvedValueOnce({
      data: {
        success: true,
        data: {
          items: [{ id: 'node-1', hostname: 'edge-1' }],
          limit: 200,
          offset: 0,
          count: 1
        }
      }
    })

    const nodes = await useTenantApi.getTenantNodes()

    expect(api.get).toHaveBeenCalledWith('/v2/tenants/tenant-1/nodes')
    expect(nodes).toEqual([{ id: 'node-1', hostname: 'edge-1' }])
  })

  it('uses paginated tenant nodes when aggregating ACL rules', async () => {
    vi.mocked(api.get)
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: {
            items: [{ id: 'node-1', hostname: 'edge-1' }]
          }
        }
      })
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: [{ id: 'rule-1', action: 'allow' }]
        }
      })

    const rules = await useTenantApi.getTenantACLRules()

    expect(api.get).toHaveBeenNthCalledWith(1, '/v2/tenants/tenant-1/nodes')
    expect(api.get).toHaveBeenNthCalledWith(2, '/v2/tenants/tenant-1/nodes/node-1/security/acls')
    expect(rules).toEqual([{
      id: 'rule-1',
      action: 'allow',
      node_id: 'node-1',
      node_name: 'edge-1'
    }])
  })
})
