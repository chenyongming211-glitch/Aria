// src/stores/node.js
import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '@/composables/useApi'
import { API_ENDPOINTS } from '@/config/api'

export default defineStore('node', () => {
  const nodes = ref([])
  const currentNode = ref(null)
  const loading = ref(false)

  async function loadNodes() {
    loading.value = true
    try {
      console.log('[Node Store] Loading nodes...')
      
      // 使用租户节点 API
      const response = await api.get(API_ENDPOINTS.TENANT.NODES)
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
          status: node.status || 'offline',
          version: node.kernel_version || '0.2.26',
          mode: node.runtime_mode || 'kernel',
          lastSeen: node.last_seen ? formatTimestamp(node.last_seen) : 'N/A',
          uptime: node.last_seen ? formatUptime(node.last_seen) : '0 days',
          routes: node.advertised_routes || [],
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

  // 格式化时间戳
  function formatTimestamp(timestamp) {
    if (!timestamp) return 'N/A'
    const date = new Date(timestamp * 1000)
    return date.toISOString().slice(0, 19).replace('T', ' ')
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

  function setCurrentNode(node) {
    currentNode.value = node
  }

  return {
    nodes,
    currentNode,
    loading,
    loadNodes,
    getNodeById,
    updateNode,
    deleteNode,
    setCurrentNode
  }
})
