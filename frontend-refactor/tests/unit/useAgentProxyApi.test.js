import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useAgentProxyApi } from '@/composables/useAgentProxyApi'

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

describe('useAgentProxyApi', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('sendAgentCommand', () => {
    it('应该发送命令到单个节点', async () => {
      const command = {
        command: 'sync',
        timeout: 30,
        priority: 0
      }
      
      const mockResponse = {
        command_id: 'cmd-123',
        status: 'pending'
      }
      
      api.post.mockResolvedValue({
        data: { success: true, data: mockResponse }
      })
      
      const result = await useAgentProxyApi.sendAgentCommand('node-1', command)
      
      expect(api.post).toHaveBeenCalledWith('/v1/agent/node-1/command', command)
      expect(result).toEqual(mockResponse)
    })
  })

  describe('getAgentStatus', () => {
    it('应该获取节点状态', async () => {
      const mockStatus = {
        node_id: 'node-1',
        hostname: 'aria-sh-1',
        status: 'online',
        version: '0.2.26'
      }
      
      api.get.mockResolvedValue({
        data: { success: true, data: mockStatus }
      })
      
      const result = await useAgentProxyApi.getAgentStatus('node-1')
      
      expect(api.get).toHaveBeenCalledWith('/v1/agent/node-1/status')
      expect(result).toEqual(mockStatus)
    })
  })

  describe('sendBatchCommand', () => {
    it('应该批量发送命令', async () => {
      const batchCommand = {
        node_ids: ['node-1', 'node-2'],
        command: {
          command: 'sync',
          timeout: 30
        }
      }
      
      const mockResponse = {
        total: 2,
        success: 2,
        failed: 0
      }
      
      api.post.mockResolvedValue({
        data: { success: true, data: mockResponse }
      })
      
      const result = await useAgentProxyApi.sendBatchCommand(batchCommand)
      
      expect(api.post).toHaveBeenCalledWith('/v1/agents/command', batchCommand)
      expect(result).toEqual(mockResponse)
    })
  })
})
