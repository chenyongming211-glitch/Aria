<!-- src/views/Monitoring.vue -->
<template>
  <div class="monitoring page-shell">
    <PageHeader
      title="Monitoring Center"
      subtitle="Track alerts, certificate risk, sync health, policy delivery, and command outcomes across the tenant."
    >
      <template #actions>
        <el-button type="primary" :icon="Refresh" :loading="refreshing" @click="refreshAll">
          Refresh
        </el-button>
      </template>
    </PageHeader>

    <MetricStrip :metrics="monitorMetricItems" @select="handleMetricSelect" />

    <FilterBar>
      <template #filters>
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
          <el-option label="Certificate Expiring" value="certificate_expiring" />
          <el-option label="Certificate Expired" value="certificate_expired" />
          <el-option label="Certificate Renew Failed" value="certificate_renew_failed" />
          <el-option label="Certificate Renewed" value="certificate_renewed" />
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
      </template>
    </FilterBar>

    <DataPanel
      ref="alertsSectionRef"
      class="alerts-card"
      title="Active Alerts"
      subtitle="Current tenant alerts that need operator attention or explicit resolution."
      v-loading="alertsLoading"
    >
      <template #actions>
        <el-tag size="small" type="danger" v-if="filteredAlerts.length > 0">{{ filteredAlerts.length }} open</el-tag>
        <el-tag size="small" type="warning" v-if="alertsFilterMode === 'certificate'">Certificate focus</el-tag>
        <el-button size="small" text @click="setAlertsFilterMode('all')">All Alerts</el-button>
        <el-button size="small" text type="warning" @click="setAlertsFilterMode('certificate')">
          Cert Alerts
        </el-button>
      </template>

      <el-empty v-if="filteredAlerts.length === 0 && !alertsLoading" description="No active alerts" />

      <el-table
        v-else
        :data="filteredAlerts"
        size="small"
        style="width: 100%"
      >
        <el-table-column prop="severity" label="Severity" width="120">
          <template #default="{ row }">
            <StatusBadge :status="row.severity || 'info'" :label="row.severity || 'info'" :tone="severityTone(row.severity)" />
          </template>
        </el-table-column>
        <el-table-column prop="alert_type" label="Type" width="160" />
        <el-table-column prop="title" label="Title" min-width="180" show-overflow-tooltip />
        <el-table-column prop="message" label="Message" min-width="220" show-overflow-tooltip />
        <el-table-column label="Context" min-width="220">
          <template #default="{ row }">
            <div class="context-tags">
              <el-tag v-if="row.context?.policy_ref" size="small" effect="plain">{{ row.context.policy_ref }}</el-tag>
              <el-tag v-if="row.context?.command_id" size="small" effect="plain">{{ shortId(row.context.command_id) }}</el-tag>
              <el-tag v-if="row.context?.not_after" size="small" type="warning" effect="plain">
                Exp: {{ formatTime(row.context.not_after) }}
              </el-tag>
              <span
                v-if="!row.context?.policy_ref && !row.context?.command_id && !row.context?.not_after"
                class="muted-text"
              >
                -
              </span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="Actions" width="430">
          <template #default="{ row }">
            <div class="table-actions">
              <el-button v-if="row.node_id" size="small" @click="goToNodeFromAlert(row)">
                View Node
              </el-button>
              <el-button v-if="row.context?.policy_ref" size="small" @click="goToPolicyFromContext(row.node_id, row.context)">
                View Policy
              </el-button>
              <el-button
                v-if="isActionableAlert(row) && hasPermission('commands:write')"
                size="small"
                type="primary"
                plain
                :loading="commandActionKey === alertCommandKey(row, 'sync')"
                @click="handleAlertCommand(row, 'sync')"
              >
                Run Sync
              </el-button>
              <el-button
                v-if="isActionableAlert(row) && hasPermission('commands:write')"
                size="small"
                plain
                :loading="commandActionKey === alertCommandKey(row, 'health_check')"
                @click="handleAlertCommand(row, 'health_check')"
              >
                Health Check
              </el-button>
              <el-button
                v-if="isActionableAlert(row) && hasPermission('ai:use')"
                size="small"
                type="success"
                plain
                @click="askAIForAlert(row)"
              >
                Ask AI
              </el-button>
              <el-button
                v-if="hasPermission('commands:write')"
                size="small"
                type="warning"
                plain
                :loading="resolvingId === row.id"
                @click="handleResolve(row)"
              >
                Resolve
              </el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </DataPanel>

    <!-- Event Feed Timeline -->
    <DataPanel
      ref="eventsSectionRef"
      class="events-card"
      title="Event Feed"
      subtitle="Audit events and alert events merged into a single operations timeline."
      v-loading="eventsLoading"
    >
      <template #actions>
        <el-tag size="small" type="info" v-if="eventsTotal > 0">{{ eventsTotal }} total</el-tag>
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
                <StatusBadge
                  :status="event.event_type"
                  :label="formatEventType(event.event_type)"
                  :tone="eventTypeTone(event.event_type)"
                />
                <StatusBadge
                  v-if="event.source === 'alert' && event.severity"
                  :status="event.severity"
                  :label="event.severity"
                  :tone="severityTone(event.severity)"
                />
              </div>
              <span class="event-time">{{ formatTime(event.created_at) }}</span>
            </div>
            <div class="event-title">{{ event.title }}</div>
            <div v-if="event.detail && Object.keys(event.detail).length > 0" class="event-context-row">
              <el-tag v-if="event.detail.policy_ref" size="small" effect="plain">
                Policy: {{ event.detail.policy_ref }}
              </el-tag>
              <el-tag v-if="event.detail.command_id" size="small" effect="plain">
                Command: {{ shortId(event.detail.command_id) }}
              </el-tag>
              <el-tag v-if="event.detail.policy_domain" size="small" effect="plain">
                Domain: {{ event.detail.policy_domain }}
              </el-tag>
              <el-tag v-if="event.detail.not_after" size="small" type="warning" effect="plain">
                Exp: {{ formatTime(event.detail.not_after) }}
              </el-tag>
              <el-tag v-if="event.detail.renewed_from" size="small" type="success" effect="plain">
                Renewed
              </el-tag>
            </div>
            <div class="event-actions-row">
              <span
                v-if="event.node_id"
                class="event-node-link"
                @click="goToNodeFromEvent(event)"
              >
                Node: {{ event.node_id.substring(0, 8) }}…
              </span>
              <el-button
                v-if="event.node_id"
                size="small"
                @click="goToNodeFromEvent(event)"
              >
                View Node
              </el-button>
              <el-button
                v-if="event.detail?.policy_ref"
                size="small"
                @click="goToPolicyFromContext(event.node_id, event.detail)"
              >
                View Policy
              </el-button>
              <el-button
                v-if="event.detail?.command_id && event.node_id"
                size="small"
                @click="goToNodeFromEvent(event, 'commands')"
              >
                View Command
              </el-button>
              <el-button
                v-if="event.source === 'alert' && event.severity && hasPermission('commands:write')"
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
    </DataPanel>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import {
  Refresh
} from '@element-plus/icons-vue'
import { useMonitorApi } from '@/composables/useMonitorApi'
import { useAgentProxyApi } from '@/composables/useAgentProxyApi'
import { usePermission } from '../composables/usePermission'
import { useTenantChangeReload } from '@/composables/useTenantChangeReload'
import { ElMessage } from 'element-plus'
import PageHeader from '@/components/ui/PageHeader.vue'
import MetricStrip from '@/components/ui/MetricStrip.vue'
import FilterBar from '@/components/ui/FilterBar.vue'
import DataPanel from '@/components/ui/DataPanel.vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'

const router = useRouter()
const { hasPermission } = usePermission()

type AnyRecord = Record<string, any>
type SectionRef = { value?: any } | any

interface MonitorStats {
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

interface EventParams {
  limit: number
  offset: number
  event_type?: string
  severity?: string
}

const errorMessage = (error: unknown, fallback = '未知错误'): string =>
  error instanceof Error ? error.message : (typeof error === 'string' ? error : fallback)

// --- State ---
const statsLoading = ref(false)
const eventsLoading = ref(false)
const alertsLoading = ref(false)
const refreshing = ref(false)
const resolvingId = ref<string | null>(null)
const commandActionKey = ref('')
const alertsSectionRef = ref<SectionRef | null>(null)
const eventsSectionRef = ref<SectionRef | null>(null)

const stats = ref<MonitorStats>({
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

const events = ref<AnyRecord[]>([])
const alerts = ref<AnyRecord[]>([])
const eventsTotal = ref(0)
const eventsPage = ref(1)
const eventsLimit = 50
const alertsFilterMode = ref('all')

const filterEventType = ref('')
const filterSeverity = ref('')
const certificateAlertTypes = ['certificate_expiring', 'certificate_expired', 'certificate_renew_failed']
const actionableAlertTypes = ['sync_failed', 'policy_failed', 'command_failed', 'node_offline']

// --- Computed ---
const monitorMetricItems = computed(() => [
  {
    key: 'nodes',
    label: 'Nodes',
    value: `${stats.value.online_nodes} / ${stats.value.total_nodes}`,
    meta: 'Online / total',
    status: stats.value.offline_nodes > 0 ? 'warning' : 'success',
    clickable: isStatCardClickable('nodes')
  },
  {
    key: 'sync',
    label: 'Sync Rate',
    value: `${stats.value.sync_success_rate.toFixed(1)}%`,
    meta: 'Recent sync success',
    status: stats.value.sync_success_rate >= 99 ? 'success' : 'warning'
  },
  {
    key: 'peers',
    label: 'Peers',
    value: stats.value.total_peers,
    meta: 'WireGuard peers',
    status: 'info'
  },
  {
    key: 'acl',
    label: 'ACL Rules',
    value: stats.value.total_acl_rules,
    meta: 'Tenant policies',
    status: 'info'
  },
  {
    key: 'qos',
    label: 'QoS Rules',
    value: stats.value.total_qos_rules,
    meta: 'Bandwidth policies',
    status: 'info'
  },
  {
    key: 'failed',
    label: 'Failed Cmds',
    value: stats.value.failed_commands_count,
    meta: 'Click to filter events',
    status: stats.value.failed_commands_count > 0 ? 'danger' : 'success',
    clickable: isStatCardClickable('failed')
  },
  {
    key: 'alerts',
    label: 'Active Alerts',
    value: stats.value.active_alerts_count,
    meta: 'Open alert queue',
    status: stats.value.active_alerts_count > 0 ? 'danger' : 'success',
    clickable: isStatCardClickable('alerts')
  },
  {
    key: 'certificates',
    label: 'Cert Alerts',
    value: certificateAlertsCount.value,
    meta: 'Certificate focus',
    status: certificateAlertsCount.value > 0 ? 'warning' : 'success',
    clickable: isStatCardClickable('certificates')
  }
])

const filteredAlerts = computed(() => {
  if (alertsFilterMode.value === 'certificate') {
    return alerts.value.filter((alert) => isCertificateAlert(alert?.alert_type))
  }
  return alerts.value
})

const certificateAlertsCount = computed(() => (
  alerts.value.filter((alert) => isCertificateAlert(alert?.alert_type)).length
))

const isStatCardClickable = (key?: string) => ['nodes', 'failed', 'alerts', 'certificates'].includes(key || '')

const handleMetricSelect = (metric: AnyRecord = {}) => handleStatCardClick(metric.key)

const normalizeContextRecord = (context: unknown): AnyRecord => {
  if (!context || typeof context !== 'object' || Array.isArray(context)) {
    return {}
  }
  return context as AnyRecord
}

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
    const params: EventParams = {
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

const loadAlerts = async () => {
  try {
    alertsLoading.value = true
    const data = await useMonitorApi.getAlerts({ status: 'active', limit: 100 })
    alerts.value = data?.items || []
  } catch (e) {
    console.error('Failed to load alerts:', e)
    alerts.value = []
  } finally {
    alertsLoading.value = false
  }
}

const refreshAll = async () => {
  refreshing.value = true
  await Promise.all([loadStats(), loadEvents(), loadAlerts()])
  refreshing.value = false
}

const scrollToSection = async (sectionRef: SectionRef | null) => {
  await nextTick()
  const target = sectionRef?.value?.$el || sectionRef?.value
  if (typeof target?.scrollIntoView === 'function') {
    target.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }
}

const onPageChange = () => {
  loadEvents()
}

const buildResolvePayload = (alert: AnyRecord = {}) => {
  const context = normalizeContextRecord(alert.context)
  return {
    source: 'monitoring',
    reason: 'Resolved from Monitoring',
    ...(context.command_id ? { command_id: context.command_id } : {})
  }
}

const handleResolve = async (alert: AnyRecord | string) => {
  if (!hasPermission('commands:write')) {
    ElMessage.error('Missing command permission')
    return
  }
  const alertId = typeof alert === 'string' ? alert : alert?.id
  if (!alertId) {
    ElMessage.error('Alert context is missing')
    return
  }

  try {
    resolvingId.value = alertId
    await useMonitorApi.resolveAlert(alertId, buildResolvePayload(typeof alert === 'string' ? {} : alert))
    ElMessage.success('Alert resolved')
    await Promise.all([loadStats(), loadEvents(), loadAlerts()])
  } catch (e) {
    ElMessage.error('Failed to resolve alert')
  } finally {
    resolvingId.value = null
  }
}

const isCertificateAlert = (alertType = '') => certificateAlertTypes.includes(alertType)

const isActionableAlert = (alert: AnyRecord = {}) => (
  Boolean(alert?.node_id) && actionableAlertTypes.includes(alert?.alert_type)
)

const alertCommandKey = (alert: AnyRecord = {}, command = '') => `${alert?.id || alert?.node_id || 'alert'}:${command}`

const buildAlertCommandParams = (alert: AnyRecord = {}) => {
  const context = normalizeContextRecord(alert.context)
  return {
    source: 'monitoring',
    alert_id: alert.id || '',
    event_type: alert.alert_type || '',
    ...(context.command_id ? { command_id: context.command_id } : {}),
    ...(context.policy_ref ? { policy_ref: context.policy_ref } : {}),
    ...(context.policy_domain ? { policy_domain: context.policy_domain } : {})
  }
}

const commandIdFromResponse = (response: AnyRecord = {}) => (
  response.command_id || response.commandId || response.id || ''
)

const openQueuedCommandFromAlert = (alert: AnyRecord = {}, commandId = '') => {
  if (!alert?.node_id) return
  const context = normalizeContextRecord(alert.context)
  router.push({
    name: 'NodeMonitorDetail',
    params: { nodeId: alert.node_id },
    query: buildNodeQuery({
      alertId: alert.id,
      eventType: alert.alert_type,
      command_id: commandId || context.command_id,
      policy_ref: context.policy_ref,
      policy_domain: context.policy_domain
    }, 'commands')
  })
}

const buildAlertAIQuery = (alert: AnyRecord = {}) => {
  const context = normalizeContextRecord(alert.context)
  return {
    source: 'monitoring',
    ...(alert.node_id ? { nodeId: alert.node_id } : {}),
    ...(alert.id ? { alertId: alert.id } : {}),
    ...(alert.alert_type ? { eventType: alert.alert_type } : {}),
    ...(context.command_id ? { commandId: context.command_id } : {}),
    ...(context.policy_ref ? { policyRef: context.policy_ref } : {}),
    ...(context.policy_domain ? { policyDomain: context.policy_domain } : {})
  }
}

const askAIForAlert = (alert: AnyRecord = {}) => {
  if (!hasPermission('ai:use')) {
    ElMessage.error('Missing AI permission')
    return
  }
  router.push({
    name: 'AiAssistant',
    query: buildAlertAIQuery(alert)
  })
}

const handleAlertCommand = async (alert: AnyRecord, command: string) => {
  if (!hasPermission('commands:write')) {
    ElMessage.error('Missing command permission')
    return
  }
  if (!alert?.node_id) {
    ElMessage.error('Alert does not reference a node')
    return
  }

  const actionKey = alertCommandKey(alert, command)
  commandActionKey.value = actionKey
  try {
    const response = await useAgentProxyApi.sendAgentCommand(alert.node_id, {
      command,
      params: buildAlertCommandParams(alert),
      timeout: 30
    } as any)
    ElMessage.success(`${command} queued`)
    await Promise.all([loadStats(), loadEvents(), loadAlerts()])
    openQueuedCommandFromAlert(alert, commandIdFromResponse(response))
  } catch (e) {
    console.error(`Failed to queue ${command} from alert:`, e)
    ElMessage.error(`Failed to queue ${command}`)
  } finally {
    if (commandActionKey.value === actionKey) {
      commandActionKey.value = ''
    }
  }
}

const setAlertsFilterMode = async (mode: string) => {
  alertsFilterMode.value = mode
  await loadAlerts()
  await scrollToSection(alertsSectionRef)
}

const goToNodeDetail = (nodeId: string) => {
  router.push({ name: 'NodeMonitorDetail', params: { nodeId } })
}

const handleStatCardClick = async (key?: string) => {
  if (!isStatCardClickable(key)) {
    return
  }

  switch (key) {
    case 'nodes':
      router.push({ name: 'Nodes' })
      break
    case 'failed':
      filterEventType.value = 'command_failed'
      eventsPage.value = 1
      await loadEvents()
      await scrollToSection(eventsSectionRef)
      break
    case 'alerts':
      await setAlertsFilterMode('all')
      break
    case 'certificates':
      filterEventType.value = 'certificate_expired'
      eventsPage.value = 1
      await loadEvents()
      await setAlertsFilterMode('certificate')
      break
    default:
      break
  }
}

const buildNodeQuery = (context: AnyRecord = {}, focus = '') => {
  const query: Record<string, string> = {}
  if (focus) {
    query.focus = focus
  }
  if (context.alertId) {
    query.alertId = context.alertId
  }
  if (context.eventId) {
    query.eventId = context.eventId
  }
  if (context.eventType) {
    query.eventType = context.eventType
  }
  if (context.command_id) {
    query.commandId = context.command_id
  }
  if (context.policy_ref) {
    query.policyRef = context.policy_ref
  }
  if (context.policy_domain) {
    query.policyDomain = context.policy_domain
  }
  return query
}

const defaultNodeFocus = (context: unknown = {}, eventType = '') => {
  const normalized = normalizeContextRecord(context)
  if (normalized.command_id) return 'commands'
  if (normalized.policy_ref) return 'policies'
  if (String(eventType).startsWith('certificate_')) return 'certificate'
  return 'alerts'
}

const goToNodeFromAlert = (alert: AnyRecord) => {
  if (!alert?.node_id) return
  const context = normalizeContextRecord(alert.context)
  router.push({
    name: 'NodeMonitorDetail',
    params: { nodeId: alert.node_id },
    query: buildNodeQuery({
      alertId: alert.id,
      eventType: alert.alert_type,
      command_id: context.command_id,
      policy_ref: context.policy_ref,
      policy_domain: context.policy_domain
    }, defaultNodeFocus(context, alert.alert_type))
  })
}

const goToNodeFromEvent = (event: AnyRecord, explicitFocus = '') => {
  if (!event?.node_id) return
  const detail = normalizeContextRecord(event.detail)
  router.push({
    name: 'NodeMonitorDetail',
    params: { nodeId: event.node_id },
    query: buildNodeQuery({
      eventId: event.id,
      eventType: event.event_type,
      command_id: detail.command_id,
      policy_ref: detail.policy_ref,
      policy_domain: detail.policy_domain
    }, explicitFocus || defaultNodeFocus(detail, event.event_type))
  })
}

const goToPolicyFromContext = (nodeId: string, context: AnyRecord = {}) => {
  const normalized = normalizeContextRecord(context)
  router.push({
    name: 'Policies',
    query: {
      ...(nodeId ? { nodeId } : {}),
      ...(normalized.policy_ref ? { policyRef: normalized.policy_ref } : {}),
      ...(normalized.policy_domain ? { kind: normalized.policy_domain } : {}),
      ...(normalized.command_id ? { commandId: normalized.command_id } : {})
    }
  })
}

const shortId = (value?: string) => {
  if (!value) return ''
  return value.length > 8 ? `${value.slice(0, 8)}...` : value
}

// --- Formatters ---
const formatTime = (iso?: string) => {
  if (!iso) return ''
  const d = new Date(iso)
  return d.toLocaleString()
}

const formatEventType = (type?: string) => {
  if (!type) return ''
  return type.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase())
}

const eventTypeTone = (type?: string) => {
  if (!type) return 'info'
  if (type.includes('failed')) return 'danger'
  if (type.includes('offline')) return 'danger'
  if (type.includes('renewed')) return 'success'
  if (type.includes('completed') || type.includes('delivered') || type.includes('online')) return 'success'
  if (type.includes('alert')) return 'warning'
  return 'info'
}

const severityTone = (severity?: string) => {
  switch (severity) {
    case 'critical': return 'danger'
    case 'warning': return 'warning'
    case 'info': return 'info'
    default: return 'info'
  }
}

const severityDotClass = (event: AnyRecord) => {
  if (event.source === 'alert') {
    return `dot-${event.severity || 'info'}`
  }
  if (event.event_type?.includes('failed') || event.event_type?.includes('offline')) return 'dot-critical'
  if (event.event_type?.includes('completed') || event.event_type?.includes('online')) return 'dot-success'
  return 'dot-info'
}

// --- Lifecycle ---
const reloadTenantScopedData = async () => {
  eventsPage.value = 1
  events.value = []
  alerts.value = []
  await refreshAll()
}

onMounted(() => {
  loadStats()
  loadEvents()
  loadAlerts()
})

useTenantChangeReload(reloadTenantScopedData)
</script>

<style scoped>
.monitoring {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.alerts-card :deep(.ui-data-panel__body),
.events-card :deep(.ui-data-panel__body) {
  padding: 12px 16px 16px;
}

.events-card {
  min-height: 300px;
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

.event-context-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 8px;
}

.event-actions-row {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
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

.table-actions,
.context-tags {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.muted-text {
  color: var(--aria-text-muted, #94A3B8);
  font-size: 12px;
}
</style>
