<!-- src/views/Nodes.vue - 现代化节点管理页面 -->
<template>
  <div class="nodes-page">
    <!-- 页面头部 -->
    <div class="page-header">
      <div class="header-content">
        <h2 class="page-title">
          <el-icon class="title-icon"><Monitor /></el-icon>
          Node Management
        </h2>
        <p class="page-subtitle">Monitor and manage your network nodes</p>
      </div>
      <div class="header-actions">
        <el-input
          v-model="searchQuery"
          placeholder="Search nodes..."
          class="search-input"
          clearable
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>
        <el-button
          :icon="Refresh"
          @click="refreshNodes"
          :loading="loading"
        >
          Refresh
        </el-button>
        <el-button type="primary" :icon="Plus" @click="addNode">
          Add Node
        </el-button>
      </div>
    </div>

    <!-- 统计卡片 -->
    <div class="stats-cards">
      <div class="stat-item">
        <div class="stat-icon blue">
          <el-icon><Monitor /></el-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ nodes.length }}</div>
          <div class="stat-label">Total Nodes</div>
        </div>
      </div>
      <div class="stat-item">
        <div class="stat-icon green">
          <el-icon><CircleCheck /></el-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ onlineCount }}</div>
          <div class="stat-label">Online</div>
        </div>
      </div>
      <div class="stat-item">
        <div class="stat-icon orange">
          <el-icon><CircleClose /></el-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ offlineCount }}</div>
          <div class="stat-label">Offline</div>
        </div>
      </div>
      <div class="stat-item">
        <div class="stat-icon purple">
          <el-icon><Setting /></el-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ maintenanceCount }}</div>
          <div class="stat-label">Maintenance</div>
        </div>
      </div>
    </div>

    <!-- 节点列表卡片 -->
    <el-card class="nodes-card glass-card" shadow="never">
      <el-table
        :data="paginatedNodes"
        stripe
        class="nodes-table"
        v-loading="loading"
      >
        <el-table-column prop="hostname" label="Hostname" min-width="140" />
        <el-table-column prop="publicIp" label="Public IP" width="130" />
        <el-table-column prop="vpnIp" label="VPN IP" width="120" />
        <el-table-column prop="region" label="Region" width="80">
          <template #default="{ row }">
            <span class="region-badge">{{ row.region.toUpperCase() }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="version" label="Version" width="100" />
        <el-table-column prop="mode" label="Mode" width="100">
          <template #default="{ row }">
            <div class="mode-badge">
              {{ row.mode }}
              <el-tag
                v-if="row.mode === 'kernel'"
                size="small"
                type="success"
                effect="plain"
              >
                Opt
              </el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="Status" width="100">
          <template #default="{ row }">
            <span class="status-badge" :class="row.status">
              {{ row.status }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="Config" width="120">
          <template #default="{ row }">
            <el-tag size="small" :type="getConfigTagType(row.configurationStatus)">
              {{ formatConfigStatus(row.configurationStatus) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="Desired / Applied" width="180">
          <template #default="{ row }">
            <div class="state-version-cell">
              <div>{{ shortStateVersion(row.desiredStateVersion) }}</div>
              <div class="muted-line">{{ shortStateVersion(row.appliedStateVersion) }}</div>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="pendingCmds" label="Pending" width="90" />
        <el-table-column prop="lastSeen" label="Last Seen" width="150" />
        <el-table-column label="Actions" width="180" fixed="right">
          <template #default="{ row }">
            <div class="action-buttons">
              <el-button
                size="small"
                link
                @click="viewNodeDetails(row)"
              >
                <el-icon><View /></el-icon>
              </el-button>
              <el-button
                size="small"
                link
                type="primary"
                @click="editNode(row)"
              >
                <el-icon><Edit /></el-icon>
              </el-button>
              <el-popconfirm
                title="Are you sure to delete this node?"
                @confirm="deleteNode(row.id)"
              >
                <template #reference>
                  <el-button
                    size="small"
                    link
                    type="danger"
                  >
                    <el-icon><Delete /></el-icon>
                  </el-button>
                </template>
              </el-popconfirm>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination-wrapper">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :page-sizes="[10, 20, 50, 100]"
          :total="filteredNodes.length"
          layout="sizes, prev, pager, next, jumper, total"
          @size-change="handleSizeChange"
          @current-change="handleCurrentChange"
        />
      </div>
    </el-card>

    <!-- 节点详情对话框 -->
    <el-dialog
      v-model="detailDialogVisible"
      title="Node Details"
      width="60%"
      :before-close="closeDetailDialog"
      class="node-detail-dialog"
    >
      <div v-if="selectedNode" class="node-detail-content">
        <div class="detail-toolbar">
          <el-button size="small" type="primary" :loading="commandLoading" @click="runQuickCommand('sync')">
            Sync
          </el-button>
          <el-button size="small" :loading="commandLoading" @click="runQuickCommand('health_check')">
            Health Check
          </el-button>
          <el-button size="small" :loading="commandLoading" @click="runQuickCommand('config_reload')">
            Reload Config
          </el-button>
        </div>

        <!-- 基本信息 -->
        <div class="detail-section">
          <h4 class="section-title">
            <el-icon><InfoFilled /></el-icon>
            Basic Information
          </h4>
          <el-descriptions :column="2" border>
            <el-descriptions-item label="Hostname">
              {{ selectedNode.hostname }}
            </el-descriptions-item>
            <el-descriptions-item label="Public IP">
              {{ selectedNode.publicIp }}
            </el-descriptions-item>
            <el-descriptions-item label="VPN IP">
              {{ selectedNode.vpnIp }}
            </el-descriptions-item>
            <el-descriptions-item label="Region">
              <span class="region-badge">{{ selectedNode.region.toUpperCase() }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="Mode">
              <div class="mode-badge">
                {{ selectedNode.mode }}
                <el-tag
                  v-if="selectedNode.mode === 'kernel'"
                  size="small"
                  type="success"
                  effect="plain"
                >
                  Optimized
                </el-tag>
              </div>
            </el-descriptions-item>
            <el-descriptions-item label="Status">
              <span class="status-badge" :class="selectedNode.status">
                {{ selectedNode.status }}
              </span>
            </el-descriptions-item>
            <el-descriptions-item label="Version">
              {{ selectedNode.version }}
            </el-descriptions-item>
            <el-descriptions-item label="Last Seen">
              {{ selectedNode.lastSeen }}
            </el-descriptions-item>
            <el-descriptions-item label="Uptime" :span="2">
              {{ selectedNode.uptime }}
            </el-descriptions-item>
            <el-descriptions-item label="Config Status">
              <el-tag size="small" :type="getConfigTagType(selectedNode.configurationStatus)">
                {{ formatConfigStatus(selectedNode.configurationStatus) }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="Pending Commands">
              {{ selectedNode.pendingCmds }}
            </el-descriptions-item>
            <el-descriptions-item label="Last Sync">
              {{ selectedNode.lastSyncAt }}
            </el-descriptions-item>
            <el-descriptions-item label="Last Command Status">
              <span v-if="selectedNode.lastCommandStatus">{{ selectedNode.lastCommandStatus }}</span>
              <span v-else>N/A</span>
            </el-descriptions-item>
            <el-descriptions-item label="Last Command Error" :span="2">
              {{ selectedNode.lastCommandError || 'N/A' }}
            </el-descriptions-item>
            <el-descriptions-item label="Desired Version">
              {{ selectedNode.desiredStateVersion || 'N/A' }}
            </el-descriptions-item>
            <el-descriptions-item label="Desired Updated">
              {{ selectedNode.desiredStateUpdatedAt || 'N/A' }}
            </el-descriptions-item>
            <el-descriptions-item label="Applied Version">
              {{ selectedNode.appliedStateVersion || 'N/A' }}
            </el-descriptions-item>
            <el-descriptions-item label="Applied Updated">
              {{ selectedNode.appliedStateUpdatedAt || 'N/A' }}
            </el-descriptions-item>
            <el-descriptions-item label="Observed State">
              <el-tag size="small" :type="getObservedTagType(selectedNode.observedState)">
                {{ formatObservedState(selectedNode.observedState) }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="Convergence">
              <el-tag size="small" :type="getConvergenceTagType(selectedNode.stateConvergence)">
                {{ formatConvergence(selectedNode.stateConvergence) }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="Observed At">
              {{ selectedNode.observedAt || 'N/A' }}
            </el-descriptions-item>
            <el-descriptions-item label="Observed Message" :span="2">
              {{ selectedNode.observedMessage || 'N/A' }}
            </el-descriptions-item>
          </el-descriptions>
        </div>

        <!-- 网络统计 -->
        <div class="detail-section">
          <h4 class="section-title">
            <el-icon><TrendCharts /></el-icon>
            Network Statistics
          </h4>
          <div class="stats-grid">
            <div class="stat-box">
              <div class="stat-icon-box upload">
                <el-icon><Upload /></el-icon>
              </div>
              <div class="stat-info">
                <div class="stat-box-value">{{ selectedNode.bandwidth.upload }} Mbps</div>
                <div class="stat-box-label">Upload</div>
              </div>
            </div>
            <div class="stat-box">
              <div class="stat-icon-box download">
                <el-icon><Download /></el-icon>
              </div>
              <div class="stat-info">
                <div class="stat-box-value">{{ selectedNode.bandwidth.download }} Mbps</div>
                <div class="stat-box-label">Download</div>
              </div>
            </div>
            <div class="stat-box">
              <div class="stat-icon-box latency">
                <el-icon><Timer /></el-icon>
              </div>
              <div class="stat-info">
                <div class="stat-box-value">{{ selectedNode.latency }} ms</div>
                <div class="stat-box-label">Latency</div>
              </div>
            </div>
          </div>
        </div>

        <!-- 路由信息 -->
        <div class="detail-section">
          <h4 class="section-title">
            <el-icon><Position /></el-icon>
            Advertised Routes
          </h4>
          <div class="routes-list">
            <el-tag
              v-for="route in selectedNode.routes"
              :key="route"
              size="large"
              type="info"
              effect="plain"
              class="route-tag"
            >
              {{ route }}
            </el-tag>
          </div>
        </div>

        <div class="detail-section">
          <h4 class="section-title">
            <el-icon><Timer /></el-icon>
            Recent Commands
          </h4>
          <el-table
            :data="selectedNode.recentCommands || []"
            size="small"
            empty-text="No commands yet"
          >
            <el-table-column prop="command" label="Command" min-width="120" />
            <el-table-column prop="status" label="Status" width="120" />
            <el-table-column prop="message" label="Message" min-width="220" />
            <el-table-column label="Created" width="180">
              <template #default="{ row }">
                {{ formatCommandTime(row.created_at) }}
              </template>
            </el-table-column>
          </el-table>
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import {
  Search,
  Refresh,
  Plus,
  Monitor,
  CircleCheck,
  CircleClose,
  Setting,
  View,
  Edit,
  Delete,
  InfoFilled,
  TrendCharts,
  Upload,
  Download,
  Timer,
  Position
} from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import useNodeStore from '../stores/node'
import { useAgentProxyApi } from '../composables/useAgentProxyApi'
import { useMonitorApi } from '../composables/useMonitorApi'

// 使用节点 store
const nodeStore = useNodeStore()

// 节点数据从 store 获取
const nodes = computed(() => nodeStore.nodes)
const loading = computed(() => nodeStore.loading)

const searchQuery = ref('')
const currentPage = ref(1)
const pageSize = ref(10)
const detailDialogVisible = ref(false)
const selectedNode = ref(null)
const commandLoading = ref(false)

// 计算属性
const onlineCount = computed(() => nodes.value.filter(n => n.status === 'online').length)
const offlineCount = computed(() => nodes.value.filter(n => n.status === 'offline').length)
const maintenanceCount = computed(() => nodes.value.filter(n => n.status === 'maintenance').length)

const filteredNodes = computed(() => {
  if (!searchQuery.value) {
    return nodes.value
  }
  const query = searchQuery.value.toLowerCase()
  return nodes.value.filter(node =>
    node.hostname.toLowerCase().includes(query) ||
    node.publicIp.includes(query) ||
    node.vpnIp.includes(query) ||
    node.region.toLowerCase().includes(query)
  )
})

const paginatedNodes = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  const end = start + pageSize.value
  return filteredNodes.value.slice(start, end)
})

// 方法
const refreshNodes = async () => {
  await nodeStore.loadNodes()
}

const addNode = () => {
  ElMessageBox.alert('Add node functionality will be implemented here.', 'Info', {
    confirmButtonText: 'OK',
  })
}

const viewNodeDetails = async (node) => {
  try {
    selectedNode.value = await nodeStore.loadNodeDetail(node.id)
    // Fetch real bandwidth/latency metrics
    try {
      const metrics = await useMonitorApi.getNodeMetrics(node.id)
      if (metrics && selectedNode.value) {
        selectedNode.value.bandwidth = {
          upload: metrics.upload_mbps != null ? Number(metrics.upload_mbps.toFixed(2)) : 'N/A',
          download: metrics.download_mbps != null ? Number(metrics.download_mbps.toFixed(2)) : 'N/A'
        }
        selectedNode.value.latency = metrics.latency_ms != null ? Number(metrics.latency_ms.toFixed(1)) : 'N/A'
      }
    } catch (metricsError) {
      console.error('Failed to load node metrics:', metricsError)
      if (selectedNode.value) {
        selectedNode.value.bandwidth = { upload: 'N/A', download: 'N/A' }
        selectedNode.value.latency = 'N/A'
      }
    }
    detailDialogVisible.value = true
  } catch (error) {
    console.error('Failed to load node detail:', error)
    ElMessage.error('Failed to load node details')
  }
}

const editNode = (node) => {
  ElMessageBox.alert(`Edit node functionality for ${node.hostname} will be implemented here.`, 'Info', {
    confirmButtonText: 'OK',
  })
}

const deleteNode = (id) => {
  nodes.value = nodes.value.filter(node => node.id !== id)
  ElMessage.success('Node deleted')
}

const closeDetailDialog = () => {
  detailDialogVisible.value = false
  selectedNode.value = null
}

const handleSizeChange = (size) => {
  pageSize.value = size
  currentPage.value = 1
}

const handleCurrentChange = (page) => {
  currentPage.value = page
}

const reloadSelectedNode = async () => {
  if (!selectedNode.value?.id) return
  selectedNode.value = await nodeStore.loadNodeDetail(selectedNode.value.id)
}

const runQuickCommand = async (command) => {
  if (!selectedNode.value?.id) return

  commandLoading.value = true
  try {
    await useAgentProxyApi.sendAgentCommand(selectedNode.value.id, {
      command,
      params: {},
      timeout: 30
    })
    ElMessage.success(`${command} queued`)
    await reloadSelectedNode()
  } catch (error) {
    console.error(`Failed to queue ${command}:`, error)
    ElMessage.error(`Failed to queue ${command}`)
  } finally {
    commandLoading.value = false
  }
}

const formatConfigStatus = (status) => {
  const map = {
    applied: 'Applied',
    pending: 'Pending',
    in_progress: 'In Progress',
    error: 'Error',
    idle: 'Idle'
  }
  return map[status] || status || 'Unknown'
}

const getConfigTagType = (status) => {
  switch (status) {
    case 'applied':
      return 'success'
    case 'pending':
    case 'in_progress':
      return 'warning'
    case 'error':
      return 'danger'
    default:
      return 'info'
  }
}

const formatObservedState = (state) => {
  const map = {
    applied: 'Applied',
    healthy: 'Healthy',
    error: 'Error',
    in_progress: 'In Progress',
    idle: 'Idle'
  }
  return map[state] || state || 'Unknown'
}

const getObservedTagType = (state) => {
  switch (state) {
    case 'applied':
    case 'healthy':
      return 'success'
    case 'in_progress':
      return 'warning'
    case 'error':
      return 'danger'
    default:
      return 'info'
  }
}

const formatConvergence = (state) => {
  const map = {
    converged: 'Converged',
    pending: 'Pending',
    diverged: 'Diverged',
    idle: 'Idle'
  }
  return map[state] || state || 'Unknown'
}

const getConvergenceTagType = (state) => {
  switch (state) {
    case 'converged':
      return 'success'
    case 'pending':
      return 'warning'
    case 'diverged':
      return 'danger'
    default:
      return 'info'
  }
}

const shortStateVersion = (value) => {
  if (!value) return 'N/A'
  return value.length > 18 ? `${value.slice(0, 18)}...` : value
}

const formatCommandTime = (value) => {
  if (!value) return 'N/A'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return 'N/A'
  return date.toLocaleString()
}

onMounted(() => {
  refreshNodes()
})
</script>

<style scoped>
/* ============================================
   Nodes Page Styles
   ============================================ */
.nodes-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.detail-toolbar {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 4px;
}

.state-version-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 12px;
}

.muted-line {
  color: var(--aria-text-secondary);
}

/* ============================================
   Page Header
   ============================================ */
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 20px;
  flex-wrap: wrap;
}

.header-content {
  flex: 1;
  min-width: 300px;
}

.page-title {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 24px;
  font-weight: 700;
  margin: 0 0 8px 0;
  color: var(--aria-text-primary);
  letter-spacing: -0.3px;
}

.title-icon {
  font-size: 28px;
  color: var(--aria-primary);
}

.page-subtitle {
  font-size: 14px;
  color: var(--aria-text-secondary);
  margin: 0;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.search-input {
  width: 280px;
}

/* ============================================
   Stats Cards
   ============================================ */
.stats-cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 16px;
}

.stat-item {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 20px;
  background: var(--aria-bg-secondary);
  border: 1px solid var(--aria-border-primary);
  border-radius: var(--aria-radius-lg);
  transition: all var(--aria-transition-base);
}

.stat-item:hover {
  border-color: var(--aria-border-hover);
  transform: translateY(-2px);
  box-shadow: var(--aria-shadow);
}

.stat-icon {
  width: 56px;
  height: 56px;
  border-radius: var(--aria-radius-md);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 28px;
  flex-shrink: 0;
}

.stat-icon.blue {
  background: rgba(59, 130, 246, 0.15);
  color: #3B82F6;
}

.stat-icon.green {
  background: rgba(34, 197, 94, 0.15);
  color: #22C55E;
}

.stat-icon.orange {
  background: rgba(245, 158, 11, 0.15);
  color: #F59E0B;
}

.stat-icon.purple {
  background: rgba(139, 92, 246, 0.15);
  color: #8B5CF6;
}

.stat-content {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.stat-value {
  font-size: 28px;
  font-weight: 700;
  color: var(--aria-text-primary);
  line-height: 1;
}

.stat-label {
  font-size: 13px;
  font-weight: 500;
  color: var(--aria-text-secondary);
}

/* ============================================
   Nodes Table
   ============================================ */
.nodes-card {
  background: var(--aria-bg-secondary);
  border: 1px solid var(--aria-border-primary);
  border-radius: var(--aria-radius-lg);
}

:deep(.nodes-table) {
  background: transparent;
}

:deep(.nodes-table .el-table__header) {
  background: var(--aria-bg-tertiary);
}

:deep(.nodes-table .el-table__header th) {
  background: transparent;
  color: var(--aria-text-secondary);
  font-weight: 600;
  font-size: 13px;
  border-bottom: 1px solid var(--aria-border-primary);
}

:deep(.nodes-table .el-table__body tr) {
  background: transparent;
  transition: background-color var(--aria-transition-fast);
}

:deep(.nodes-table .el-table__body tr:hover > td) {
  background: var(--aria-bg-tertiary);
}

:deep(.nodes-table .el-table__body td) {
  border-bottom: 1px solid var(--aria-border-primary);
  color: var(--aria-text-secondary);
}

/* Region Badge */
.region-badge {
  display: inline-block;
  padding: 4px 10px;
  background: rgba(59, 130, 246, 0.1);
  color: #3B82F6;
  border-radius: var(--radius-full);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.5px;
}

/* Mode Badge */
.mode-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

/* Status Badge */
.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 12px;
  border-radius: var(--radius-full);
  font-size: 12px;
  font-weight: 500;
}

.status-badge::before {
  content: '';
  width: 6px;
  height: 6px;
  border-radius: 50%;
}

.status-badge.online {
  background: rgba(34, 197, 94, 0.15);
  color: #22C55E;
}

.status-badge.online::before {
  background: #22C55E;
  animation: pulse-dot 2s ease-in-out infinite;
}

.status-badge.offline {
  background: rgba(239, 68, 68, 0.15);
  color: #EF4444;
}

.status-badge.offline::before {
  background: #EF4444;
}

.status-badge.maintenance {
  background: rgba(245, 158, 11, 0.15);
  color: #F59E0B;
}

.status-badge.maintenance::before {
  background: #F59E0B;
}

@keyframes pulse-dot {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

/* Action Buttons */
.action-buttons {
  display: flex;
  align-items: center;
  gap: 4px;
}

/* Pagination */
.pagination-wrapper {
  display: flex;
  justify-content: flex-end;
  padding-top: 20px;
  border-top: 1px solid var(--aria-border-primary);
}

/* ============================================
   Node Detail Dialog
   ============================================ */
:deep(.node-detail-dialog) {
  background: var(--aria-bg-secondary);
}

:deep(.node-detail-dialog .el-dialog__header) {
  border-bottom: 1px solid var(--aria-border-primary);
  padding: 20px 24px;
}

:deep(.node-detail-dialog .el-dialog__title) {
  color: var(--aria-text-primary);
  font-weight: 600;
}

:deep(.node-detail-dialog .el-dialog__body) {
  padding: 24px;
}

.node-detail-content {
  max-height: 600px;
  overflow-y: auto;
}

/* Detail Section */
.detail-section {
  margin-bottom: 32px;
}

.section-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 600;
  color: var(--aria-text-primary);
  margin: 0 0 16px 0;
}

.section-title .el-icon {
  color: var(--aria-primary);
}

:deep(.detail-section .el-descriptions) {
  --el-descriptions-table-border-color: var(--aria-border-primary);
  --el-descriptions-item-bordered-label-bg: var(--aria-bg-tertiary);
}

:deep(.detail-section .el-descriptions__label) {
  color: var(--aria-text-secondary);
  font-weight: 500;
}

:deep(.detail-section .el-descriptions__content) {
  color: var(--aria-text-primary);
}

/* Stats Grid */
.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 16px;
}

.stat-box {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 20px;
  background: var(--aria-bg-tertiary);
  border: 1px solid var(--aria-border-primary);
  border-radius: var(--aria-radius-lg);
}

.stat-icon-box {
  width: 48px;
  height: 48px;
  border-radius: var(--aria-radius-md);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 24px;
  flex-shrink: 0;
}

.stat-icon-box.upload {
  background: rgba(59, 130, 246, 0.15);
  color: #3B82F6;
}

.stat-icon-box.download {
  background: rgba(34, 197, 94, 0.15);
  color: #22C55E;
}

.stat-icon-box.latency {
  background: rgba(245, 158, 11, 0.15);
  color: #F59E0B;
}

.stat-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.stat-box-value {
  font-size: 20px;
  font-weight: 700;
  color: var(--aria-text-primary);
  line-height: 1;
}

.stat-box-label {
  font-size: 12px;
  font-weight: 500;
  color: var(--aria-text-secondary);
}

/* Routes List */
.routes-list {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.route-tag {
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
}

/* ============================================
   Responsive
   ============================================ */
@media (max-width: 1024px) {
  .page-header {
    flex-direction: column;
  }

  .header-actions {
    width: 100%;
  }

  .search-input {
    flex: 1;
    width: auto;
  }
}

@media (max-width: 768px) {
  .stats-cards {
    grid-template-columns: repeat(2, 1fr);
  }

  .stats-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 480px) {
  .stats-cards {
    grid-template-columns: 1fr;
  }

  .action-buttons {
    flex-direction: column;
  }
}
</style>
