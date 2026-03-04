import api from './useApi'

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
      const response = await api.post(`/v1/agent/${nodeId}/command`, command)
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
      const response = await api.get(`/v1/agent/${nodeId}/status`)
      return response.data?.data || response.data
    } catch (error) {
      console.error('获取 Agent 状态失败:', error)
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
      const response = await api.post('/v1/agents/command', batchCommand)
      return response.data?.data || response.data
    } catch (error) {
      console.error('批量发送命令失败:', error)
      throw error
    }
  }
}
