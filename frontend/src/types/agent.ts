import type { CommandId, ISODateTimeString, NodeId, TenantId } from './api'

export type AgentCommandStatus =
  | 'pending'
  | 'sent'
  | 'acknowledged'
  | 'queued'
  | 'in_progress'
  | 'completed'
  | 'failed'
  | 'stale'
export type AgentCommandType = 'sync' | 'health_check' | 'apply_policy' | 'reload' | string

export interface AgentCommandPayload {
  type?: AgentCommandType
  action?: string
  payload?: Record<string, unknown>
  reason?: string
}

export interface AgentCommandRecord {
  id?: CommandId
  command_id?: CommandId
  tenant_id?: TenantId
  node_id?: NodeId
  type?: AgentCommandType
  action?: string
  status?: AgentCommandStatus | string
  last_error?: string
  created_at?: ISODateTimeString
  updated_at?: ISODateTimeString
}

export interface AgentStatus {
  node_id?: NodeId
  availability_status?: string
  configuration_status?: string
  pending_cmds?: number
  desired_state_version?: string
  applied_state_version?: string
  observed_state?: string
  observed_message?: string
  last_sync_at?: ISODateTimeString
  last_sync_error?: string
}

