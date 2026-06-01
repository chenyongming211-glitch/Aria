// src/stores/node.js
import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '@/composables/useApi'
import { API_ENDPOINTS, requireCurrentTenantId } from '@/config/api'
import { useAgentProxyApi } from '@/composables/useAgentProxyApi'
import { useMonitorApi } from '@/composables/useMonitorApi'

export default defineStore('node', () => {
  const nodes = ref([])
  const currentNode = ref(null)
  const loading = ref(false)

  function normalizeAdvertisedRoutes(routes) {
    if (Array.isArray(routes)) return routes
    if (!routes) return []
    if (typeof routes !== 'string') return []

    try {
      const parsed = JSON.parse(routes)
      return Array.isArray(parsed) ? parsed : []
    } catch (error) {
      console.warn('[Node Store] Invalid advertised_routes payload:', error)
      return []
    }
  }

  function normalizeNodeRecord(node = {}, fallback = {}) {
    const lastSeen = node.last_seen
    const lastSyncAt = node.last_sync_at
    return {
      id: node.id || fallback.id || '',
      hostname: node.hostname || fallback.hostname || 'unknown',
      ip: node.assigned_ip || node.private_ip || node.public_ip || fallback.ip || 'N/A',
      publicIp: node.public_ip || fallback.publicIp || 'N/A',
      vpnIp: node.assigned_ip || fallback.vpnIp || 'N/A',
      region: node.region || fallback.region || 'unknown',
      status: node.availability_status || node.status || fallback.status || 'offline',
      rawStatus: node.status || fallback.rawStatus || 'offline',
      version: node.kernel_version || fallback.version || '0.2.26',
      mode: node.runtime_mode || fallback.mode || 'kernel',
      lastSeen: lastSeen ? formatTimestamp(lastSeen) : (fallback.lastSeen || 'N/A'),
      uptime: lastSeen ? formatUptime(lastSeen) : (fallback.uptime || '0 days'),
      routes: normalizeAdvertisedRoutes(node.advertised_routes ?? fallback.routes),
      pendingCmds: node.pending_cmds ?? fallback.pendingCmds ?? 0,
      configurationStatus: node.configuration_status || fallback.configurationStatus || 'idle',
      lastSyncAt: lastSyncAt ? formatTimestamp(lastSyncAt) : (fallback.lastSyncAt || 'N/A'),
      desiredStateVersion: node.desired_state_version || fallback.desiredStateVersion || '',
      desiredStateUpdatedAt: node.desired_state_updated_at ? formatDateTime(node.desired_state_updated_at) : (fallback.desiredStateUpdatedAt || 'N/A'),
      appliedStateVersion: node.applied_state_version || fallback.appliedStateVersion || '',
      appliedStateUpdatedAt: node.applied_state_updated_at ? formatDateTime(node.applied_state_updated_at) : (fallback.appliedStateUpdatedAt || 'N/A'),
      observedState: node.observed_state || node.configuration_status || fallback.observedState || 'idle',
      observedMessage: node.observed_message || node.last_sync_error || fallback.observedMessage || '',
      observedAt: node.observed_at ? formatDateTime(node.observed_at) : (fallback.observedAt || 'N/A'),
      stateConvergence: node.convergence_status || node.state_convergence || fallback.stateConvergence || 'idle',
      lastCommand: node.last_command || fallback.lastCommand || null,
      lastCommandStatus: node.last_command_status || fallback.lastCommandStatus || '',
      lastCommandError: node.last_command_error || fallback.lastCommandError || '',
      recentCommands: Array.isArray(node.recent_commands) ? node.recent_commands : (fallback.recentCommands || []),
      bandwidth: fallback.bandwidth || { upload: 0, download: 0 },
      latency: fallback.latency || 0
    }
  }

  async function loadNodes() {
    loading.value = true
    try {
      console.log('[Node Store] Loading nodes...')
      
      // 使用租户节点 API
      const tenantId = requireCurrentTenantId()
      const response = await api.get(API_ENDPOINTS.TENANT.NODES(tenantId))
      console.log('[Node Store] Response:', response.data)
      
      let nodeData = []
      const result = response.data
      
      // 解析响应格式
      if (Array.isArray(result)) {
        nodeData = result
      } else if (result && Array.isArray(result.data)) {
        nodeData = result.data
      } else if (result && result.success && Array.isArray(result.data)) {
        nodeData = result.data
      }
      
      console.log('[Node Store] Parsed node data:', nodeData)
      
      if (nodeData.length > 0) {
        nodes.value = nodeData.map(node => normalizeNodeRecord(node))
      } else {
        nodes.value = []
      }
    } catch (error) {
      console.error('[Node Store] Failed to load nodes:', error)
      console.error('[Node Store] Error response:', error.response)
      if (error.response?.status === 401) {
        console.warn('[Node Store] Unauthorized: Missing or invalid token')
      }
      nodes.value = []
    } finally {
      loading.value = false
    }
  }

  async function loadNodeDetail(id) {
    const tenantId = requireCurrentTenantId()
    const [detailResult, monitorResult, statusResult, commandsResult] = await Promise.allSettled([
      api.get(API_ENDPOINTS.TENANT.NODE_DETAIL(tenantId, id)),
      useMonitorApi.getNodeDetail(id),
      useAgentProxyApi.getAgentStatus(id),
      useAgentProxyApi.getAgentCommands(id, 10)
    ])

    if (detailResult.status === 'rejected' && monitorResult.status === 'rejected') {
      throw detailResult.reason || monitorResult.reason
    }

    if (statusResult.status === 'rejected') {
      console.warn('[Node Store] Failed to load node operation status:', statusResult.reason)
    }
    if (commandsResult.status === 'rejected') {
      console.warn('[Node Store] Failed to load node command history:', commandsResult.reason)
    }

    const detailResponse = detailResult.status === 'fulfilled' ? detailResult.value : { data: {} }
    const monitorDetail = monitorResult.status === 'fulfilled' ? (monitorResult.value || {}) : {}
    const statusResponse = statusResult.status === 'fulfilled' ? (statusResult.value || {}) : {}
    const commandsResponse = commandsResult.status === 'fulfilled' ? (commandsResult.value || {}) : {}
    const detail = detailResponse.data?.data || detailResponse.data || {}
    const status = statusResponse || {}
    const commands = Array.isArray(commandsResponse?.items)
      ? commandsResponse.items
      : (Array.isArray(monitorDetail?.recent_commands) ? monitorDetail.recent_commands : [])

    const node = {
      id: detail.id || id,
      hostname: detail.hostname || monitorDetail?.hostname || 'unknown',
      ip: detail.assigned_ip || detail.private_ip || detail.public_ip || 'N/A',
      publicIp: detail.public_ip || 'N/A',
      vpnIp: detail.assigned_ip || 'N/A',
      region: detail.region || 'unknown',
      status: status.availability_status || monitorDetail?.availability_status || detail.availability_status || detail.status || 'offline',
      rawStatus: detail.status || 'offline',
      version: detail.kernel_version || '0.2.26',
      mode: detail.runtime_mode || 'kernel',
      lastSeen: detail.last_seen ? formatTimestamp(detail.last_seen) : 'N/A',
      uptime: status.uptime ? formatDurationSeconds(status.uptime) : (detail.last_seen ? formatUptime(detail.last_seen) : '0 days'),
      routes: normalizeAdvertisedRoutes(detail.advertised_routes),
      pendingCmds: status.pending_cmds || detail.pending_cmds || 0,
      configurationStatus: status.configuration_status || detail.configuration_status || 'idle',
      lastSyncAt: status.last_sync_at
        ? formatTimestamp(status.last_sync_at)
        : (monitorDetail?.last_sync_at
            ? formatTimestamp(monitorDetail.last_sync_at)
            : (detail.last_sync_at ? formatTimestamp(detail.last_sync_at) : 'N/A')),
      desiredStateVersion: status.desired_state_version || monitorDetail?.desired_state_version || detail.desired_state_version || '',
      desiredStateUpdatedAt: formatDateTime(status.desired_state_updated_at || detail.desired_state_updated_at),
      appliedStateVersion: status.applied_state_version || monitorDetail?.applied_state_version || detail.applied_state_version || '',
      appliedStateUpdatedAt: formatDateTime(status.applied_state_updated_at || detail.applied_state_updated_at),
      observedState: status.observed_state || monitorDetail?.observed_state || detail.observed_state || status.configuration_status || detail.configuration_status || 'idle',
      observedMessage: status.observed_message || monitorDetail?.observed_message || detail.observed_message || status.last_sync_error || monitorDetail?.last_sync_error || detail.last_sync_error || '',
      observedAt: formatDateTime(status.observed_at || detail.observed_at),
      stateConvergence: status.convergence_status || monitorDetail?.state_convergence || detail.convergence_status || status.state_convergence || detail.state_convergence || 'idle',
      learnedRoutes: Array.isArray(monitorDetail?.learned_routes) ? monitorDetail.learned_routes : [],
      lastCommand: status.last_command || detail.last_command || null,
      lastCommandStatus: status.last_command_status || detail.last_command_status || '',
      lastCommandError: status.last_command_error || detail.last_command_error || '',
      recentCommands: Array.isArray(commands) ? commands : [],
      recentPolicyDeliveries: Array.isArray(monitorDetail?.recent_policy_deliveries) ? monitorDetail.recent_policy_deliveries : [],
      activeAlerts: Array.isArray(monitorDetail?.active_alerts) ? monitorDetail.active_alerts : [],
      certificate: monitorDetail?.certificate || detail.certificate || null,
      certificateActivity: monitorDetail?.certificate_activity || detail.certificate_activity || null,
      bandwidth: { upload: 0, download: 0 },
      latency: 0
    }

    currentNode.value = node
    return node
  }

  // 格式化时间戳（按 Asia/Shanghai 展示，不对原始时间手动加偏移）
  function formatTimestamp(timestamp) {
    if (!timestamp) return 'N/A'
    const date = typeof timestamp === 'number'
      ? new Date(timestamp * 1000)
      : new Date(timestamp)
    if (Number.isNaN(date.getTime())) return 'N/A'

    return new Intl.DateTimeFormat('zh-CN', {
      timeZone: 'Asia/Shanghai',
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false
    }).format(date).replace(/\//g, '-')
  }

  function formatDateTime(value) {
    if (!value) return 'N/A'
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return 'N/A'
    return date.toLocaleString()
  }

  // 格式化运行时间
  function formatUptime(lastSeen) {
    const lastSeenMs = typeof lastSeen === 'number'
      ? lastSeen * 1000
      : new Date(lastSeen).getTime()
    if (Number.isNaN(lastSeenMs)) return '0 days'

    const diff = Math.max(0, Math.floor((Date.now() - lastSeenMs) / 1000))

    if (diff < 60) return '< 1 minute'
    if (diff < 3600) return `${Math.floor(diff / 60)} minutes`
    if (diff < 86400) return `${Math.floor(diff / 3600)} hours`
    return `${Math.floor(diff / 86400)} days`
  }

  function formatDurationSeconds(seconds) {
    if (!seconds || seconds <= 0) return '0 minutes'
    if (seconds < 60) return `${seconds} seconds`
    if (seconds < 3600) return `${Math.floor(seconds / 60)} minutes`
    if (seconds < 86400) return `${Math.floor(seconds / 3600)} hours`
    return `${Math.floor(seconds / 86400)} days`
  }

  function getNodeById(id) {
    return nodes.value.find(node => node.id === id)
  }

  function updateNode(updatedNode) {
    const index = nodes.value.findIndex(node => node.id === updatedNode.id)
    if (index !== -1) {
      nodes.value[index] = { ...updatedNode }
    }
  }

  function deleteNode(id) {
    nodes.value = nodes.value.filter(node => node.id !== id)
  }

  async function deleteNodeRemote(id) {
    loading.value = true
    try {
      const tenantId = requireCurrentTenantId()
      await api.delete(API_ENDPOINTS.TENANT.NODE_DETAIL(tenantId, id))
      deleteNode(id)
    } catch (error) {
      console.error('[Node Store] Failed to delete node:', error)
      throw error
    } finally {
      loading.value = false
    }
  }

  async function updateNodeRemote(id, data) {
    loading.value = true
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.put(API_ENDPOINTS.TENANT.NODE_DETAIL(tenantId, id), data)
      const updatedData = response.data?.data || response.data
      
      const index = nodes.value.findIndex(node => node.id === id)
      if (index !== -1) {
        nodes.value[index] = normalizeNodeRecord(updatedData || {}, nodes.value[index])
      }
      if (currentNode.value?.id === id) {
        currentNode.value = {
          ...currentNode.value,
          ...normalizeNodeRecord(updatedData || {}, currentNode.value)
        }
      }
      
      return updatedData
    } catch (error) {
      console.error('[Node Store] Failed to update node:', error)
      throw error
    } finally {
      loading.value = false
    }
  }

  function setCurrentNode(node) {
    currentNode.value = node
  }

  return {
    nodes,
    currentNode,
    loading,
    loadNodes,
    loadNodeDetail,
    getNodeById,
    updateNode,
    updateNodeRemote,
    deleteNode,
    deleteNodeRemote,
    setCurrentNode
  }
})
