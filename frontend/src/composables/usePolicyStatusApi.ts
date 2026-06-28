import api from './useApi'
import { unwrapApiData } from './apiResponse'
import { API_ENDPOINTS, getCurrentTenantId } from '@/config/api'

type AnyRow = Record<string, any>

export interface PolicyDeliveryStatusRef {
  nodeId?: string
  node_id?: string
  policyDomain?: string
  policy_domain?: string
  kind?: string
  policyRef?: string
  policy_ref?: string
  id?: string | number
  cidr?: string
}

export interface PolicyDeliveryStatusItem {
  node_id: string
  policy_domain: string
  policy_ref: string
  policy_status: string
  pending_cmds: number
  last_delivery?: AnyRow | null
  delivery_history?: AnyRow[]
}

export interface NodeStatusItem extends AnyRow {
  node_id: string
  status?: string
  availability_status?: string
  configuration_status?: string
  convergence_status?: string
  state_convergence?: string
  pending_cmds?: number
}

const activePolicyStatuses = new Set([
  'pending',
  'queued',
  'sent',
  'ack',
  'acked',
  'acknowledged',
  'in_progress',
  'syncing'
])

const activeNodeStatuses = new Set([
  'pending',
  'queued',
  'sent',
  'ack',
  'acked',
  'acknowledged',
  'in_progress',
  'syncing',
  'diverged'
])

const normalizeString = (value: unknown): string => String(value || '').trim()
const normalizeStatus = (value: unknown): string => normalizeString(value).toLowerCase()

export const policyRefFromRow = (row: PolicyDeliveryStatusRef): string => (
  normalizeString(row.policyRef) ||
  normalizeString(row.policy_ref) ||
  normalizeString(row.id) ||
  normalizeString(row.cidr)
)

export const policyDomainFromRow = (row: PolicyDeliveryStatusRef): string => (
  normalizeString(row.policyDomain) ||
  normalizeString(row.policy_domain) ||
  normalizeString(row.kind)
).toLowerCase()

export const nodeIdFromPolicyRow = (row: PolicyDeliveryStatusRef): string => (
  normalizeString(row.nodeId) ||
  normalizeString(row.node_id)
)

export const isActivePolicyStatus = (row: AnyRow = {}): boolean => {
  const status = normalizeStatus(row.policy_status || row.policyStatus || row.status || row.observedState || row.observed_state)
  const pending = Number(row.pending_cmds ?? row.pendingCmds ?? 0)
  return pending > 0 || activePolicyStatuses.has(status)
}

export const isActiveNodeStatus = (row: AnyRow = {}): boolean => {
  const status = normalizeStatus(row.convergence_status || row.stateConvergence || row.state_convergence || row.configurationStatus || row.configuration_status)
  const commandStatus = normalizeStatus(row.lastCommandStatus || row.last_command_status)
  const pending = Number(row.pendingCmds ?? row.pending_cmds ?? 0)
  return pending > 0 || activeNodeStatuses.has(status) || activeNodeStatuses.has(commandStatus)
}

export const normalizePolicyStatusItem = (item: PolicyDeliveryStatusItem): AnyRow => {
  const lastDelivery = item.last_delivery || null
  const deliveryHistory = Array.isArray(item.delivery_history)
    ? item.delivery_history
    : (lastDelivery ? [lastDelivery] : [])
  const lastError = lastDelivery?.last_error || ''

  return {
    node_id: item.node_id,
    nodeId: item.node_id,
    policy_domain: item.policy_domain,
    policyDomain: item.policy_domain,
    policy_ref: item.policy_ref,
    policyRef: item.policy_ref,
    policy_status: item.policy_status || 'idle',
    policyStatus: item.policy_status || 'idle',
    status: item.policy_status || 'idle',
    pending_cmds: Number(item.pending_cmds || 0),
    pendingCmds: Number(item.pending_cmds || 0),
    last_delivery: lastDelivery,
    lastDelivery,
    delivery_history: deliveryHistory,
    deliveryHistory,
    last_delivery_command_id: lastDelivery?.command_id || '',
    lastDeliveryCommandId: lastDelivery?.command_id || '',
    last_delivery_action: lastDelivery?.action || '',
    lastDeliveryAction: lastDelivery?.action || '',
    last_delivery_error: lastError,
    lastDeliveryError: lastError,
    last_command_error: lastError,
    lastCommandError: lastError
  }
}

export const patchPolicyStatusRow = (row: AnyRow, statusItem: AnyRow): void => {
  Object.assign(row, statusItem)
}

export const usePolicyStatusApi = {
  getPolicyDeliveryStatuses: async (items: PolicyDeliveryStatusRef[]): Promise<AnyRow[]> => {
    const tenantId = getCurrentTenantId()
    if (!tenantId) {
      return []
    }
    const requestItems = items
      .map((item) => ({
        node_id: nodeIdFromPolicyRow(item),
        policy_domain: policyDomainFromRow(item),
        policy_ref: policyRefFromRow(item)
      }))
      .filter((item) => item.node_id && item.policy_domain && item.policy_ref)

    if (requestItems.length === 0) {
      return []
    }

    const response = await api.post(API_ENDPOINTS.TENANT.POLICY_DELIVERY_STATUS(tenantId), {
      items: requestItems
    })
    const data = unwrapApiData<{ items?: PolicyDeliveryStatusItem[] }>(response) || {}
    return (Array.isArray(data.items) ? data.items : []).map(normalizePolicyStatusItem)
  },

  getNodeStatuses: async (nodeIds: string[]): Promise<NodeStatusItem[]> => {
    const tenantId = getCurrentTenantId()
    if (!tenantId) {
      return []
    }
    const ids = Array.from(new Set(nodeIds.map((id) => normalizeString(id)).filter(Boolean)))
    if (ids.length === 0) {
      return []
    }

    const response = await api.post(API_ENDPOINTS.TENANT.NODES_STATUS(tenantId), {
      node_ids: ids
    })
    const data = unwrapApiData<{ items?: NodeStatusItem[] }>(response) || {}
    return Array.isArray(data.items) ? data.items : []
  }
}
