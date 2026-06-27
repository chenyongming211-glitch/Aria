import type { AlertId, ISODateTimeString, ListResult, NodeId } from './api'
import type { PolicyDelivery, PolicyStatus } from './policy'

export type AlertStatus = 'active' | 'resolved'
export type AlertSeverity = 'info' | 'warning' | 'critical'
export type MonitoringRange = '1h' | '24h' | '7d' | '30d'

export interface MonitoringStats {
  total_nodes: number
  online_nodes: number
  offline_nodes: number
  sync_success_rate: number
  total_peers: number
  total_acl_rules: number
  total_qos_rules: number
  failed_commands_count: number
  active_alerts_count: number
}

export interface AlertRecord {
  id: AlertId
  node_id?: NodeId
  alert_type?: string
  severity: AlertSeverity | string
  status: AlertStatus | string
  message?: string
  created_at?: ISODateTimeString
  resolved_at?: ISODateTimeString
}

export interface MonitoringNodeDetail {
  id: NodeId
  hostname?: string
  desired_state_version?: string
  applied_state_version?: string
  observed_state?: PolicyStatus | string
  observed_message?: string
  last_sync_at?: ISODateTimeString
  last_sync_error?: string
  state_convergence?: string
  recent_policy_deliveries?: PolicyDelivery[]
  active_alerts?: AlertRecord[]
}

export interface MonitoringEventParams {
  limit?: number
  offset?: number
  node_id?: NodeId
  event_type?: string
  severity?: AlertSeverity | string
  since?: ISODateTimeString
}

export interface MonitoringAlertParams {
  status?: AlertStatus | 'all' | string
  alert_type?: string
  node_id?: NodeId
  limit?: number
  offset?: number
}

export interface MonitoringEvent {
  id?: string
  node_id?: NodeId
  event_type?: string
  severity?: AlertSeverity | string
  message?: string
  created_at?: ISODateTimeString
  source?: string
}

export interface MonitoringTrafficPoint {
  timestamp?: ISODateTimeString
  upload?: number
  download?: number
  upload_bps?: number
  download_bps?: number
}

export interface MonitoringTraffic {
  range?: MonitoringRange | string
  points?: MonitoringTrafficPoint[]
  upload?: MonitoringTrafficPoint[]
  download?: MonitoringTrafficPoint[]
}

export interface MonitoringHealth {
  status?: string
  controller?: string
  database?: string
  storage?: string
  checks?: Record<string, unknown>
}

export interface MonitoringNodeMetrics {
  node_id?: NodeId
  bandwidth?: MonitoringTraffic
  latency_ms?: number
  packet_loss?: number
  samples?: MonitoringTrafficPoint[]
}

export interface MonitoringTopologyNode {
  id: NodeId
  hostname?: string
  status?: string
  vpn_ip?: string
  public_ip?: string
}

export interface MonitoringTopologyLink {
  source: NodeId
  target: NodeId
  status?: string
  latency_ms?: number
}

export interface MonitoringTopology {
  nodes?: MonitoringTopologyNode[]
  links?: MonitoringTopologyLink[]
}

export type AlertListResult = ListResult<AlertRecord>
export type MonitoringEventListResult = ListResult<MonitoringEvent>

