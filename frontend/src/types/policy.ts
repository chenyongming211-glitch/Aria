import type { CommandId, ISODateTimeString, NodeId, PolicyId, TenantId } from './api'

export type PolicyKind = 'route' | 'acl' | 'qos' | 'blacklist' | 'unknown'
export type PolicyScope = 'node' | 'tenant'
export type DeliveryCommandStatus =
  | 'pending'
  | 'sent'
  | 'acknowledged'
  | 'queued'
  | 'in_progress'
  | 'completed'
  | 'failed'
  | 'stale'
export type PolicyStatus = 'idle' | 'pending' | 'in_progress' | 'applied' | 'error' | 'stale'

export interface PolicyDelivery {
  id?: string
  tenant_id?: TenantId
  node_id?: NodeId
  policy_id?: PolicyId
  command_id?: CommandId
  action?: string
  command_status?: DeliveryCommandStatus | string
  last_error?: string
  created_at?: ISODateTimeString
  updated_at?: ISODateTimeString
}

export type IPGroupKind = 'custom' | 'inline' | 'system' | string

export interface IPGroupMember {
  id?: string
  group_id?: string
  cidr: string
  note?: string
}

export interface IPGroupRecord {
  id?: string
  name?: string
  description?: string
  kind?: IPGroupKind
  members?: IPGroupMember[]
  warnings?: string[]
}

export interface NormalizedIPGroup {
  id: string
  name: string
  description: string
  kind: IPGroupKind
  members: IPGroupMember[]
  warnings: string[]
}

export interface DispatchResult {
  desired_state_version?: string
  desired_state_updated_at?: ISODateTimeString
  command_id?: CommandId
  status?: DeliveryCommandStatus | string
  last_delivery?: PolicyDelivery | null
}

export interface PolicyRecord {
  policy_id?: PolicyId
  policy_ref?: string
  tenant_id?: TenantId
  node_id?: NodeId
  node_name?: string
  target_nodes?: NodeId[]
  scope?: PolicyScope | string
  kind?: PolicyKind | string
  name?: string
  enabled?: boolean
  priority?: number
  version?: string
  policy_status?: PolicyStatus | string
  status?: PolicyStatus | string
  desired_state_version?: string
  desired_state_updated_at?: ISODateTimeString
  applied_state_version?: string
  applied_state_updated_at?: ISODateTimeString
  observed_state?: PolicyStatus | string
  observed_message?: string
  last_sync_error?: string
  observed_at?: ISODateTimeString
  state_convergence?: string
  spec?: Record<string, unknown>
  pending_cmds?: number
  dispatch?: DispatchResult
  last_delivery?: PolicyDelivery | null
  delivery_history?: PolicyDelivery[]
  last_delivery_command_id?: CommandId
  last_delivery_action?: string
  last_delivery_error?: string
  updated_at?: ISODateTimeString
  created_at?: ISODateTimeString
}

export interface NormalizedPolicy extends PolicyRecord {
  id?: PolicyId
  policyId?: PolicyId
  policyRef?: string
  tenantId?: TenantId
  nodeId?: NodeId
  nodeName?: string
  targetNodes: NodeId[]
  scope: PolicyScope | string
  kind: PolicyKind | string
  name: string
  enabled: boolean
  priority: number
  version: string
  status: PolicyStatus | string
  desiredStateVersion: string
  desiredStateUpdatedAt: ISODateTimeString | null
  appliedStateVersion: string
  appliedStateUpdatedAt: ISODateTimeString | null
  observedState: PolicyStatus | string
  observedMessage: string
  observedAt: ISODateTimeString | null
  stateConvergence: string
  spec: Record<string, unknown>
  pendingCmds: number
  lastDelivery: PolicyDelivery | null
  deliveryHistory: PolicyDelivery[]
  lastDeliveryCommandId: CommandId | string
  lastDeliveryAction: string
  lastDeliveryError: string
  updatedAt: ISODateTimeString | null
  createdAt: ISODateTimeString | null
}
