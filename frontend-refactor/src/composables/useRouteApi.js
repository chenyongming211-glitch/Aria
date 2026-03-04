import api from './useApi'
import { API_ENDPOINTS } from '@/config/api'

/**
 * 路由管理 API
 */
export const useRouteApi = {
  /**
   * 获取所有节点的路由信息
   * @returns {Promise<Array>} 节点路由列表
   */
  getRoutes: async () => {
    try {
      const response = await api.get(API_ENDPOINTS.TENANT.NODES)
      
      let nodeData = []
      const result = response.data
      
      // 解析响应格式
      if (Array.isArray(result)) {
        nodeData = result
      } else if (result && Array.isArray(result.data)) {
        nodeData = result.data
      } else if (result && result.success && Array.isArray(result.data)) {
        nodeData = result.data
      }
      
      // 解析路由信息
      return nodeData.map(node => ({
        id: node.id,
        hostname: node.hostname || 'unknown',
        publicIp: node.public_ip || 'N/A',
        region: node.region || 'unknown',
        routes: parseAdvertisedRoutes(node.advertised_routes),
        rawRoutes: node.advertised_routes // 保留原始数据用于编辑
      }))
    } catch (error) {
      console.error('[RouteApi] 获取路由信息失败:', error)
      throw error
    }
  },

  /**
   * 更新节点的路由信息
   * @param {string} nodeId - 节点 ID
   * @param {Array} routes - 路由数组
   * @returns {Promise<Object>} 更新结果
   */
  updateRoutes: async (nodeId, routes) => {
    try {
      // 将路由数组编码为 base64
      const routesString = `{${routes.join(',')}}`
      const advertisedRoutes = btoa(routesString)
      
      console.log('[RouteApi] 更新路由:', { nodeId, routes, advertisedRoutes })
      
      const response = await api.put(`/v1/tenant-management/nodes/${nodeId}`, {
        advertised_routes: advertisedRoutes
      })
      
      console.log('[RouteApi] 更新成功:', response.data)
      return response.data?.data || response.data
    } catch (error) {
      console.error('[RouteApi] 更新路由失败:', error)
      throw error
    }
  },

  /**
   * 添加路由到节点
   * @param {string} nodeId - 节点 ID
   * @param {string} newRoute - 新路由（CIDR 格式）
   * @param {Array} currentRoutes - 当前路由列表
   * @returns {Promise<Object>} 更新结果
   */
  addRoute: async (nodeId, newRoute, currentRoutes) => {
    // 检查路由是否已存在
    if (currentRoutes.includes(newRoute)) {
      throw new Error('路由已存在')
    }
    
    // 添加新路由
    const updatedRoutes = [...currentRoutes, newRoute]
    return await useRouteApi.updateRoutes(nodeId, updatedRoutes)
  },

  /**
   * 从节点删除路由
   * @param {string} nodeId - 节点 ID
   * @param {string} routeToDelete - 要删除的路由
   * @param {Array} currentRoutes - 当前路由列表
   * @returns {Promise<Object>} 更新结果
   */
  deleteRoute: async (nodeId, routeToDelete, currentRoutes) => {
    // 过滤掉要删除的路由
    const updatedRoutes = currentRoutes.filter(route => route !== routeToDelete)
    return await useRouteApi.updateRoutes(nodeId, updatedRoutes)
  }
}

/**
 * 解析 advertised_routes（base64 编码的路由信息）
 * @param {string} advertisedRoutes - base64 编码的路由字符串
 * @returns {Array} 路由数组
 */
function parseAdvertisedRoutes(advertisedRoutes) {
  if (!advertisedRoutes) return []
  
  try {
    // 解码 base64
    const decoded = atob(advertisedRoutes)
    
    // 解析格式：{2.2.2.0/24,3.3.3.0/24}
    const match = decoded.match(/\{(.+)\}/)
    if (match && match[1]) {
      return match[1].split(',').map(route => route.trim())
    }
    
    return []
  } catch (error) {
    console.error('[RouteApi] 解析路由失败:', error)
    return []
  }
}
