import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useAclApi } from '@/composables/useAclApi'

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
    requireCurrentTenantId: vi.fn(() => 'tenant-1')
  }
})

import api from '@/composables/useApi'

describe('useAclApi', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('getACLRules', () => {
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
        action: 'allow',
        enabled: true,
        priority: 100,
        description: ''
      })
      expect(result).toEqual(createdRule)
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
        action: 'deny',
        enabled: true,
        priority: 100,
        description: ''
      })
      expect(result).toEqual(updatedRule)
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
