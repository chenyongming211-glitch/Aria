import api from './useApi'
import { API_ENDPOINTS, requireCurrentTenantId } from '@/config/api'

/**
 * 令牌管理API接口
 * 与后端 /v2/tenants/{tenant_id}/tokens API 对接
 */
export const useTokenApi = {
  /**
   * 获取所有令牌
   * 返回格式: { success: true, data: [...], message: "X tokens retrieved" }
   */
  getAllTokens: async () => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.get(API_ENDPOINTS.TENANT.TOKENS(tenantId))
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
      const tenantId = requireCurrentTenantId()
      const payload = { ...params }
      if (payload.ttl_hours && !payload.ttl) {
        payload.ttl = `${payload.ttl_hours}h`
      }
      delete payload.ttl_hours

      const response = await api.post(API_ENDPOINTS.TENANT.TOKENS(tenantId), payload)
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
      const tenantId = requireCurrentTenantId()
      const response = await api.delete(API_ENDPOINTS.TENANT.TOKEN_DETAIL(tenantId, tokenId))
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
  getTokenDetail: async (tokenId) => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.get(API_ENDPOINTS.TENANT.TOKEN_DETAIL(tenantId, tokenId))
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
    return await useTokenApi.deleteToken(tokenId)
  },

  /**
   * 获取令牌的使用节点
   * @param {string} token - 令牌值
   */
  getTokenNodes: async (token) => {
    try {
      const tenantId = requireCurrentTenantId()
      const detail = await useTokenApi.getTokenDetail(token)
      if (!detail?.token) {
        return []
      }
      const response = await api.get(API_ENDPOINTS.TENANT.TOKENS(tenantId))
      const tokens = response.data?.data || response.data || []
      const current = tokens.find((item) => item.id === token)
      return current?.nodes || detail?.nodes || []
    } catch (error) {
      console.error('获取令牌使用节点失败:', error)
      throw error
    }
  }
}
