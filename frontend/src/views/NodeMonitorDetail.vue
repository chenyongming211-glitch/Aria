<!-- src/views/NodeMonitorDetail.vue -->
<template>
  <div class="node-monitor-detail" v-loading="loading">
    <!-- Back link -->
    <div class="back-row">
      <el-button link @click="router.push({ name: 'Monitoring' })">
        <el-icon><ArrowLeft /></el-icon> Back to Monitoring
      </el-button>
    </div>

    <template v-if="node">
      <el-card v-if="hasContext" class="context-card light-card" shadow="never">
        <div class="context-header">
          <div>
            <div class="header-title">Monitoring Context</div>
            <div class="context-description">{{ contextDescription }}</div>
          </div>
          <div class="context-actions">
            <el-button v-if="contextQuery.policyRef || contextQuery.policyDomain || contextQuery.commandId" size="small" @click="openPolicyCenter">
              Open Policy
            </el-button>
            <el-button
              v-if="hasPermission('ai:use')"
              size="small"
              type="success"
              plain
              @click="askAIForContext"
            >
              Ask AI
            </el-button>
            <el-button
              v-if="hasPermission('commands:write')"
              size="small"
              type="primary"
              :loading="contextCommandLoading === 'sync'"
              @click="runContextCommand('sync')"
            >
              Run Sync
            </el-button>
            <el-button
              v-if="hasPermission('commands:write')"
              size="small"
              :loading="contextCommandLoading === 'health_check'"
              @click="runContextCommand('health_check')"
            >
              Health Check
            </el-button>
            <el-button
              v-if="contextQuery.alertId && hasPermission('commands:write')"
              size="small"
              type="warning"
              plain
              :loading="resolvingContextAlert"
              @click="resolveContextAlert"
            >
              Resolve Alert
            </el-button>
            <el-button v-if="contextQuery.focus === 'certificate'" size="small" @click="scrollToFocusSection">
              Focus Certificate
            </el-button>
            <el-button v-if="contextQuery.focus === 'commands'" size="small" @click="scrollToFocusSection">
              Focus Commands
            </el-button>
            <el-button v-if="contextQuery.focus === 'policies'" size="small" @click="scrollToFocusSection">
              Focus Deliveries
            </el-button>
            <el-button v-if="contextQuery.focus === 'alerts'" size="small" @click="scrollToFocusSection">
              Focus Alerts
            </el-button>
          </div>
        </div>
      </el-card>

      <!-- Node Header -->
      <el-card class="header-card light-card" shadow="never">
        <div class="node-header">
          <div class="node-identity">
            <h2 class="node-hostname">{{ node.hostname || nodeId }}</h2>
            <el-tag
              :type="node.availability_status === 'online' ? 'success' : 'danger'"
              size="large"
            >
              {{ node.availability_status || 'unknown' }}
            </el-tag>
          </div>
          <div class="node-meta">
            <span v-if="node.last_sync_at" class="meta-item">
              Last sync: {{ formatTime(node.last_sync_at) }}
            </span>
          </div>
        </div>
      </el-card>

      <el-card class="state-card light-card" shadow="never">
        <template #header>
          <span class="header-title">Network Identity</span>
        </template>
        <el-row :gutter="20">
          <el-col :xs="24" :sm="6">
            <div class="state-block">
              <div class="state-label">Public IP</div>
              <div class="state-value state-wrap">{{ node.public_ip || '—' }}</div>
            </div>
          </el-col>
          <el-col :xs="24" :sm="6">
            <div class="state-block">
              <div class="state-label">VPN IP</div>
              <div class="state-value state-wrap">{{ node.assigned_ip || '—' }}</div>
            </div>
          </el-col>
          <el-col :xs="24" :sm="8">
            <div class="state-block">
              <div class="state-label">Endpoint</div>
              <div class="state-value state-wrap">{{ node.endpoint || '—' }}</div>
            </div>
          </el-col>
          <el-col :xs="24" :sm="4">
            <div class="state-block">
              <div class="state-label">Region</div>
              <div class="state-value state-wrap">{{ node.region || '—' }}</div>
            </div>
          </el-col>
        </el-row>
      </el-card>

      <el-card class="summary-card light-card" shadow="never">
        <template #header>
          <span class="header-title">Operations Summary</span>
        </template>
        <div class="summary-grid">
          <button
            v-for="item in workbenchSummary"
            :key="item.key"
            class="summary-item"
            type="button"
            @click="scrollToSection(item.focus)"
          >
            <span class="summary-label">{{ item.label }}</span>
            <span class="summary-value">{{ item.value }}</span>
            <el-tag :type="item.type" size="small">{{ item.status }}</el-tag>
          </button>
        </div>
      </el-card>

      <!-- Three-State Panel -->
      <el-card class="state-card light-card" shadow="never">
        <template #header>
          <div class="card-header">
            <span class="header-title">State Convergence</span>
            <el-tag
              :type="convergenceBadgeType"
              effect="dark"
              size="large"
            >
              {{ node.state_convergence || 'idle' }}
            </el-tag>
          </div>
        </template>

        <el-row :gutter="20">
          <el-col :xs="24" :sm="8">
            <div class="state-block">
              <div class="state-label">Desired State Version</div>
              <div class="state-value" :class="{ 'state-diverged': isDiverged }">
                {{ node.desired_state_version || '—' }}
              </div>
            </div>
          </el-col>
          <el-col :xs="24" :sm="8">
            <div class="state-block">
              <div class="state-label">Applied State Version</div>
              <div class="state-value" :class="{ 'state-diverged': isDiverged }">
                {{ node.applied_state_version || '—' }}
              </div>
            </div>
          </el-col>
          <el-col :xs="24" :sm="8">
            <div class="state-block">
              <div class="state-label">Observed State</div>
              <div class="state-value">
                {{ node.observed_state || '—' }}
              </div>
              <div v-if="node.observed_message" class="state-message">
                {{ node.observed_message }}
              </div>
            </div>
          </el-col>
        </el-row>

        <!-- Sync Error -->
        <div v-if="node.last_sync_error" class="sync-error">
          <el-alert
            :title="'Sync Error: ' + node.last_sync_error"
            type="error"
            show-icon
            :closable="false"
          />
        </div>
      </el-card>

      <el-card ref="certificateSectionRef" class="state-card light-card" shadow="never">
        <template #header>
          <div class="card-header">
            <span class="header-title">Certificate Status</span>
            <el-tag :type="certificateBadgeType" effect="dark" size="large">
              {{ certificateStatusLabel }}
            </el-tag>
          </div>
        </template>

        <el-row :gutter="20">
          <el-col :xs="24" :sm="8">
            <div class="state-block">
              <div class="state-label">Expires At</div>
              <div class="state-value">
                {{ node.certificate?.not_after ? formatTime(node.certificate.not_after) : '—' }}
              </div>
            </div>
          </el-col>
          <el-col :xs="24" :sm="8">
            <div class="state-block">
              <div class="state-label">Issued At</div>
              <div class="state-value">
                {{ node.certificate?.issued_at ? formatTime(node.certificate.issued_at) : '—' }}
              </div>
            </div>
          </el-col>
          <el-col :xs="24" :sm="8">
            <div class="state-block">
              <div class="state-label">Serial Number</div>
              <div class="state-value">
                {{ node.certificate?.serial_number || '—' }}
              </div>
            </div>
          </el-col>
        </el-row>

        <div v-if="node.certificate?.revoke_reason" class="state-message">
          Revoke reason: {{ node.certificate.revoke_reason }}
        </div>
        <div v-if="node.certificate?.renewed_from" class="state-message">
          Renewed from: {{ node.certificate.renewed_from }}
        </div>
        <el-row :gutter="20" class="certificate-activity-row">
          <el-col :xs="24" :sm="8">
            <div class="state-block">
              <div class="state-label">Last Renewed At</div>
              <div class="state-value">
                {{ certificateActivity?.last_renewed_at ? formatTime(certificateActivity.last_renewed_at) : '—' }}
              </div>
            </div>
          </el-col>
          <el-col :xs="24" :sm="8">
            <div class="state-block">
              <div class="state-label">Last Renew Failed At</div>
              <div class="state-value">
                {{ certificateActivity?.last_renew_failed_at ? formatTime(certificateActivity.last_renew_failed_at) : '—' }}
              </div>
            </div>
          </el-col>
          <el-col :xs="24" :sm="8">
            <div class="state-block">
              <div class="state-label">Last Renew Failure</div>
              <div class="state-value state-wrap">
                {{ certificateActivity?.last_renew_failure || '—' }}
              </div>
            </div>
          </el-col>
        </el-row>
        <div v-if="certificateActivity?.last_renewed_serial_number" class="state-message">
          Last renewed serial: {{ certificateActivity.last_renewed_serial_number }}
        </div>
      </el-card>

      <!-- Recent Commands -->
      <el-card ref="commandsSectionRef" class="table-card light-card" shadow="never">
        <template #header>
          <span class="header-title">Recent Commands</span>
        </template>
        <el-table
          :data="node.recent_commands || []"
          :row-class-name="commandRowClassName"
          stripe
          style="width: 100%"
          max-height="360"
          empty-text="No recent commands"
        >
          <el-table-column label="Type" width="140">
            <template #default="{ row }">
              {{ row.command || row.command_type || '—' }}
            </template>
          </el-table-column>
          <el-table-column prop="status" label="Status" width="120">
            <template #default="{ row }">
              <el-tag :type="commandStatusTagType(row.status)" size="small">{{ commandStatusLabel(row.status, t) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="created_at" label="Created" width="180">
            <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column prop="completed_at" label="Completed" width="180">
            <template #default="{ row }">{{ row.completed_at ? formatTime(row.completed_at) : '—' }}</template>
          </el-table-column>
          <el-table-column prop="error_message" label="Error" min-width="200">
            <template #default="{ row }">
              <span v-if="row.error_message || row.message" class="error-text">{{ row.error_message || row.message }}</span>
              <span v-else class="muted-text">—</span>
            </template>
          </el-table-column>
        </el-table>
      </el-card>

      <!-- Recent Policy Deliveries -->
      <el-card ref="policiesSectionRef" class="table-card light-card" shadow="never">
        <template #header>
          <span class="header-title">Recent Policy Deliveries</span>
        </template>
        <el-table
          :data="node.recent_policy_deliveries || []"
          :row-class-name="policyRowClassName"
          stripe
          style="width: 100%"
          max-height="360"
          empty-text="No recent policy deliveries"
        >
          <el-table-column prop="policy_domain" label="Domain" width="140" />
          <el-table-column prop="policy_ref" label="Policy Ref" width="160" />
          <el-table-column prop="command_status" label="Status" width="120">
            <template #default="{ row }">
              <el-tag :type="commandStatusTagType(row.command_status)" size="small">{{ commandStatusLabel(row.command_status, t) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="created_at" label="Created" width="180">
            <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column prop="last_error" label="Error" min-width="200">
            <template #default="{ row }">
              <span v-if="row.last_error" class="error-text">{{ row.last_error }}</span>
              <span v-else class="muted-text">—</span>
            </template>
          </el-table-column>
        </el-table>
      </el-card>

      <el-card ref="alertsSectionRef" class="table-card light-card" shadow="never">
        <template #header>
          <span class="header-title">Active Alerts</span>
        </template>
        <el-table
          :data="node.active_alerts || []"
          :row-class-name="alertRowClassName"
          stripe
          style="width: 100%"
          max-height="320"
          empty-text="No active alerts"
        >
          <el-table-column prop="severity" label="Severity" width="120">
            <template #default="{ row }">
              <el-tag :type="alertSeverityType(row.severity)" size="small">{{ row.severity || 'info' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="alert_type" label="Type" width="160" />
          <el-table-column prop="title" label="Title" min-width="180" />
          <el-table-column prop="message" label="Message" min-width="220" show-overflow-tooltip />
          <el-table-column prop="created_at" label="Created" width="180">
            <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
          </el-table-column>
        </el-table>
      </el-card>
    </template>

    <!-- Error state -->
    <el-empty v-if="!loading && !node" description="Node not found or failed to load" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, nextTick, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { useMonitorApi } from '@/composables/useMonitorApi'
import { useAgentProxyApi } from '@/composables/useAgentProxyApi'
import { usePermission } from '@/composables/usePermission'
import { t } from '@/i18n'
import {
  commandStatusLabel,
  commandStatusTagType,
  isPendingCommandStatus
} from '@/utils/controlLoopStatus'

const route = useRoute()
const router = useRouter()
const { hasPermission } = usePermission()

type AnyRecord = Record<string, any>
type SectionRef = { $el?: Element; scrollIntoView?: (options?: ScrollIntoViewOptions) => void } | Element

const loading = ref(false)
const contextCommandLoading = ref('')
const resolvingContextAlert = ref(false)
const node = ref<AnyRecord | null>(null)
const certificateSectionRef = ref<SectionRef | null>(null)
const commandsSectionRef = ref<SectionRef | null>(null)
const policiesSectionRef = ref<SectionRef | null>(null)
const alertsSectionRef = ref<SectionRef | null>(null)

const nodeId = computed(() => {
  const value = route.params.nodeId
  return Array.isArray(value) ? value[0] || '' : String(value || '')
})

const queryString = (...keys: string[]) => {
  for (const key of keys) {
    const value = route.query[key]
    if (typeof value === 'string') {
      return value
    }
  }
  return ''
}

const contextQuery = computed(() => ({
  focus: queryString('focus'),
  commandId: queryString('commandId', 'command_id'),
  policyRef: queryString('policyRef', 'policy_ref'),
  policyDomain: queryString('policyDomain', 'policy_domain', 'kind'),
  alertId: queryString('alertId', 'alert_id'),
  eventType: queryString('eventType', 'event_type')
}))
const hasContext = computed(() => Object.values(contextQuery.value).some(Boolean))
const contextDescription = computed(() => {
  const parts = []
  if (contextQuery.value.eventType) {
    parts.push(`Event: ${contextQuery.value.eventType}`)
  }
  if (contextQuery.value.policyRef) {
    parts.push(`Policy: ${contextQuery.value.policyRef}`)
  }
  if (contextQuery.value.commandId) {
    parts.push(`Command: ${contextQuery.value.commandId}`)
  }
  if (contextQuery.value.alertId) {
    parts.push(`Alert: ${contextQuery.value.alertId}`)
  }
  return parts.join(' | ')
})

const isDiverged = computed(() => {
  if (!node.value) return false
  const d = node.value.desired_state_version
  const a = node.value.applied_state_version
  return d && a && d !== a
})

const convergenceBadgeType = computed(() => {
  if (!node.value) return 'info'
  switch (node.value.state_convergence) {
    case 'converged': return 'success'
    case 'pending': return 'warning'
    case 'diverged': return 'danger'
    default: return 'info'
  }
})

const certificateStatusLabel = computed(() => node.value?.certificate?.status || 'missing')
const certificateActivity = computed(() => node.value?.certificate_activity || null)
const recentCommands = computed(() => node.value?.recent_commands || [])
const recentPolicyDeliveries = computed(() => node.value?.recent_policy_deliveries || [])
const activeAlerts = computed(() => node.value?.active_alerts || [])

const certificateBadgeType = computed(() => {
  switch (node.value?.certificate?.status) {
    case 'issued': return 'success'
    case 'expiring': return 'warning'
    case 'expired':
    case 'revoked':
      return 'danger'
    default:
      return 'info'
  }
})

const statusCount = (items: AnyRecord[], statuses: string[], field = 'status') => (
  items.filter((item) => statuses.includes(item?.[field])).length
)

const workbenchSummary = computed(() => {
  const failedCommands = statusCount(recentCommands.value, ['failed'])
  const pendingCommands = recentCommands.value.filter((item: AnyRecord) => isPendingCommandStatus(item?.status)).length
  const failedDeliveries = statusCount(recentPolicyDeliveries.value, ['failed'], 'command_status')
  const pendingDeliveries = recentPolicyDeliveries.value.filter((item: AnyRecord) => isPendingCommandStatus(item?.command_status)).length
  const certificateStatus = certificateStatusLabel.value

  return [
    {
      key: 'commands',
      label: 'Commands',
      value: recentCommands.value.length,
      status: failedCommands > 0 ? `${failedCommands} failed` : `${pendingCommands} pending`,
      type: failedCommands > 0 ? 'danger' : pendingCommands > 0 ? 'warning' : 'success',
      focus: 'commands'
    },
    {
      key: 'policies',
      label: 'Policy Deliveries',
      value: recentPolicyDeliveries.value.length,
      status: failedDeliveries > 0 ? `${failedDeliveries} failed` : `${pendingDeliveries} pending`,
      type: failedDeliveries > 0 ? 'danger' : pendingDeliveries > 0 ? 'warning' : 'success',
      focus: 'policies'
    },
    {
      key: 'alerts',
      label: 'Active Alerts',
      value: activeAlerts.value.length,
      status: activeAlerts.value.length > 0 ? 'active' : 'clear',
      type: activeAlerts.value.length > 0 ? 'danger' : 'success',
      focus: 'alerts'
    },
    {
      key: 'certificate',
      label: 'Certificate',
      value: certificateStatus,
      status: certificateActivity.value?.last_renew_failure ? 'renew failed' : certificateStatus,
      type: certificateBadgeType.value,
      focus: 'certificate'
    }
  ]
})

const loadNode = async () => {
  try {
    loading.value = true
    node.value = await useMonitorApi.getNodeDetail(nodeId.value)
  } catch (e) {
    console.error('Failed to load node detail:', e)
    node.value = null
  } finally {
    loading.value = false
  }
}

const buildContextCommandParams = () => ({
  source: 'node_monitor_detail',
  ...(contextQuery.value.alertId ? { alert_id: contextQuery.value.alertId } : {}),
  ...(contextQuery.value.eventType ? { event_type: contextQuery.value.eventType } : {}),
  ...(contextQuery.value.commandId ? { command_id: contextQuery.value.commandId } : {}),
  ...(contextQuery.value.policyRef ? { policy_ref: contextQuery.value.policyRef } : {}),
  ...(contextQuery.value.policyDomain ? { policy_domain: contextQuery.value.policyDomain } : {})
})

const normalizeQueuedCommand = (command: string, response: AnyRecord = {}) => ({
  id: response.command_id || response.id || '',
  command: response.command || command,
  status: response.status || 'pending',
  message: response.message || 'Command queued for delivery',
  created_at: response.created_at || new Date().toISOString(),
  updated_at: response.updated_at || response.created_at || new Date().toISOString(),
  timeout_seconds: response.timeout_seconds,
  priority: response.priority
})

const prependQueuedCommand = (command: AnyRecord) => {
  if (!node.value || !command?.id) return
  const existing = Array.isArray(node.value.recent_commands) ? node.value.recent_commands : []
  if (existing.some((item) => item.id === command.id)) {
    node.value.recent_commands = existing
    return
  }
  node.value.recent_commands = [command, ...existing]
}

const runContextCommand = async (command: string) => {
  if (!hasPermission('commands:write')) {
    ElMessage.error('Missing command permission')
    return
  }
  if (!nodeId.value) {
    ElMessage.error('Node context is missing')
    return
  }

  contextCommandLoading.value = command
  try {
    const response = await useAgentProxyApi.sendAgentCommand(nodeId.value, {
      command,
      params: buildContextCommandParams(),
      timeout: 30
    } as any)
    const queuedCommand = normalizeQueuedCommand(command, response)
    prependQueuedCommand(queuedCommand)
    ElMessage.success(`${command} queued`)
    await loadNode()
    prependQueuedCommand(queuedCommand)
  } catch (e) {
    console.error(`Failed to queue ${command}:`, e)
    ElMessage.error(`Failed to queue ${command}`)
  } finally {
    contextCommandLoading.value = ''
  }
}

const resolveContextAlert = async () => {
  if (!hasPermission('commands:write')) {
    ElMessage.error('Missing command permission')
    return
  }
  if (!contextQuery.value.alertId) {
    ElMessage.error('Alert context is missing')
    return
  }

  resolvingContextAlert.value = true
  try {
    await useMonitorApi.resolveAlert(contextQuery.value.alertId, {
      source: 'node_monitor_detail',
      reason: 'Resolved from node monitoring detail',
      ...(contextQuery.value.commandId ? { command_id: contextQuery.value.commandId } : {})
    })
    ElMessage.success('Alert resolved')
    await loadNode()
  } catch (e) {
    console.error('Failed to resolve context alert:', e)
    ElMessage.error('Failed to resolve alert')
  } finally {
    resolvingContextAlert.value = false
  }
}

const askAIForContext = () => {
  if (!hasPermission('ai:use')) {
    ElMessage.error('Missing AI permission')
    return
  }
  router.push({
    name: 'AiAssistant',
    query: {
      source: 'node_monitor_detail',
      nodeId: nodeId.value,
      ...(contextQuery.value.alertId ? { alertId: contextQuery.value.alertId } : {}),
      ...(contextQuery.value.eventType ? { eventType: contextQuery.value.eventType } : {}),
      ...(contextQuery.value.commandId ? { commandId: contextQuery.value.commandId } : {}),
      ...(contextQuery.value.policyRef ? { policyRef: contextQuery.value.policyRef } : {}),
      ...(contextQuery.value.policyDomain ? { policyDomain: contextQuery.value.policyDomain } : {})
    }
  })
}

const sectionRefForFocus = (focus: string): SectionRef | null => {
  if (focus === 'commands') return commandsSectionRef.value
  if (focus === 'certificate') return certificateSectionRef.value
  if (focus === 'policies') return policiesSectionRef.value
  if (focus === 'alerts') return alertsSectionRef.value
  return null
}

const scrollToSection = async (focus: string) => {
  await nextTick()
  const targetRef = sectionRefForFocus(focus)
  const target = (targetRef as any)?.$el || targetRef
  if (typeof target?.scrollIntoView === 'function') {
    target.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }
}

const scrollToFocusSection = () => scrollToSection(contextQuery.value.focus)

const formatTime = (iso?: string) => {
  if (!iso) return ''
  return new Date(iso).toLocaleString()
}

const cmdStatusType = commandStatusTagType

const alertSeverityType = (severity?: string) => {
  switch (severity) {
    case 'critical':
    case 'error':
      return 'danger'
    case 'warning':
      return 'warning'
    default:
      return 'info'
  }
}

const commandRowClassName = ({ row }: { row: AnyRecord }) => {
  if (!contextQuery.value.commandId) return ''
  return String(row.id) === contextQuery.value.commandId ? 'context-match-row' : ''
}

const policyRowClassName = ({ row }: { row: AnyRecord }) => {
  if (contextQuery.value.commandId && row.command_id === contextQuery.value.commandId) {
    return 'context-match-row'
  }
  if (contextQuery.value.policyRef && row.policy_ref === contextQuery.value.policyRef) {
    return 'context-match-row'
  }
  return ''
}

const alertRowClassName = ({ row }: { row: AnyRecord }) => {
  if (!contextQuery.value.alertId) return ''
  return row.id === contextQuery.value.alertId ? 'context-match-row' : ''
}

const openPolicyCenter = () => {
  router.push({
    name: 'Policies',
    query: {
      nodeId: nodeId.value,
      ...(contextQuery.value.policyRef ? { policyRef: contextQuery.value.policyRef } : {}),
      ...(contextQuery.value.policyDomain ? { kind: contextQuery.value.policyDomain } : {}),
      ...(contextQuery.value.commandId ? { commandId: contextQuery.value.commandId } : {})
    }
  })
}

onMounted(() => {
  loadNode()
})

watch(() => route.fullPath, async () => {
  await loadNode()
  await scrollToFocusSection()
})

watch(node, async (value) => {
  if (value) {
    await scrollToFocusSection()
  }
})
</script>

<style scoped>
.node-monitor-detail {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.context-card .context-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  flex-wrap: wrap;
}

.context-description {
  margin-top: 6px;
  color: var(--aria-text-muted, #94A3B8);
  font-size: 13px;
}

.context-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.back-row {
  margin-bottom: 4px;
}

/* Header Card */
.header-card .node-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
}

.node-identity {
  display: flex;
  align-items: center;
  gap: 12px;
}

.node-hostname {
  font-size: 22px;
  font-weight: 700;
  color: var(--aria-text-primary, #1E293B);
  margin: 0;
}

.node-meta {
  display: flex;
  gap: 16px;
}

.meta-item {
  font-size: 13px;
  color: var(--aria-text-muted, #94A3B8);
}

.summary-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.summary-item {
  min-height: 96px;
  padding: 14px;
  border: 1px solid var(--aria-border-color, #E2E8F0);
  border-radius: 8px;
  background: var(--aria-content-bg-tertiary, #F8FAFC);
  color: inherit;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: space-between;
  text-align: left;
}

.summary-item:hover {
  border-color: var(--el-color-primary);
}

.summary-label {
  font-size: 12px;
  color: var(--aria-text-muted, #94A3B8);
  text-transform: uppercase;
  letter-spacing: 0;
}

.summary-value {
  font-size: 22px;
  font-weight: 700;
  color: var(--aria-text-primary, #1E293B);
  word-break: break-word;
}

:deep(.context-match-row > td) {
  background: rgba(59, 130, 246, 0.10) !important;
}

/* State Card */
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--aria-text-primary, #1E293B);
}

.state-block {
  padding: 16px;
  background: var(--aria-content-bg-tertiary, #F8FAFC);
  border-radius: 8px;
  text-align: center;
  margin-bottom: 12px;
}

.state-label {
  font-size: 12px;
  color: var(--aria-text-muted, #94A3B8);
  margin-bottom: 6px;
  text-transform: uppercase;
  letter-spacing: 0;
}

.state-value {
  font-size: 15px;
  font-weight: 600;
  color: var(--aria-text-primary, #1E293B);
  word-break: break-all;
}

.state-value.state-diverged {
  color: #EF4444;
}

.state-message {
  font-size: 12px;
  color: var(--aria-text-muted, #94A3B8);
  margin-top: 4px;
}

.certificate-activity-row {
  margin-top: 8px;
}

.state-wrap {
  white-space: normal;
  word-break: break-word;
}

.sync-error {
  margin-top: 16px;
}

/* Tables */
.table-card {
  overflow: hidden;
}

.error-text {
  color: #EF4444;
  font-size: 13px;
}

.muted-text {
  color: var(--aria-text-muted, #94A3B8);
}

@media (max-width: 768px) {
  .summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .node-hostname {
    font-size: 18px;
  }
}

@media (max-width: 520px) {
  .summary-grid {
    grid-template-columns: 1fr;
  }
}
</style>
