import api from './useApi'
import { unwrapApiData, unwrapApiList } from './apiResponse'
import { API_ENDPOINTS, requireCurrentTenantId } from '@/config/api'
import {
  mapCommandStatusToPolicyStatus,
  pendingCountForCommandStatus
} from '@/utils/controlLoopStatus'
import type { NormalizedPolicy, PolicyKind, PolicyRecord } from '@/types'

interface PolicyFilters {
  kind?: string
  nodeId?: string
  enabled?: boolean | string
}

interface PolicyRetryPayload {
  nodeId?: string
  kind?: PolicyKind | string
  policyRef?: string
  policyName?: string
}

function normalizePolicy(policy: PolicyRecord): NormalizedPolicy {
  const lastDelivery = policy.last_delivery || null
  const deliveryHistory = Array.isArray(policy.delivery_history)
    ? policy.delivery_history
    : (lastDelivery ? [lastDelivery] : [])
  const deliveryStatus = lastDelivery?.command_status || policy.dispatch?.status || ''
  const mappedDeliveryStatus = mapCommandStatusToPolicyStatus(deliveryStatus)
  const status = policy.policy_status || mappedDeliveryStatus || policy.status || 'idle'
  const pendingCmds = typeof policy.pending_cmds === 'number'
    ? policy.pending_cmds
    : (deliveryHistory.length > 0
        ? deliveryHistory.reduce((total, delivery) => total + pendingCountForCommandStatus(delivery?.command_status), 0)
        : pendingCountForCommandStatus(deliveryStatus))
  const lastDeliveryError = policy.last_delivery_error || lastDelivery?.last_error || ''

  const normalized = {
    id: policy.policy_id,
    policyId: policy.policy_id,
    policyRef: policy.policy_ref,
    tenantId: policy.tenant_id,
    nodeId: policy.node_id,
    nodeName: policy.node_name,
    targetNodes: Array.isArray(policy.target_nodes) ? policy.target_nodes : [],
    scope: policy.scope || 'node',
    kind: policy.kind || 'unknown',
    name: policy.name || policy.policy_ref || policy.policy_id,
    enabled: policy.enabled !== false,
    priority: Number(policy.priority || 100),
    version: policy.version || '',
    status,
    desiredStateVersion: policy.desired_state_version || '',
    desiredStateUpdatedAt: policy.desired_state_updated_at || null,
    appliedStateVersion: policy.applied_state_version || '',
    appliedStateUpdatedAt: policy.applied_state_updated_at || null,
    observedState: policy.observed_state || status,
    observedMessage: policy.observed_message || policy.last_sync_error || '',
    observedAt: policy.observed_at || null,
    stateConvergence: policy.state_convergence || '',
    spec: policy.spec || {},
    pendingCmds,
    lastDelivery,
    deliveryHistory,
    lastDeliveryCommandId: policy.last_delivery_command_id || lastDelivery?.command_id || '',
    lastDeliveryAction: policy.last_delivery_action || lastDelivery?.action || '',
    lastDeliveryError,
    updatedAt: policy.updated_at || null,
    createdAt: policy.created_at || null
  }

  return {
    ...policy,
    ...normalized,
    policy_status: normalized.status,
    last_delivery_command_id: normalized.lastDeliveryCommandId,
    last_delivery_action: normalized.lastDeliveryAction,
    last_delivery_error: normalized.lastDeliveryError,
    last_delivery: normalized.lastDelivery,
    delivery_history: normalized.deliveryHistory,
    pending_cmds: normalized.pendingCmds
  } as NormalizedPolicy
}

export const usePolicyApi = {
  listPolicies: async (filters: PolicyFilters = {}): Promise<NormalizedPolicy[]> => {
    const tenantId = requireCurrentTenantId()
    const params: Record<string, string> = {}

    if (filters.kind) {
      params.kind = filters.kind
    }
    if (filters.nodeId) {
      params.node_id = filters.nodeId
    }
    if (filters.enabled !== undefined && filters.enabled !== null && filters.enabled !== '') {
      params.enabled = String(filters.enabled)
    }

    const response = await api.get(API_ENDPOINTS.TENANT.POLICIES(tenantId), { params })
    const items = unwrapApiList<PolicyRecord>(response)
    return items.map(normalizePolicy)
  },

  retryPolicySync: async ({ nodeId, kind, policyRef, policyName = '' }: PolicyRetryPayload): Promise<NormalizedPolicy> => {
    const tenantId = requireCurrentTenantId()
    if (!nodeId) {
      throw new Error('nodeId is required for policy retry')
    }
    if (!kind) {
      throw new Error('kind is required for policy retry')
    }
    if (!policyRef) {
      throw new Error('policyRef is required for policy retry')
    }

    const response = await api.post(API_ENDPOINTS.TENANT.POLICY_RETRY(tenantId), {
      node_id: nodeId,
      kind,
      policy_ref: policyRef,
      policy_name: policyName
    })
    return normalizePolicy(unwrapApiData<PolicyRecord>(response) || {})
  }
}
