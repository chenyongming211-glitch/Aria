import api from './useApi'
import { API_ENDPOINTS, requireCurrentTenantId } from '@/config/api'

function normalizePolicy(policy) {
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
    status: policy.status || policy.policy_status || 'idle',
    desiredStateVersion: policy.desired_state_version || '',
    desiredStateUpdatedAt: policy.desired_state_updated_at || null,
    appliedStateVersion: policy.applied_state_version || '',
    appliedStateUpdatedAt: policy.applied_state_updated_at || null,
    observedState: policy.observed_state || policy.status || policy.policy_status || 'idle',
    observedMessage: policy.observed_message || policy.last_sync_error || '',
    observedAt: policy.observed_at || null,
    stateConvergence: policy.state_convergence || '',
    spec: policy.spec || {},
    pendingCmds: Number(policy.pending_cmds || 0),
    lastDelivery: policy.last_delivery || null,
    deliveryHistory: Array.isArray(policy.delivery_history) ? policy.delivery_history : [],
    lastDeliveryCommandId: policy.last_delivery_command_id || policy.last_delivery?.command_id || '',
    lastDeliveryAction: policy.last_delivery_action || policy.last_delivery?.action || '',
    lastDeliveryError: policy.last_delivery_error || '',
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
  }
}

export const usePolicyApi = {
  listPolicies: async (filters = {}) => {
    const tenantId = requireCurrentTenantId()
    const params = {}

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
    const items = response.data?.data || response.data || []
    return items.map(normalizePolicy)
  },

  retryPolicySync: async ({ nodeId, kind, policyRef, policyName = '' }) => {
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
    return normalizePolicy(response.data?.data || response.data || {})
  }
}
