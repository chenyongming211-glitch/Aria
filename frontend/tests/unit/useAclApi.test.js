import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useAclApi } from '@/composables/useAclApi'

const currentTenantId = vi.hoisted(() => ({ value: 'tenant-1' }))

// Mock axios
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
    requireCurrentTenantId: vi.fn(() => currentTenantId.value)
  }
})

import api from '@/composables/useApi'

describe('useAclApi', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    currentTenantId.value = 'tenant-1'
  })

  describe('getACLRules', () => {
    it('应该把空 ACL 规则响应规范化为空数组', async () => {
      api.get.mockResolvedValue({
        data: { success: true, data: null }
      })

      const rules = await useAclApi.getACLRulesByNode('node-1')

      expect(api.get).toHaveBeenCalledWith('/v2/tenants/tenant-1/nodes/node-1/security/acls')
      expect(rules).toEqual([])
    })

    it('应该返回 ACL 规则列表', async () => {
      const mockRules = [
        { id: 1, name: 'allow-web', action: 'allow', src_cidr: '10.0.0.0/24', dst_cidr: '0.0.0.0/0', dst_port: 443 },
        { id: 2, name: 'deny-ssh', action: 'deny', src_cidr: '10.0.1.0/24', dst_cidr: '0.0.0.0/0', dst_port: 22 }
      ]
      
      api.get.mockResolvedValue({
        data: { success: true, data: mockRules }
      })
      
      const rules = await useAclApi.getACLRulesByNode('node-1')
      
      expect(api.get).toHaveBeenCalledWith('/v2/tenants/tenant-1/nodes/node-1/security/acls')
      expect(rules).toHaveLength(2)
      expect(rules[0]).toMatchObject({ ...mockRules[0], node_id: 'node-1', src_net: '10.0.0.0/24', dst_net: '0.0.0.0/0', min_port: 443, max_port: 443 })
      expect(rules[1]).toMatchObject({ ...mockRules[1], node_id: 'node-1', src_net: '10.0.1.0/24', dst_net: '0.0.0.0/0', min_port: 22, max_port: 22 })
    })

    it('应该支持过滤参数', async () => {
      const mockRules = [
        { id: 1, name: 'allow-web', action: 'allow', node_id: 'node-1' },
        { id: 2, name: 'deny-ssh', action: 'deny', node_id: 'node-1' }
      ]
      
      api.get.mockResolvedValue({
        data: { success: true, data: mockRules }
      })
      
      const filters = { action: 'allow' }
      const rules = await useAclApi.getACLRulesByNode('node-1', filters)
      
      expect(api.get).toHaveBeenCalledWith('/v2/tenants/tenant-1/nodes/node-1/security/acls')
      expect(rules).toHaveLength(1)
      expect(rules[0]).toMatchObject({ id: 1, name: 'allow-web', action: 'allow', node_id: 'node-1' })
    })

    it('缺少 IP Group 元数据时不应把 group id 当成用户可读名称', async () => {
      api.get.mockResolvedValue({
        data: {
          success: true,
          data: [{
            id: 99,
            name: 'group-acl',
            action: 'deny',
            src_group_id: 'src-group-id',
            dst_group_id: 'dst-group-id',
            direction: 'ingress'
          }]
        }
      })

      const rules = await useAclApi.getACLRulesByNode('node-1')

      expect(rules[0].runtime_src_group).toBe('未知 IP Group')
      expect(rules[0].runtime_dst_group).toBe('未知 IP Group')
    })

  })

  describe('createACLRule', () => {
    it('应该创建新规则', async () => {
      const newRule = {
        node_id: 'node-1',
        name: 'test-rule',
        src_cidr: '192.168.1.0/24',
        dst_cidr: '10.0.0.0/24',
        dst_port: 443,
        protocol: 6,
        action: 'allow',
        enabled: true,
        priority: 100
      }
      
      const createdRule = { id: 1, ...newRule }
      
      api.post.mockResolvedValue({
        data: { success: true, data: createdRule }
      })
      
      const result = await useAclApi.createACLRule(newRule)
      
      expect(api.post).toHaveBeenCalledWith('/v2/tenants/tenant-1/nodes/node-1/security/acls', {
        name: 'test-rule',
        src_cidr: '192.168.1.0/24',
        dst_cidr: '10.0.0.0/24',
        protocol: 6,
        dst_port: 443,
        direction: 'ingress',
        ports: '443',
        action: 'allow',
        enabled: true,
        priority: 100,
        description: ''
      })
      expect(result).toEqual(createdRule)
    })

    it('应该优先发送源/目标 IP Group ID 并清空对应 CIDR', async () => {
      api.post.mockResolvedValue({
        data: { success: true, data: { id: 2 } }
      })

      await useAclApi.createACLRule({
        node_id: 'node-1',
        name: 'group-acl',
        src_group_id: 'src-group',
        dst_group_id: 'dst-group',
        src_cidr: '192.168.1.0/24',
        dst_cidr: '10.0.0.0/24',
        action: 'deny',
        direction: 'egress'
      })

      expect(api.post).toHaveBeenCalledWith('/v2/tenants/tenant-1/nodes/node-1/security/acls', expect.objectContaining({
        src_group_id: 'src-group',
        dst_group_id: 'dst-group',
        src_cidr: '',
        dst_cidr: '',
        action: 'deny',
        direction: 'egress'
      }))
    })

    it('应该把创建返回的 dispatch 归一化为待下发状态', async () => {
      api.post.mockResolvedValue({
        data: {
          success: true,
          data: {
            id: 'acl-1',
            name: 'allow-icmp',
            action: 'allow',
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

      const result = await useAclApi.createACLRule({
        node_id: 'node-1',
        name: 'allow-icmp',
        action: 'allow',
        protocol: 1,
        direction: 'egress'
      })

      expect(result.policy_status).toBe('pending')
      expect(result.pending_cmds).toBe(1)
      expect(result.last_delivery_command_id).toBe('cmd-1')
      expect(result.delivery_history).toHaveLength(1)
      expect(result.desired_state_version).toBe('dsv-1')
    })
  })

  describe('updateACLRule', () => {
    it('应该更新规则', async () => {
      const updatedRule = {
        id: 1,
        name: 'updated-rule',
        action: 'deny'
      }
      
      api.put.mockResolvedValue({
        data: { success: true, data: updatedRule }
      })
      
      const result = await useAclApi.updateACLRule(1, { ...updatedRule, node_id: 'node-1' })
      
      expect(api.put).toHaveBeenCalledWith('/v2/tenants/tenant-1/nodes/node-1/security/acls/1', {
        name: 'updated-rule',
        src_cidr: '',
        dst_cidr: '',
        protocol: 0,
        dst_port: 0,
        direction: 'ingress',
        ports: '',
        action: 'deny',
        enabled: true,
        priority: 100,
        description: ''
      })
      expect(result).toEqual(updatedRule)
    })

    it('不会跨租户复用同 ID 规则的节点映射', async () => {
      api.get.mockResolvedValueOnce({
        data: {
          success: true,
          data: [
            { id: 1, name: 'tenant-1-rule', action: 'allow', src_cidr: '10.0.0.0/24', dst_cidr: '0.0.0.0/0' }
          ]
        }
      })
      api.put.mockResolvedValue({
        data: { success: true, data: { id: 1, name: 'updated-rule' } }
      })

      await useAclApi.getACLRulesByNode('node-tenant-1')
      currentTenantId.value = 'tenant-2'

      await expect(useAclApi.updateACLRule(1, { name: 'updated-rule' })).rejects.toThrow('node_id is required')

      await useAclApi.updateACLRule(1, { name: 'updated-rule', node_id: 'node-tenant-2' })

      expect(api.put).toHaveBeenCalledWith('/v2/tenants/tenant-2/nodes/node-tenant-2/security/acls/1', expect.any(Object))
    })
  })

  describe('deleteACLRule', () => {
    it('应该删除规则', async () => {
      api.delete.mockResolvedValue({
        data: { success: true, message: 'Rule deleted' }
      })
      
      const result = await useAclApi.deleteACLRule(1, 'node-1')
      
      expect(api.delete).toHaveBeenCalledWith('/v2/tenants/tenant-1/nodes/node-1/security/acls/1')
      expect(result).toEqual({ success: true, message: 'Rule deleted' })
    })
  })
})
