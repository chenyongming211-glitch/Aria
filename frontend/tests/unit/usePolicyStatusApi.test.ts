import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  isActivePolicyStatus,
  patchPolicyStatusRow,
  usePolicyStatusApi
} from '@/composables/usePolicyStatusApi'

vi.mock('@/composables/useApi', () => ({
  default: {
    post: vi.fn()
  }
}))

vi.mock('@/config/api', async () => {
  const actual = await vi.importActual('@/config/api')
  return {
    ...actual,
    getCurrentTenantId: vi.fn(() => 'tenant-1')
  }
})

import api from '@/composables/useApi'

describe('usePolicyStatusApi', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('posts policy refs and normalizes latest delivery status', async () => {
    vi.mocked(api.post).mockResolvedValue({
      data: {
        success: true,
        data: {
          items: [{
            node_id: 'node-1',
            policy_domain: 'acl',
            policy_ref: 'rule-1',
            policy_status: 'in_progress',
            pending_cmds: 1,
            last_delivery: {
              command_id: 'cmd-1',
              command_status: 'sent',
              action: 'create',
              last_error: ''
            },
            delivery_history: [{
              command_id: 'cmd-1',
              command_status: 'sent',
              action: 'create'
            }]
          }]
        }
      }
    })

    const result = await usePolicyStatusApi.getPolicyDeliveryStatuses([{
      nodeId: 'node-1',
      policyDomain: 'acl',
      policyRef: 'rule-1'
    }])

    expect(api.post).toHaveBeenCalledWith('/v2/tenants/tenant-1/policy-deliveries/status', {
      items: [{
        node_id: 'node-1',
        policy_domain: 'acl',
        policy_ref: 'rule-1'
      }]
    })
    expect(result[0].policyStatus).toBe('in_progress')
    expect(result[0].pendingCmds).toBe(1)
    expect(result[0].lastDeliveryCommandId).toBe('cmd-1')
    expect(result[0].deliveryHistory).toHaveLength(1)
  })

  it('detects active rows and patches both snake_case and camelCase status fields', () => {
    const row: Record<string, any> = {
      policy_status: 'pending',
      pending_cmds: 1
    }

    expect(isActivePolicyStatus(row)).toBe(true)

    patchPolicyStatusRow(row, {
      policy_status: 'applied',
      policyStatus: 'applied',
      pending_cmds: 0,
      pendingCmds: 0,
      last_delivery_command_id: 'cmd-2',
      lastDeliveryCommandId: 'cmd-2'
    })

    expect(row.policy_status).toBe('applied')
    expect(row.policyStatus).toBe('applied')
    expect(row.pending_cmds).toBe(0)
    expect(row.pendingCmds).toBe(0)
    expect(row.last_delivery_command_id).toBe('cmd-2')
    expect(row.lastDeliveryCommandId).toBe('cmd-2')
    expect(isActivePolicyStatus(row)).toBe(false)
  })
})
