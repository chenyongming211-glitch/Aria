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

import api from '@/composables/useApi'

describe('useAclApi', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('getACLRules', () => {
    it('应该返回 ACL 规则列表', async () => {
      const mockRules = [
        { id: 1, name: 'allow-web', action: 'allow' },
        { id: 2, name: 'deny-ssh', action: 'deny' }
      ]
      
      api.get.mockResolvedValue({
        data: { success: true, data: mockRules }
      })
      
      const rules = await useAclApi.getACLRules()
      
      expect(api.get).toHaveBeenCalledWith('/v1/tenant-management/acl-rules')
      expect(rules).toEqual(mockRules)
    })

    it('应该支持过滤参数', async () => {
      const mockRules = [{ id: 1, name: 'allow-web', action: 'allow' }]
      
      api.get.mockResolvedValue({
        data: { success: true, data: mockRules }
      })
      
      const filters = { action: 'allow', enabled: true, page: 1, page_size: 10 }
      const rules = await useAclApi.getACLRules(filters)
      
      expect(api.get).toHaveBeenCalledWith(
        '/v1/tenant-management/acl-rules?action=allow&enabled=true&page=1&page_size=10'
      )
      expect(rules).toEqual(mockRules)
    })
  })

  describe('createACLRule', () => {
    it('应该创建新规则', async () => {
      const newRule = {
        name: 'test-rule',
        src_net: '192.168.1.0/24',
        dst_net: '10.0.0.0/24',
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
      
      expect(api.post).toHaveBeenCalledWith('/v1/tenant-management/acl-rules', newRule)
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
      
      const result = await useAclApi.updateACLRule(1, updatedRule)
      
      expect(api.put).toHaveBeenCalledWith('/v1/tenant-management/acl-rules/1', updatedRule)
      expect(result).toEqual(updatedRule)
    })
  })

  describe('deleteACLRule', () => {
    it('应该删除规则', async () => {
      api.delete.mockResolvedValue({
        data: { success: true, message: 'Rule deleted' }
      })
      
      const result = await useAclApi.deleteACLRule(1)
      
      expect(api.delete).toHaveBeenCalledWith('/v1/tenant-management/acl-rules/1')
      expect(result).toEqual({ success: true, message: 'Rule deleted' })
    })
  })
})
