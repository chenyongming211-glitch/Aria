import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useRouteApi } from '@/composables/useRouteApi'

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

describe('useRouteApi', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('应该从 route 最近投递状态推导策略状态和待执行数量', async () => {
    api.get
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: [{
            id: 'node-1',
            hostname: 'node-1',
            public_ip: '203.0.113.10',
            region: 'sh',
            configuration_status: 'pending',
            pending_cmds: 2
          }]
        }
      })
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: [{
            id: 'route-1',
            cidr: '10.10.0.0/24',
            last_delivery: {
              id: 'delivery-1',
              command_id: 'cmd-1',
              command_status: 'completed',
              action: 'update'
            },
            delivery_history: [{
              id: 'delivery-1',
              command_id: 'cmd-1',
              command_status: 'completed',
              action: 'update'
            }]
          }]
        }
      })

    const result = await useRouteApi.getRoutes()

    expect(result[0].policyStatus).toBe('applied')
    expect(result[0].pendingCmds).toBe(0)
    expect(result[0].lastDeliveryCommandId).toBe('cmd-1')
  })

  it('应该保留 route 投递失败原因用于页面展示', async () => {
    api.get
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: [{ id: 'node-1', hostname: 'node-1' }]
        }
      })
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: [{
            id: 'route-1',
            cidr: '10.10.0.0/24',
            last_delivery: {
              id: 'delivery-1',
              command_id: 'cmd-1',
              command_status: 'failed',
              last_error: 'apply failed'
            }
          }]
        }
      })

    const result = await useRouteApi.getRoutes()

    expect(result[0].policyStatus).toBe('error')
    expect(result[0].pendingCmds).toBe(0)
    expect(result[0].lastCommandError).toBe('apply failed')
  })
})
