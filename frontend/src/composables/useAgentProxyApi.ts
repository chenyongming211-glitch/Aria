import api from './useApi'
import { unwrapApiData } from './apiResponse'
import { API_ENDPOINTS, requireCurrentTenantId } from '@/config/api'
import type { AgentCommandPayload, AgentCommandRecord, AgentStatus } from '@/types'

interface BatchAgentCommandPayload {
  node_ids?: string[]
  command?: AgentCommandPayload
  payload?: Record<string, unknown>
}

type AgentCommandListResult = { items?: AgentCommandRecord[] } | AgentCommandRecord[]

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
  sendAgentCommand: async (nodeId: string, command: AgentCommandPayload): Promise<AgentCommandRecord> => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.post(API_ENDPOINTS.AGENT.COMMAND(tenantId, nodeId), command)
      return unwrapApiData<AgentCommandRecord>(response)
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
  getAgentStatus: async (nodeId: string): Promise<AgentStatus> => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.get(API_ENDPOINTS.AGENT.STATUS(tenantId, nodeId))
      return unwrapApiData<AgentStatus>(response)
    } catch (error) {
      console.error('获取 Agent 状态失败:', error)
      throw error
    }
  },

  getAgentCommands: async (nodeId: string, limit = 10): Promise<AgentCommandListResult> => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.get(API_ENDPOINTS.AGENT.COMMANDS(tenantId, nodeId), {
        params: { limit }
      })
      return unwrapApiData<AgentCommandListResult>(response)
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
  sendBatchCommand: async (batchCommand: BatchAgentCommandPayload): Promise<unknown> => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.post(API_ENDPOINTS.AGENT.BATCH_COMMAND(tenantId), batchCommand)
      return unwrapApiData<unknown>(response)
    } catch (error) {
      console.error('批量发送命令失败:', error)
      throw error
    }
  }
}
