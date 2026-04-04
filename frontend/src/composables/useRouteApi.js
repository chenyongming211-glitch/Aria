import api from './useApi'
import { API_ENDPOINTS, requireCurrentTenantId } from '@/config/api'

function normalizeRoute(node, route) {
  return {
    id: route.id || route.cidr,
    cidr: route.cidr || route.id,
    nodeId: node.id,
    nodeName: node.hostname || node.public_key || node.id,
    hostname: node.hostname || 'unknown',
    publicIp: node.public_ip || 'N/A',
    region: node.region || 'unknown',
    policyStatus: route.policy_status || node.configuration_status || 'idle',
    pendingCmds: typeof route.pending_cmds === 'number' ? route.pending_cmds : (node.pending_cmds || 0),
    lastCommandError: route.last_delivery_error || node.last_command_error || '',
    lastSyncAt: route.last_delivery_at || node.last_sync_at || null,
    lastDelivery: route.last_delivery || null,
    deliveryHistory: Array.isArray(route.delivery_history) ? route.delivery_history : [],
    lastDeliveryCommandId: route.last_delivery_command_id || route.last_delivery?.command_id || '',
    lastDeliveryAction: route.last_delivery_action || route.last_delivery?.action || ''
  }
}

async function listTenantNodes() {
  const tenantId = requireCurrentTenantId()
  const response = await api.get(API_ENDPOINTS.TENANT.NODES(tenantId))
  return { tenantId, nodes: response.data?.data || response.data || [] }
}

/**
 * 路由管理 API
 */
export const useRouteApi = {
  getRoutes: async () => {
    try {
      const { tenantId, nodes } = await listTenantNodes()
      const routeGroups = await Promise.all(
        nodes.map(async (node) => {
          const response = await api.get(API_ENDPOINTS.TENANT.NODE_ROUTES(tenantId, node.id))
          const routes = response.data?.data || response.data || []
          return routes.map((route) => normalizeRoute(node, route))
        })
      )

      return routeGroups.flat()
    } catch (error) {
      console.error('[RouteApi] 获取路由信息失败:', error)
      throw error
    }
  },

  addRoute: async (nodeId, cidr) => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.post(API_ENDPOINTS.TENANT.NODE_ROUTES(tenantId, nodeId), { cidr })
      return response.data?.data || response.data
    } catch (error) {
      console.error('[RouteApi] 添加路由失败:', error)
      throw error
    }
  },

  updateRoute: async (nodeId, routeId, cidr) => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.put(API_ENDPOINTS.TENANT.NODE_ROUTE(tenantId, nodeId, routeId), { cidr })
      return response.data?.data || response.data
    } catch (error) {
      console.error('[RouteApi] 更新路由失败:', error)
      throw error
    }
  },

  deleteRoute: async (nodeId, routeId) => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.delete(API_ENDPOINTS.TENANT.NODE_ROUTE(tenantId, nodeId, routeId))
      return response.data?.data || response.data
    } catch (error) {
      console.error('[RouteApi] 删除路由失败:', error)
      throw error
    }
  }
}
