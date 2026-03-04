import api from './useApi'
import { API_ENDPOINTS } from '@/config/api'

/**
 * 令牌管理API接口
 * 与后端 /tokens API 对接（注意：后端端点是 /tokens 不是 /v1/tokens）
 */
export const useTokenApi = {
  /**
   * 获取所有令牌
   * 返回格式: { success: true, data: [...], message: "X tokens retrieved" }
   */
  getAllTokens: async () => {
    try {
      const response = await api.get(API_ENDPOINTS.TOKENS.LIST)
      // 后端返回的统一响应格式: { success, data, message }
      return response.data?.data || response.data || []
    } catch (error) {
      console.error('获取令牌列表失败:', error)
      throw error
    }
  },

  /**
   * 创建新令牌
   * @param {Object} params - 创建令牌参数
   *  {
   *    tag: string,           // 令牌标签（必填）
   *    max_uses: number,      // 最大使用次数（必填，默认1）
   *    ttl: string            // 有效期 "1h", "24h", "7d", "30d"（可选）
   *  }
   */
  createToken: async (params) => {
    try {
      const response = await api.post(API_ENDPOINTS.TOKENS.CREATE, params)
      // 后端返回: { success: true, data: { id, token, ... }, message: "..." }
      return response.data?.data || response.data
    } catch (error) {
      console.error('创建令牌失败:', error)
      throw error
    }
  },

  /**
   * 删除/吊销令牌（新 API）
   * @param {string} tokenId - 令牌ID
   */
  deleteToken: async (tokenId) => {
    try {
      const response = await api.delete(API_ENDPOINTS.TOKENS.DELETE(tokenId))
      return response.data?.data || response.data
    } catch (error) {
      console.error('删除令牌失败:', error)
      throw error
    }
  },

  /**
   * 获取令牌详情（查询参数方式）
   * @param {string} token - 令牌值
   */
  getTokenDetail: async (token) => {
    try {
      const response = await api.get(`${API_ENDPOINTS.TOKENS.DETAIL}?token=${encodeURIComponent(token)}`)
      return response.data?.data || response.data
    } catch (error) {
      console.error('获取令牌详情失败:', error)
      throw error
    }
  },

  // ========== 旧版兼容方法 ==========

  /**
   * 吊销令牌（旧版兼容）
   * @param {string} tokenId - 令牌ID
   */
  revokeToken: async (tokenId) => {
    console.warn('revokeToken 已弃用，请使用 deleteToken')
    // 尝试使用新版 API
    try {
      return await useTokenApi.deleteToken(tokenId)
    } catch (error) {
      // 如果新版失败，尝试旧版
      try {
        const response = await api.post(API_ENDPOINTS.TOKENS.REVOKE, { id: tokenId })
        return response.data?.data || response.data
      } catch (oldError) {
        console.error('吊销令牌失败（旧版）:', oldError)
        throw error
      }
    }
  },

  /**
   * 获取令牌的使用节点
   * @param {string} token - 令牌值
   */
  getTokenNodes: async (token) => {
    try {
      const response = await api.get(`/tokens/detail?token=${encodeURIComponent(token)}`)
      return response.data?.data?.nodes || []
    } catch (error) {
      console.error('获取令牌使用节点失败:', error)
      throw error
    }
  }
}
