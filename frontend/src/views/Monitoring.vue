<!-- src/views/Monitoring.vue -->
<template>
  <div class="monitoring">
    <!-- Stats Cards -->
    <el-row :gutter="16" class="stats-row">
      <el-col :xs="12" :sm="8" :lg="4" v-for="card in statCards" :key="card.key">
        <div class="stat-card light-card" :class="`stat-card-${card.color}`">
          <div class="stat-icon-wrap">
            <el-icon :size="22"><component :is="card.icon" /></el-icon>
          </div>
          <div class="stat-value">{{ card.value }}</div>
          <div class="stat-label">{{ card.label }}</div>
        </div>
      </el-col>
    </el-row>

    <!-- Filter Bar + Refresh -->
    <el-card class="filter-card light-card" shadow="never">
      <div class="filter-bar">
        <div class="filter-left">
          <el-select
            v-model="filterEventType"
            placeholder="Event Type"
            clearable
            style="width: 180px"
            @change="loadEvents"
          >
            <el-option label="All Types" value="" />
            <el-option label="Node Offline" value="node_offline" />
            <el-option label="Node Online" value="node_online" />
            <el-option label="Sync Failed" value="sync_failed" />
            <el-option label="Policy Failed" value="policy_failed" />
            <el-option label="Command Completed" value="command_completed" />
            <el-option label="Command Failed" value="command_failed" />
            <el-option label="Policy Delivered" value="policy_delivered" />
            <el-option label="Alert Created" value="alert_created" />
            <el-option label="Alert Resolved" value="alert_resolved" />
          </el-select>
          <el-select
            v-model="filterSeverity"
            placeholder="Severity"
            clearable
            style="width: 150px"
            @change="loadEvents"
          >
            <el-option label="All Severities" value="" />
            <el-option label="Critical" value="critical" />
            <el-option label="Warning" value="warning" />
            <el-option label="Info" value="info" />
          </el-select>
        </div>
        <el-button type="primary" :icon="Refresh" :loading="refreshing" @click="refreshAll">
          Refresh
        </el-button>
      </div>
    </el-card>

    <!-- Event Feed Timeline -->
    <el-card class="events-card light-card" shadow="never" v-loading="eventsLoading">
      <template #header>
        <div class="card-header">
          <div class="header-left">
            <el-icon class="header-icon"><Clock /></el-icon>
            <span class="header-title">Event Feed</span>
            <el-tag size="small" type="info" v-if="eventsTotal > 0">{{ eventsTotal }} total</el-tag>
          </div>
        </div>
      </template>

      <div v-if="events.length === 0 && !eventsLoading" class="empty-state">
        <el-empty description="No events found" />
      </div>

      <div class="event-timeline" v-else>
        <div
          v-for="event in events"
          :key="event.id"
          class="event-item"
          :class="`event-${event.source}`"
        >
          <div class="event-indicator">
            <div class="event-dot" :class="severityDotClass(event)"></div>
            <div class="event-line"></div>
          </div>
          <div class="event-body">
            <div class="event-header-row">
              <div class="event-tags">
                <el-tag size="small" :type="eventTypeTagType(event.event_type)" effect="plain">
                  {{ formatEventType(event.event_type) }}
                </el-tag>
                <el-tag
                  v-if="event.source === 'alert' && event.severity"
                  size="small"
                  :type="severityTagType(event.severity)"
                >
                  {{ event.severity }}
                </el-tag>
              </div>
              <span class="event-time">{{ formatTime(event.created_at) }}</span>
            </div>
            <div class="event-title">{{ event.title }}</div>
            <div class="event-actions-row">
              <span
                v-if="event.node_id"
                class="event-node-link"
                @click="goToNodeDetail(event.node_id)"
              >
                Node: {{ event.node_id.substring(0, 8) }}…
              </span>
              <el-button
                v-if="event.source === 'alert' && event.severity"
                size="small"
                type="warning"
                plain
                :loading="resolvingId === event.id"
                @click="handleResolve(event.id)"
              >
                Resolve
              </el-button>
            </div>
          </div>
        </div>
      </div>

      <!-- Pagination -->
      <div class="events-pagination" v-if="eventsTotal > eventsLimit">
        <el-pagination
          v-model:current-page="eventsPage"
          :page-size="eventsLimit"
          :total="eventsTotal"
          layout="prev, pager, next"
          @current-change="onPageChange"
        />
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import {
  Refresh,
  Clock,
  Monitor,
  CircleCheck,
  Connection,
  Lock,
  Setting,
  Warning,
  Bell
} from '@element-plus/icons-vue'
import { useMonitorApi } from '@/composables/useMonitorApi'
import { ElMessage } from 'element-plus'

const router = useRouter()

// --- State ---
const statsLoading = ref(false)
const eventsLoading = ref(false)
const refreshing = ref(false)
const resolvingId = ref(null)

const stats = ref({
  total_nodes: 0,
  online_nodes: 0,
  offline_nodes: 0,
  sync_success_rate: 100,
  total_peers: 0,
  total_acl_rules: 0,
  total_qos_rules: 0,
  failed_commands_count: 0,
  active_alerts_count: 0
})

const events = ref([])
const eventsTotal = ref(0)
const eventsPage = ref(1)
const eventsLimit = 50

const filterEventType = ref('')
const filterSeverity = ref('')

// --- Computed ---
const statCards = computed(() => [
  {
    key: 'nodes',
    label: 'Nodes',
    value: `${stats.value.online_nodes} / ${stats.value.total_nodes}`,
    icon: Monitor,
    color: 'blue'
  },
  {
    key: 'sync',
    label: 'Sync Rate',
    value: `${stats.value.sync_success_rate.toFixed(1)}%`,
    icon: CircleCheck,
    color: 'green'
  },
  {
    key: 'peers',
    label: 'Peers',
    value: stats.value.total_peers,
    icon: Connection,
    color: 'cyan'
  },
  {
    key: 'acl',
    label: 'ACL Rules',
    value: stats.value.total_acl_rules,
    icon: Lock,
    color: 'orange'
  },
  {
    key: 'qos',
    label: 'QoS Rules',
    value: stats.value.total_qos_rules,
    icon: Setting,
    color: 'purple'
  },
  {
    key: 'failed',
    label: 'Failed Cmds',
    value: stats.value.failed_commands_count,
    icon: Warning,
    color: stats.value.failed_commands_count > 0 ? 'red' : 'green'
  },
  {
    key: 'alerts',
    label: 'Active Alerts',
    value: stats.value.active_alerts_count,
    icon: Bell,
    color: stats.value.active_alerts_count > 0 ? 'red' : 'green'
  }
])

// --- Methods ---
const loadStats = async () => {
  try {
    statsLoading.value = true
    const data = await useMonitorApi.getStats()
    if (data) {
      stats.value = { ...stats.value, ...data }
    }
  } catch (e) {
    console.error('Failed to load stats:', e)
  } finally {
    statsLoading.value = false
  }
}

const loadEvents = async () => {
  try {
    eventsLoading.value = true
    const params = {
      limit: eventsLimit,
      offset: (eventsPage.value - 1) * eventsLimit
    }
    if (filterEventType.value) params.event_type = filterEventType.value
    if (filterSeverity.value) params.severity = filterSeverity.value

    const data = await useMonitorApi.getEvents(params)
    if (data) {
      events.value = data.items || []
      eventsTotal.value = data.total || 0
    }
  } catch (e) {
    console.error('Failed to load events:', e)
    events.value = []
    eventsTotal.value = 0
  } finally {
    eventsLoading.value = false
  }
}

const refreshAll = async () => {
  refreshing.value = true
  await Promise.all([loadStats(), loadEvents()])
  refreshing.value = false
  ElMessage.success('Data refreshed')
}

const onPageChange = () => {
  loadEvents()
}

const handleResolve = async (alertId) => {
  try {
    resolvingId.value = alertId
    await useMonitorApi.resolveAlert(alertId)
    ElMessage.success('Alert resolved')
    await Promise.all([loadStats(), loadEvents()])
  } catch (e) {
    ElMessage.error('Failed to resolve alert')
  } finally {
    resolvingId.value = null
  }
}

const goToNodeDetail = (nodeId) => {
  router.push({ name: 'NodeMonitorDetail', params: { nodeId } })
}

// --- Formatters ---
const formatTime = (iso) => {
  if (!iso) return ''
  const d = new Date(iso)
  return d.toLocaleString()
}

const formatEventType = (type) => {
  if (!type) return ''
  return type.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase())
}

const eventTypeTagType = (type) => {
  if (!type) return 'info'
  if (type.includes('failed')) return 'danger'
  if (type.includes('offline')) return 'danger'
  if (type.includes('completed') || type.includes('delivered') || type.includes('online')) return 'success'
  if (type.includes('alert')) return 'warning'
  return 'info'
}

const severityTagType = (severity) => {
  switch (severity) {
    case 'critical': return 'danger'
    case 'warning': return 'warning'
    case 'info': return 'info'
    default: return 'info'
  }
}

const severityDotClass = (event) => {
  if (event.source === 'alert') {
    return `dot-${event.severity || 'info'}`
  }
  if (event.event_type?.includes('failed') || event.event_type?.includes('offline')) return 'dot-critical'
  if (event.event_type?.includes('completed') || event.event_type?.includes('online')) return 'dot-success'
  return 'dot-info'
}

// --- Lifecycle ---
onMounted(() => {
  loadStats()
  loadEvents()
})
</script>

<style scoped>
.monitoring {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* Stats Cards */
.stats-row {
  margin-bottom: 0;
}

.stat-card {
  padding: 16px;
  border-radius: var(--aria-radius-lg, 12px);
  text-align: center;
  transition: all 0.2s;
  margin-bottom: 12px;
}

.stat-card:hover {
  transform: translateY(-2px);
  box-shadow: var(--aria-shadow, 0 2px 8px rgba(0,0,0,0.08));
}

.stat-icon-wrap {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 0 auto 8px;
}

.stat-card-blue .stat-icon-wrap { background: rgba(59,130,246,0.1); color: #3B82F6; }
.stat-card-green .stat-icon-wrap { background: rgba(34,197,94,0.1); color: #22C55E; }
.stat-card-cyan .stat-icon-wrap { background: rgba(6,182,212,0.1); color: #06B6D4; }
.stat-card-orange .stat-icon-wrap { background: rgba(245,158,11,0.1); color: #F59E0B; }
.stat-card-purple .stat-icon-wrap { background: rgba(139,92,246,0.1); color: #8B5CF6; }
.stat-card-red .stat-icon-wrap { background: rgba(239,68,68,0.1); color: #EF4444; }

.stat-value {
  font-size: 22px;
  font-weight: 700;
  color: var(--aria-text-primary, #1E293B);
  margin-bottom: 2px;
}

.stat-label {
  font-size: 12px;
  color: var(--aria-text-muted, #94A3B8);
}

/* Filter Bar */
.filter-card {
  padding: 0;
}

.filter-card :deep(.el-card__body) {
  padding: 12px 16px;
}

.filter-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}

.filter-left {
  display: flex;
  gap: 10px;
}

/* Events Card */
.events-card {
  min-height: 300px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.header-icon {
  font-size: 18px;
  color: var(--aria-primary, #3B82F6);
}

.header-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--aria-text-primary, #1E293B);
}

/* Event Timeline */
.event-timeline {
  display: flex;
  flex-direction: column;
}

.event-item {
  display: flex;
  gap: 14px;
  padding: 12px 0;
}

.event-item + .event-item {
  border-top: 1px solid var(--aria-border-primary, #F1F5F9);
}

.event-indicator {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding-top: 4px;
  min-width: 16px;
}

.event-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex-shrink: 0;
}

.dot-critical { background: #EF4444; }
.dot-warning { background: #F59E0B; }
.dot-info { background: #3B82F6; }
.dot-success { background: #22C55E; }

.event-line {
  width: 2px;
  flex: 1;
  background: var(--aria-border-primary, #F1F5F9);
  margin-top: 4px;
}

.event-body {
  flex: 1;
  min-width: 0;
}

.event-header-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 4px;
}

.event-tags {
  display: flex;
  gap: 6px;
}

.event-time {
  font-size: 12px;
  color: var(--aria-text-muted, #94A3B8);
  white-space: nowrap;
}

.event-title {
  font-size: 14px;
  color: var(--aria-text-primary, #1E293B);
  margin-bottom: 6px;
  line-height: 1.5;
}

.event-actions-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.event-node-link {
  font-size: 12px;
  color: var(--aria-primary, #3B82F6);
  cursor: pointer;
  text-decoration: underline;
}

.event-node-link:hover {
  opacity: 0.8;
}

.events-pagination {
  display: flex;
  justify-content: center;
  padding-top: 16px;
}

.empty-state {
  padding: 40px 0;
}

/* Responsive */
@media (max-width: 768px) {
  .filter-bar {
    flex-direction: column;
    align-items: stretch;
  }
  .filter-left {
    flex-direction: column;
  }
}
</style>
