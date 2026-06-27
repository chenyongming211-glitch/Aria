import api from './useApi'
import { API_ENDPOINTS, requireCurrentTenantId } from '@/config/api'
import {
  mapCommandStatusToPolicyStatus,
  pendingCountForCommandStatus
} from '@/utils/controlLoopStatus'

function normalizeRoute(node, route) {
  const lastDelivery = route.last_delivery || null
  const deliveryHistory = Array.isArray(route.delivery_history)
    ? route.delivery_history
    : (lastDelivery ? [lastDelivery] : [])
  const deliveryStatus = lastDelivery?.command_status || route.dispatch?.status || ''
  const mappedDeliveryStatus = mapCommandStatusToPolicyStatus(deliveryStatus)
  const pendingCmds = typeof route.pending_cmds === 'number'
    ? route.pending_cmds
    : (deliveryHistory.length > 0
        ? deliveryHistory.reduce((total, delivery) => total + pendingCountForCommandStatus(delivery?.command_status), 0)
        : (mappedDeliveryStatus ? pendingCountForCommandStatus(deliveryStatus) : (node.pending_cmds || 0)))
  const lastCommandError = route.last_delivery_error ||
    lastDelivery?.last_error ||
    route.last_command_error ||
    node.last_command_error ||
    ''

  return {
    id: route.id || route.cidr,
    cidr: route.cidr || route.id,
    policyRef: route.policy_ref || route.cidr || route.id,
    policy_ref: route.policy_ref || route.cidr || route.id,
    nodeId: node.id,
    nodeName: node.hostname || node.public_key || node.id,
    hostname: node.hostname || 'unknown',
    publicIp: node.public_ip || 'N/A',
    region: node.region || 'unknown',
    policyStatus: route.policy_status || mappedDeliveryStatus || node.configuration_status || 'idle',
    pendingCmds,
    lastCommandError,
    lastSyncAt: route.last_delivery_at || node.last_sync_at || null,
    lastDelivery,
    deliveryHistory,
    lastDeliveryCommandId: route.last_delivery_command_id || lastDelivery?.command_id || '',
    lastDeliveryAction: route.last_delivery_action || lastDelivery?.action || ''
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
