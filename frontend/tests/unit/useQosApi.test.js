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
      bandwidth_mbps: 200,
      direction: 'egress',
      rate_bps: 200000000,
      burst_bytes: 2500000,
      priority: 100,
      mode: 'auto',
      description: 'https limit',
      enabled: true
    })
  })

  it('应该允许显式创建 shaping 模式 QoS 规则', async () => {
    api.post.mockResolvedValue({
      data: { success: true, data: { id: 'rule-shaping' } }
    })

    await useQosApi.createQoSRule('node-1', {
      group_cidr: '10.0.0.0/24',
      direction: 'egress',
      bandwidth_mbps: 50,
      mode: 'shaping',
      description: 'smooth tcp'
    })

    expect(api.post).toHaveBeenCalledWith('/v2/tenants/tenant-1/nodes/node-1/qos', expect.objectContaining({
      mode: 'shaping',
      rate_bps: 50000000
    }))
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

  it('应该显示 inline IP Group 的 CIDR 成员而不是 inline 名称', async () => {
    api.get.mockResolvedValue({
      data: {
        success: true,
        data: [{
          id: 'qos-1',
          group_id: 'group-inline',
          group: {
            id: 'group-inline',
            name: 'inline-100-64-0-2',
            kind: 'inline',
            members: [{ cidr: '100.64.0.2/32' }]
          },
          direction: 'egress',
          bandwidth_mbps: 1,
          enabled: true
        }]
      }
    })

    const rules = await useQosApi.getQoSRulesByNode('node-1')

    expect(rules[0].runtime_group).toBe('100.64.0.2/32')
    expect(rules[0].group_cidr).toBe('100.64.0.2/32')
  })

  it('缺少 IP Group 元数据时不应把 group_id 当成用户可读名称', async () => {
    api.get.mockResolvedValue({
      data: {
        success: true,
        data: [{
          id: 'qos-1',
          group_id: '9bd2f0aa-08f5-4b7a-a743-1eddb8e1c955',
          direction: 'egress',
          bandwidth_mbps: 1,
          enabled: true
        }]
      }
    })

    const rules = await useQosApi.getQoSRulesByNode('node-1')

    expect(rules[0].runtime_group).toBe('未知IP组')
  })

  it('应该保留 QoS 列表里的投递状态、命令和失败原因', async () => {
    api.get.mockResolvedValue({
      data: {
        success: true,
        data: [{
          id: 'qos-1',
          group_id: 'group-1',
          group_name: 'branch-office',
          direction: 'egress',
          bandwidth_mbps: 10,
          enabled: true,
          policy_status: 'error',
          pending_cmds: 0,
          last_delivery_error: 'apply failed',
          last_delivery: {
            id: 'delivery-1',
            command_id: 'cmd-1',
            command_status: 'failed',
            last_error: 'apply failed'
          },
          delivery_history: [{
            id: 'delivery-1',
            command_id: 'cmd-1',
            command_status: 'failed',
            last_error: 'apply failed'
          }]
        }]
      }
    })

    const rules = await useQosApi.getQoSRulesByNode('node-1')

    expect(rules[0].policyStatus).toBe('error')
    expect(rules[0].policy_status).toBe('error')
    expect(rules[0].last_delivery_command_id).toBe('cmd-1')
    expect(rules[0].last_command_error).toBe('apply failed')
    expect(rules[0].delivery_history).toHaveLength(1)
  })

  it('应该把创建返回的 dispatch 归一化为待下发状态', async () => {
    api.post.mockResolvedValue({
      data: {
        success: true,
        data: {
          id: 'qos-1',
          bandwidth_mbps: 10,
          direction: 'egress',
          dispatch: {
            command_id: 'cmd-1',
            status: 'pending',
            desired_state_version: 'dsv-1',
            last_delivery: {
              id: 'delivery-1',
              command_id: 'cmd-1',
              command_status: 'pending',
              action: 'create'
            }
          },
          last_delivery: {
            id: 'delivery-1',
            command_id: 'cmd-1',
            command_status: 'pending',
            action: 'create'
          },
          delivery_history: [{
            id: 'delivery-1',
            command_id: 'cmd-1',
            command_status: 'pending',
            action: 'create'
          }]
        }
      }
    })

    const result = await useQosApi.createQoSRule('node-1', {
      group_cidr: '10.0.0.0/24',
      direction: 'egress',
      bandwidth_mbps: 10
    })

    expect(result.policyStatus).toBe('pending')
    expect(result.policy_status).toBe('pending')
    expect(result.pending_cmds).toBe(1)
    expect(result.last_delivery_command_id).toBe('cmd-1')
    expect(result.delivery_history).toHaveLength(1)
    expect(result.desired_state_version).toBe('dsv-1')
  })

  it('应该把 stale QoS 投递归一化为已过期且不计入待执行', async () => {
    api.get.mockResolvedValue({
      data: {
        success: true,
        data: [{
          id: 'qos-1',
          group_id: 'group-1',
          group_name: 'branch-office',
          direction: 'egress',
          bandwidth_mbps: 10,
          enabled: true,
          last_delivery: {
            id: 'delivery-old',
            command_id: 'cmd-old',
            command_status: 'stale',
            last_error: 'superseded by desired state dsv-new'
          },
          delivery_history: [{
            id: 'delivery-old',
            command_id: 'cmd-old',
            command_status: 'stale',
            last_error: 'superseded by desired state dsv-new'
          }]
        }]
      }
    })

    const rules = await useQosApi.getQoSRulesByNode('node-1')

    expect(rules[0].policy_status).toBe('stale')
    expect(rules[0].policyStatus).toBe('stale')
    expect(rules[0].pending_cmds).toBe(0)
    expect(rules[0].last_command_error).toContain('superseded')
  })

  it('应该按 QoS 规则生成策略重试请求', async () => {
    api.post.mockResolvedValue({
      data: {
        success: true,
        data: {
          policy_ref: 'qos-1',
          kind: 'qos',
          status: 'pending',
          last_delivery_command_id: 'cmd-retry'
        }
      }
    })

    const result = await useQosApi.retryQoSPolicySync('node-1', {
      id: 'qos-1',
      description: 'limit peer'
    })

    expect(api.post).toHaveBeenCalledWith('/v2/tenants/tenant-1/policies/retry', {
      node_id: 'node-1',
      kind: 'qos',
      policy_ref: 'qos-1',
      policy_name: 'limit peer'
    })
    expect(result.policyStatus).toBe('pending')
    expect(result.last_delivery_command_id).toBe('cmd-retry')
  })

  it('不应下发旧版 QoS protocol/port 匹配字段', async () => {
    api.post.mockResolvedValue({
      data: { success: true, data: { id: 'rule-1' } }
    })

    await useQosApi.createQoSRule('node-1', {
      group_cidr: '10.0.0.0/24',
      direction: 'egress',
      bandwidth_mbps: 10,
      protocol: 6,
      src_port: 1000,
      dst_port: 443
    })

    const payload = api.post.mock.calls[0][1]
    expect(payload).not.toHaveProperty('protocol')
    expect(payload).not.toHaveProperty('src_port')
    expect(payload).not.toHaveProperty('dst_port')
  })

  it('应该拒绝 0 Mbps 的 QoS 规则', async () => {
    await expect(useQosApi.createQoSRule('node-1', {
      group_cidr: '10.0.0.0/24',
      bandwidth_mbps: 0
    })).rejects.toThrow('bandwidth_mbps must be greater than 0')

    expect(api.post).not.toHaveBeenCalled()
  })
})
