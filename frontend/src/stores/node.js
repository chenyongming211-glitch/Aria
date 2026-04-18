// src/stores/node.js
import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '@/composables/useApi'
import { API_ENDPOINTS, requireCurrentTenantId } from '@/config/api'
import { useAgentProxyApi } from '@/composables/useAgentProxyApi'

export default defineStore('node', () => {
  const nodes = ref([])
  const currentNode = ref(null)
  const loading = ref(false)

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
        // 转换 API 响应格式
        nodes.value = nodeData.map(node => ({
          id: node.id || '',
          hostname: node.hostname || 'unknown',
          ip: node.assigned_ip || node.private_ip || node.public_ip || 'N/A',
          publicIp: node.public_ip || 'N/A',
          vpnIp: node.assigned_ip || 'N/A',
          region: node.region || 'unknown',
          status: node.availability_status || node.status || 'offline',
          rawStatus: node.status || 'offline',
          version: node.kernel_version || '0.2.26',
          mode: node.runtime_mode || 'kernel',
          lastSeen: node.last_seen ? formatTimestamp(node.last_seen) : 'N/A',
          uptime: node.last_seen ? formatUptime(node.last_seen) : '0 days',
          routes: node.advertised_routes || [],
          pendingCmds: node.pending_cmds || 0,
          configurationStatus: node.configuration_status || 'idle',
          lastSyncAt: node.last_sync_at ? formatTimestamp(node.last_sync_at) : 'N/A',
          desiredStateVersion: node.desired_state_version || '',
          desiredStateUpdatedAt: node.desired_state_updated_at ? formatDateTime(node.desired_state_updated_at) : 'N/A',
          appliedStateVersion: node.applied_state_version || '',
          appliedStateUpdatedAt: node.applied_state_updated_at ? formatDateTime(node.applied_state_updated_at) : 'N/A',
          observedState: node.observed_state || node.configuration_status || 'idle',
          observedMessage: node.observed_message || node.last_sync_error || '',
          observedAt: node.observed_at ? formatDateTime(node.observed_at) : 'N/A',
          stateConvergence: node.convergence_status || node.state_convergence || 'idle',
          lastCommand: node.last_command || null,
          lastCommandStatus: node.last_command_status || '',
          lastCommandError: node.last_command_error || '',
          recentCommands: Array.isArray(node.recent_commands) ? node.recent_commands : [],
          bandwidth: { upload: 0, download: 0 },
          latency: 0
        }))
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
    const [detailResponse, statusResponse, commandsResponse] = await Promise.all([
      api.get(API_ENDPOINTS.TENANT.NODE_DETAIL(tenantId, id)),
      useAgentProxyApi.getAgentStatus(id),
      useAgentProxyApi.getAgentCommands(id, 10)
    ])

    const detail = detailResponse.data?.data || detailResponse.data || {}
    const status = statusResponse || {}
    const commands = commandsResponse?.items || []

    const node = {
      id: detail.id || id,
      hostname: detail.hostname || 'unknown',
      ip: detail.assigned_ip || detail.private_ip || detail.public_ip || 'N/A',
      publicIp: detail.public_ip || 'N/A',
      vpnIp: detail.assigned_ip || 'N/A',
      region: detail.region || 'unknown',
      status: status.availability_status || detail.availability_status || detail.status || 'offline',
      rawStatus: detail.status || 'offline',
      version: detail.kernel_version || '0.2.26',
      mode: detail.runtime_mode || 'kernel',
      lastSeen: detail.last_seen ? formatTimestamp(detail.last_seen) : 'N/A',
      uptime: status.uptime ? formatDurationSeconds(status.uptime) : (detail.last_seen ? formatUptime(detail.last_seen) : '0 days'),
      routes: detail.advertised_routes || [],
      pendingCmds: status.pending_cmds || detail.pending_cmds || 0,
      configurationStatus: status.configuration_status || detail.configuration_status || 'idle',
      lastSyncAt: status.last_sync_at ? formatTimestamp(status.last_sync_at) : (detail.last_sync_at ? formatTimestamp(detail.last_sync_at) : 'N/A'),
      desiredStateVersion: status.desired_state_version || detail.desired_state_version || '',
      desiredStateUpdatedAt: formatDateTime(status.desired_state_updated_at || detail.desired_state_updated_at),
      appliedStateVersion: status.applied_state_version || detail.applied_state_version || '',
      appliedStateUpdatedAt: formatDateTime(status.applied_state_updated_at || detail.applied_state_updated_at),
      observedState: status.observed_state || detail.observed_state || status.configuration_status || detail.configuration_status || 'idle',
      observedMessage: status.observed_message || detail.observed_message || status.last_sync_error || detail.last_sync_error || '',
      observedAt: formatDateTime(status.observed_at || detail.observed_at),
      stateConvergence: status.convergence_status || detail.convergence_status || status.state_convergence || detail.state_convergence || 'idle',
      learnedRoutes: detail.learned_routes || [],
      lastCommand: status.last_command || detail.last_command || null,
      lastCommandStatus: status.last_command_status || detail.last_command_status || '',
      lastCommandError: status.last_command_error || detail.last_command_error || '',
      recentCommands: Array.isArray(commands) ? commands : [],
      bandwidth: { upload: 0, download: 0 },
      latency: 0
    }

    currentNode.value = node
    return node
  }

  // 格式化时间戳（转换为北京时间 UTC+8）
  function formatTimestamp(timestamp) {
    if (!timestamp) return 'N/A'
    const date = typeof timestamp === 'number'
      ? new Date(timestamp * 1000)
      : new Date(timestamp)
    if (Number.isNaN(date.getTime())) return 'N/A'
    
    // 转换为北京时间（UTC+8）
    const beijingTime = new Date(date.getTime() + 8 * 60 * 60 * 1000)
    
    // 格式化为 YYYY-MM-DD HH:mm:ss
    const year = beijingTime.getUTCFullYear()
    const month = String(beijingTime.getUTCMonth() + 1).padStart(2, '0')
    const day = String(beijingTime.getUTCDate()).padStart(2, '0')
    const hours = String(beijingTime.getUTCHours()).padStart(2, '0')
    const minutes = String(beijingTime.getUTCMinutes()).padStart(2, '0')
    const seconds = String(beijingTime.getUTCSeconds()).padStart(2, '0')
    
    return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`
  }

  function formatDateTime(value) {
    if (!value) return 'N/A'
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return 'N/A'
    return date.toLocaleString()
  }

  // 格式化运行时间
  function formatUptime(lastSeen) {
    const now = Math.floor(Date.now() / 1000)
    const diff = now - lastSeen

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

  async function updateNodeRemote(id, data) {
    loading.value = true
    try {
      const tenantId = requireCurrentTenantId()
      const response = await api.put(API_ENDPOINTS.TENANT.NODE_DETAIL(tenantId, id), data)
      const updatedData = response.data?.data || response.data
      
      // 更新本地状态
      const index = nodes.value.findIndex(node => node.id === id)
      if (index !== -1) {
        // 部分更新本地字段
        nodes.value[index] = { ...nodes.value[index], ...data }
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
    setCurrentNode
  }
})
