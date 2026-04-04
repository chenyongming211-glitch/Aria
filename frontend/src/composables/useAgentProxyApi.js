import api from './useApi'
import { API_ENDPOINTS, requireCurrentTenantId } from '@/config/api'

/**
 * Agent 代理 API
 * 用于向 Agent 节点发送命令和查询状态
 */
export const useAgentProxyApi = {
  /**
   * 发送命令到单个 Agent
   * @param {string} nodeId - 节点 ID（public_key 或 hostname）
   * @param {Object} command - 命令内容
   * @returns {Promise<Object>} 命令响应
   */
  sendAgentCommand: async (nodeId, command) => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.post(API_ENDPOINTS.AGENT.COMMAND(tenantId, nodeId), command)
      return response.data?.data || response.data
    } catch (error) {
      console.error('发送 Agent 命令失败:', error)
      throw error
    }
  },

  /**
   * 获取 Agent 状态
   * @param {string} nodeId - 节点 ID
   * @returns {Promise<Object>} Agent 状态
   */
  getAgentStatus: async (nodeId) => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.get(API_ENDPOINTS.AGENT.STATUS(tenantId, nodeId))
      return response.data?.data || response.data
    } catch (error) {
      console.error('获取 Agent 状态失败:', error)
      throw error
    }
  },

  getAgentCommands: async (nodeId, limit = 10) => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.get(API_ENDPOINTS.AGENT.COMMANDS(tenantId, nodeId), {
        params: { limit }
      })
      return response.data?.data || response.data
    } catch (error) {
      console.error('获取 Agent 命令历史失败:', error)
      throw error
    }
  },

  /**
   * 批量发送命令到多个 Agent
   * @param {Object} batchCommand - 批量命令
   * @returns {Promise<Object>} 批量命令响应
   */
  sendBatchCommand: async (batchCommand) => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.post(API_ENDPOINTS.AGENT.BATCH_COMMAND(tenantId), batchCommand)
      return response.data?.data || response.data
    } catch (error) {
      console.error('批量发送命令失败:', error)
      throw error
    }
  }
}
