import api from './useApi'
import { API_ENDPOINTS } from '@/config/api'

/**
 * AI聊天API接口
 * 与后端 /v1/ai/* API 对接
 */
export const useAiApi = {
  /**
   * AI 对话
   * @param {Object} params - 对话参数
   *  {
   *    message: string,      // 消息内容（必填）
   *    session_id?: string,  // 会话ID（可选，用于上下文）
   *    tools?: boolean       // 是否使用工具（可选）
   *  }
   * 返回格式: { success: true, data: { session_id, reply, card_data, tool_calls, needs_confirm }, message: "..." }
   */
  chat: async (params) => {
    try {
      const response = await api.post(API_ENDPOINTS.AI.CHAT, params)
      return response.data?.data || response.data
    } catch (error) {
      console.error('AI对话失败:', error)
      throw error
    }
  },

  /**
   * 确认工具执行
   * @param {Object} params - 确认参数
   *  {
   *    session_id: string,      // 会话ID（必填）
   *    tool_name: string,       // 工具名称（必填）
   *    params: object,         // 工具参数（必填）
   *    confirmed: boolean      // 是否确认（必填）
   *  }
   * 返回格式: { success: true, data: { session_id, result }, message: "..." }
   */
  confirm: async (params) => {
    try {
      const response = await api.post(API_ENDPOINTS.AI.CONFIRM, params)
      return response.data?.data || response.data
    } catch (error) {
      console.error('工具执行确认失败:', error)
      throw error
    }
  }
}
