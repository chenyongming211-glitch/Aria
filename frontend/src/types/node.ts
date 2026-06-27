import type { ISODateTimeString, NodeId, TenantId } from './api'

export type NodeAvailabilityStatus = 'online' | 'offline' | 'degraded' | 'unknown'
export type NodeRuntimeStatus =
  | 'registered'
  | 'syncing'
  | 'online'
  | 'degraded'
  | 'offline'
  | 'revoked'
  | 'deleted'
export type StateConvergence = 'idle' | 'converged' | 'diverged' | 'pending'
export type ObservedState = 'idle' | 'pending' | 'applied' | 'failed' | 'stale' | 'degraded'

export interface NodeRecord {
  id: NodeId
  tenant_id?: TenantId
  hostname?: string
  assigned_ip?: string
  private_ip?: string
  public_ip?: string
  endpoint?: string
  region?: string
  status?: NodeRuntimeStatus | string
  availability_status?: NodeAvailabilityStatus | string
  runtime_mode?: string
  kernel_version?: string
  last_seen?: ISODateTimeString | number
  last_sync_at?: ISODateTimeString
  last_sync_error?: string
  desired_state_version?: string
  desired_state_updated_at?: ISODateTimeString
  applied_state_version?: string
  applied_state_updated_at?: ISODateTimeString
  observed_state?: ObservedState | string
  observed_message?: string
  observed_at?: ISODateTimeString
  state_convergence?: StateConvergence | string
  convergence_status?: StateConvergence | string
}

