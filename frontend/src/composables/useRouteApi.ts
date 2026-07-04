import api from './useApi'
import { unwrapApiList } from './apiResponse'
import { API_ENDPOINTS, requireCurrentTenantId } from '@/config/api'
import {
  mapCommandStatusToPolicyStatus,
  pendingCountForCommandStatus
} from '@/utils/controlLoopStatus'
import type { CommandId, NodeId, PolicyId } from '@/types/api'
import type { NodeRecord } from '@/types/node'
import type { DispatchResult, PolicyDelivery, PolicyStatus } from '@/types/policy'

type RouteNode = NodeRecord & {
  public_key?: string
  pending_cmds?: number
  last_command_error?: string
  configuration_status?: PolicyStatus | string
}

interface RouteRecord {
  id?: PolicyId | string
  cidr?: string
  policy_ref?: string
  policy_status?: PolicyStatus | string
  pending_cmds?: number
  dispatch?: DispatchResult
  last_delivery?: PolicyDelivery | null
  delivery_history?: PolicyDelivery[]
  last_delivery_error?: string
  last_command_error?: string
  last_delivery_at?: string | null
  last_delivery_command_id?: CommandId | string
  last_delivery_action?: string
}

export interface NormalizedRoute {
  id: string
  cidr: string
  policyRef: string
  policy_ref: string
  nodeId: NodeId
  nodeName: string
  hostname: string
  publicIp: string
  region: string
  policyStatus: PolicyStatus | string
  pendingCmds: number
  lastCommandError: string
  lastSyncAt: string | null
  lastDelivery: PolicyDelivery | null
  deliveryHistory: PolicyDelivery[]
  lastDeliveryCommandId: CommandId | string
  lastDeliveryAction: string
}

function normalizeRoute(node: RouteNode, route: RouteRecord): NormalizedRoute {
  const lastDelivery = route.last_delivery || null
  const deliveryHistory = Array.isArray(route.delivery_history)
    ? route.delivery_history
    : (lastDelivery ? [lastDelivery] : [])
  const deliveryStatus = lastDelivery?.command_status || route.dispatch?.status || ''
  const mappedDeliveryStatus = mapCommandStatusToPolicyStatus(deliveryStatus)
  const pendingCmds = typeof route.pending_cmds === 'number'
    ? route.pending_cmds
    : (deliveryHistory.length > 0
        ? deliveryHistory.reduce((total: number, delivery: PolicyDelivery) => total + pendingCountForCommandStatus(delivery?.command_status), 0)
        : (mappedDeliveryStatus ? pendingCountForCommandStatus(deliveryStatus) : (node.pending_cmds || 0)))
  const lastCommandError = route.last_delivery_error ||
    lastDelivery?.last_error ||
    route.last_command_error ||
    node.last_command_error ||
    ''
  const routeRef = route.policy_ref || route.cidr || route.id || ''

  return {
    id: route.id || route.cidr || routeRef,
    cidr: route.cidr || route.id || routeRef,
    policyRef: routeRef,
    policy_ref: routeRef,
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

function normalizeNodeList(response: { data?: unknown }): RouteNode[] {
  return unwrapApiList<RouteNode>(response)
}

function normalizeRouteList(response: { data?: unknown }): RouteRecord[] {
  return unwrapApiList<RouteRecord>(response)
}

async function listTenantNodes(): Promise<{ tenantId: string, nodes: RouteNode[] }> {
  const tenantId = requireCurrentTenantId()
  const response = await api.get(API_ENDPOINTS.TENANT.NODES(tenantId))
  return { tenantId, nodes: normalizeNodeList(response) }
}

/**
 * 路由管理 API
 */
export const useRouteApi = {
  getRoutes: async () => {
    try {
      const { tenantId, nodes } = await listTenantNodes()
      const routeGroups = await Promise.all(
        nodes.map(async (node: RouteNode) => {
          const response = await api.get(API_ENDPOINTS.TENANT.NODE_ROUTES(tenantId, node.id))
          const routes = normalizeRouteList(response)
          return routes.map((route: RouteRecord) => normalizeRoute(node, route))
        })
      )

      return routeGroups.flat()
    } catch (error) {
      console.error('[RouteApi] 获取路由信息失败:', error)
      throw error
    }
  },

  addRoute: async (nodeId: NodeId, cidr: string) => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.post(API_ENDPOINTS.TENANT.NODE_ROUTES(tenantId, nodeId), { cidr })
      return response.data?.data || response.data
    } catch (error) {
      console.error('[RouteApi] 添加路由失败:', error)
      throw error
    }
  },

  updateRoute: async (nodeId: NodeId, routeId: string, cidr: string) => {
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.put(API_ENDPOINTS.TENANT.NODE_ROUTE(tenantId, nodeId, routeId), { cidr })
      return response.data?.data || response.data
    } catch (error) {
      console.error('[RouteApi] 更新路由失败:', error)
      throw error
    }
  },

  deleteRoute: async (nodeId: NodeId, routeId: string) => {
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
