import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useQosApi } from '@/composables/useQosApi'

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

describe('useQosApi', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  it('应该把空 QoS 规则响应规范化为空数组', async () => {
    api.get.mockResolvedValue({
      data: { success: true, data: null }
    })

    const rules = await useQosApi.getQoSRulesByNode('node-1')

    expect(api.get).toHaveBeenCalledWith('/v2/tenants/tenant-1/nodes/node-1/qos')
    expect(rules).toEqual([])
  })

  it('应该创建 QoS 规则并发送标准化带宽字段', async () => {
    api.post.mockResolvedValue({
      data: { success: true, data: { id: 'rule-1' } }
    })

    await useQosApi.createQoSRule('node-1', {
      group_cidr: '10.0.0.0/24',
      direction: 'egress',
      bandwidth_mbps: 200,
      description: 'https limit'
    })

    expect(api.post).toHaveBeenCalledWith('/v2/tenants/tenant-1/nodes/node-1/qos', {
      src_cidr: '',
      dst_cidr: '10.0.0.0/24',
      src_port: 0,
      dst_port: 0,
      protocol: 0,
      bandwidth_mbps: 200,
      direction: 'egress',
      rate_bps: 200000000,
      burst_bytes: 2500000,
      priority: 100,
      mode: 'policing',
      description: 'https limit',
      enabled: true
    })
  })

  it('应该优先发送 IP Group ID 并清空直接 CIDR', async () => {
    api.post.mockResolvedValue({
      data: { success: true, data: { id: 'rule-1' } }
    })

    await useQosApi.createQoSRule('node-1', {
      group_id: 'group-1',
      group_cidr: '10.0.0.0/24',
      direction: 'egress',
      bandwidth_mbps: 10,
      priority: 20,
      description: 'group limit'
    })

    expect(api.post).toHaveBeenCalledWith('/v2/tenants/tenant-1/nodes/node-1/qos', expect.objectContaining({
      group_id: 'group-1',
      src_cidr: '',
      dst_cidr: '',
      priority: 20
    }))
  })

  it('应该拒绝 0 Mbps 的 QoS 规则', async () => {
    await expect(useQosApi.createQoSRule('node-1', {
      group_cidr: '10.0.0.0/24',
      bandwidth_mbps: 0
    })).rejects.toThrow('bandwidth_mbps must be greater than 0')

    expect(api.post).not.toHaveBeenCalled()
  })
})
