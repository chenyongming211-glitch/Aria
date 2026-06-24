import { describe, it, expect, vi, beforeEach } from 'vitest'
import { usePolicyApi } from '@/composables/usePolicyApi'

vi.mock('@/composables/useApi', () => ({
  default: {
    get: vi.fn(),
    post: vi.fn()
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

describe('usePolicyApi', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('应该重新下发失败策略并归一化返回的投递状态', async () => {
    api.post.mockResolvedValue({
      data: {
        success: true,
        data: {
          policy_id: 'acl:acl-1',
          policy_ref: 'acl-1',
          kind: 'acl',
          node_id: 'node-1',
          status: 'pending',
          last_delivery_command_id: 'cmd-retry',
          last_delivery: {
            id: 'delivery-retry',
            command_id: 'cmd-retry',
            command_status: 'pending',
            action: 'retry'
          },
          delivery_history: [{
            id: 'delivery-retry',
            command_id: 'cmd-retry',
            command_status: 'pending',
            action: 'retry'
          }]
        }
      }
    })

    const result = await usePolicyApi.retryPolicySync({
      nodeId: 'node-1',
      kind: 'acl',
      policyRef: 'acl-1',
      policyName: 'deny-test'
    })

    expect(api.post).toHaveBeenCalledWith('/v2/tenants/tenant-1/policies/retry', {
      node_id: 'node-1',
      kind: 'acl',
      policy_ref: 'acl-1',
      policy_name: 'deny-test'
    })
    expect(result.status).toBe('pending')
    expect(result.lastDeliveryCommandId).toBe('cmd-retry')
    expect(result.lastDeliveryAction).toBe('retry')
    expect(result.deliveryHistory).toHaveLength(1)
  })
})
