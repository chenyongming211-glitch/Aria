<!-- src/views/NodeMonitorDetail.vue -->
<template>
  <div class="node-monitor-detail" v-loading="loading">
    <!-- Back link -->
    <div class="back-row">
      <el-button link @click="router.push({ name: 'Monitoring' })">
        <el-icon><ArrowLeft /></el-icon> {{ t('nodeMonitorDetail.backToMonitoring') }}
      </el-button>
    </div>

    <template v-if="node">
      <el-card v-if="hasContext" class="context-card light-card" shadow="never">
        <div class="context-header">
          <div>
            <div class="header-title">{{ t('nodeMonitorDetail.monitoringContext') }}</div>
            <div class="context-description">{{ contextDescription }}</div>
          </div>
          <div class="context-actions">
            <el-button v-if="contextQuery.policyRef || contextQuery.policyDomain || contextQuery.commandId" size="small" @click="openPolicyCenter">
              {{ t('nodeMonitorDetail.openPolicy') }}
            </el-button>
            <el-button
              v-if="hasPermission('ai:use')"
              size="small"
              type="success"
              plain
              @click="askAIForContext"
            >
              {{ t('monitoringPage.askAI') }}
            </el-button>
            <el-button
              v-if="hasPermission('commands:write')"
              size="small"
              type="primary"
              :loading="contextCommandLoading === 'sync'"
              @click="runContextCommand('sync')"
            >
              {{ t('monitoringPage.runSync') }}
            </el-button>
            <el-button
              v-if="hasPermission('commands:write')"
              size="small"
              :loading="contextCommandLoading === 'health_check'"
              @click="runContextCommand('health_check')"
            >
              {{ t('monitoringPage.healthCheck') }}
            </el-button>
            <el-button
              v-if="contextQuery.alertId && hasPermission('commands:write')"
              size="small"
              type="warning"
              plain
              :loading="resolvingContextAlert"
              @click="resolveContextAlert"
            >
              {{ t('nodeMonitorDetail.resolveAlert') }}
            </el-button>
            <el-button v-if="contextQuery.focus === 'certificate'" size="small" @click="scrollToFocusSection">
              {{ t('nodeMonitorDetail.focusCertificate') }}
            </el-button>
            <el-button v-if="contextQuery.focus === 'commands'" size="small" @click="scrollToFocusSection">
              {{ t('nodeMonitorDetail.focusCommands') }}
            </el-button>
            <el-button v-if="contextQuery.focus === 'policies'" size="small" @click="scrollToFocusSection">
              {{ t('nodeMonitorDetail.focusDeliveries') }}
            </el-button>
            <el-button v-if="contextQuery.focus === 'alerts'" size="small" @click="scrollToFocusSection">
              {{ t('nodeMonitorDetail.focusAlerts') }}
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
              {{ node.availability_status || t('status.unknown') }}
            </el-tag>
          </div>
          <div class="node-meta">
            <span v-if="node.last_sync_at" class="meta-item">
              {{ t('nodeMonitorDetail.lastSync') }}: {{ formatTime(node.last_sync_at) }}
            </span>
          </div>
        </div>
      </el-card>

      <el-card class="state-card light-card" shadow="never">
        <template #header>
          <span class="header-title">{{ t('nodeMonitorDetail.networkIdentity') }}</span>
        </template>
        <el-row :gutter="20">
          <el-col :xs="24" :sm="6">
            <div class="state-block">
              <div class="state-label">{{ t('nodeMonitorDetail.publicIp') }}</div>
              <div class="state-value state-wrap">{{ node.public_ip || '—' }}</div>
            </div>
          </el-col>
          <el-col :xs="24" :sm="6">
            <div class="state-block">
              <div class="state-label">{{ t('nodeMonitorDetail.vpnIp') }}</div>
              <div class="state-value state-wrap">{{ node.assigned_ip || '—' }}</div>
            </div>
          </el-col>
          <el-col :xs="24" :sm="8">
            <div class="state-block">
              <div class="state-label">{{ t('nodeMonitorDetail.endpoint') }}</div>
              <div class="state-value state-wrap">{{ node.endpoint || '—' }}</div>
            </div>
          </el-col>
          <el-col :xs="24" :sm="4">
            <div class="state-block">
              <div class="state-label">{{ t('nodeMonitorDetail.region') }}</div>
              <div class="state-value state-wrap">{{ node.region || '—' }}</div>
            </div>
          </el-col>
        </el-row>
      </el-card>

      <el-card class="summary-card light-card" shadow="never">
        <template #header>
          <span class="header-title">{{ t('nodeMonitorDetail.operationsSummary') }}</span>
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
            <span class="header-title">{{ t('nodeMonitorDetail.stateConvergence') }}</span>
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
              <div class="state-label">{{ t('nodeMonitorDetail.desiredStateVersion') }}</div>
              <div class="state-value" :class="{ 'state-diverged': isDiverged }">
                {{ node.desired_state_version || '—' }}
              </div>
            </div>
          </el-col>
          <el-col :xs="24" :sm="8">
            <div class="state-block">
              <div class="state-label">{{ t('nodeMonitorDetail.appliedStateVersion') }}</div>
              <div class="state-value" :class="{ 'state-diverged': isDiverged }">
                {{ node.applied_state_version || '—' }}
              </div>
            </div>
          </el-col>
          <el-col :xs="24" :sm="8">
            <div class="state-block">
              <div class="state-label">{{ t('nodeMonitorDetail.observedState') }}</div>
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
            :title="`${t('nodeMonitorDetail.syncError')}: ${node.last_sync_error}`"
            type="error"
            show-icon
            :closable="false"
          />
        </div>
      </el-card>

      <el-card ref="certificateSectionRef" class="state-card light-card" shadow="never">
        <template #header>
          <div class="card-header">
            <span class="header-title">{{ t('nodeMonitorDetail.certificateStatus') }}</span>
            <el-tag :type="certificateBadgeType" effect="dark" size="large">
              {{ certificateStatusLabel }}
            </el-tag>
          </div>
        </template>

        <el-row :gutter="20">
          <el-col :xs="24" :sm="8">
            <div class="state-block">
              <div class="state-label">{{ t('nodeMonitorDetail.expiresAt') }}</div>
              <div class="state-value">
                {{ node.certificate?.not_after ? formatTime(node.certificate.not_after) : '—' }}
              </div>
            </div>
          </el-col>
          <el-col :xs="24" :sm="8">
            <div class="state-block">
              <div class="state-label">{{ t('nodeMonitorDetail.issuedAt') }}</div>
              <div class="state-value">
                {{ node.certificate?.issued_at ? formatTime(node.certificate.issued_at) : '—' }}
              </div>
            </div>
          </el-col>
          <el-col :xs="24" :sm="8">
            <div class="state-block">
              <div class="state-label">{{ t('nodeMonitorDetail.serialNumber') }}</div>
              <div class="state-value">
                {{ node.certificate?.serial_number || '—' }}
              </div>
            </div>
          </el-col>
        </el-row>

        <div v-if="node.certificate?.revoke_reason" class="state-message">
          {{ t('nodeMonitorDetail.revokeReason') }}: {{ node.certificate.revoke_reason }}
        </div>
        <div v-if="node.certificate?.renewed_from" class="state-message">
          {{ t('nodeMonitorDetail.renewedFrom') }}: {{ node.certificate.renewed_from }}
        </div>
        <el-row :gutter="20" class="certificate-activity-row">
          <el-col :xs="24" :sm="8">
            <div class="state-block">
              <div class="state-label">{{ t('nodeMonitorDetail.lastRenewedAt') }}</div>
              <div class="state-value">
                {{ certificateActivity?.last_renewed_at ? formatTime(certificateActivity.last_renewed_at) : '—' }}
              </div>
            </div>
          </el-col>
          <el-col :xs="24" :sm="8">
            <div class="state-block">
              <div class="state-label">{{ t('nodeMonitorDetail.lastRenewFailedAt') }}</div>
              <div class="state-value">
                {{ certificateActivity?.last_renew_failed_at ? formatTime(certificateActivity.last_renew_failed_at) : '—' }}
              </div>
            </div>
          </el-col>
          <el-col :xs="24" :sm="8">
            <div class="state-block">
              <div class="state-label">{{ t('nodeMonitorDetail.lastRenewFailure') }}</div>
              <div class="state-value state-wrap">
                {{ certificateActivity?.last_renew_failure || '—' }}
              </div>
            </div>
          </el-col>
        </el-row>
        <el-row :gutter="20" class="certificate-activity-row">
          <el-col :xs="24" :sm="8">
            <div class="state-block">
              <div class="state-label">{{ t('nodeMonitorDetail.lastRevokedAt') }}</div>
              <div class="state-value">
                {{ certificateActivity?.last_revoked_at ? formatTime(certificateActivity.last_revoked_at) : '—' }}
              </div>
            </div>
          </el-col>
          <el-col :xs="24" :sm="8">
            <div class="state-block">
              <div class="state-label">{{ t('nodeMonitorDetail.revokeStatus') }}</div>
              <div class="state-value">
                {{ certificateActivity?.last_revoke_node_status || '—' }}
              </div>
            </div>
          </el-col>
          <el-col :xs="24" :sm="8">
            <div class="state-block">
              <div class="state-label">{{ t('nodeMonitorDetail.revokeReason') }}</div>
              <div class="state-value state-wrap">
                {{ certificateActivity?.last_revoke_reason || node.certificate?.revoke_reason || '—' }}
              </div>
            </div>
          </el-col>
        </el-row>
        <div v-if="certificateActivity?.last_renewed_serial_number" class="state-message">
          {{ t('nodeMonitorDetail.lastRenewedSerial') }}: {{ certificateActivity.last_renewed_serial_number }}
        </div>
      </el-card>

      <!-- Recent Commands -->
      <el-card ref="commandsSectionRef" class="table-card light-card" shadow="never">
        <template #header>
          <span class="header-title">{{ t('nodeMonitorDetail.recentCommands') }}</span>
        </template>
        <el-table
          :data="node.recent_commands || []"
          :row-class-name="commandRowClassName"
          stripe
          style="width: 100%"
          max-height="360"
          :empty-text="t('nodeMonitorDetail.noRecentCommands')"
        >
          <el-table-column :label="t('nodeMonitorDetail.type')" width="140">
            <template #default="{ row }">
              {{ row.command || row.command_type || '—' }}
            </template>
          </el-table-column>
          <el-table-column prop="status" :label="t('nodeMonitorDetail.status')" width="120">
            <template #default="{ row }">
              <el-tag :type="commandStatusTagType(row.status)" size="small">{{ commandStatusLabel(row.status, t) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="created_at" :label="t('nodeMonitorDetail.created')" width="180">
            <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column prop="completed_at" :label="t('nodeMonitorDetail.completed')" width="180">
            <template #default="{ row }">{{ row.completed_at ? formatTime(row.completed_at) : '—' }}</template>
          </el-table-column>
          <el-table-column prop="error_message" :label="t('nodeMonitorDetail.error')" min-width="200">
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
          <span class="header-title">{{ t('nodeMonitorDetail.recentPolicyDeliveries') }}</span>
        </template>
        <el-table
          :data="node.recent_policy_deliveries || []"
          :row-class-name="policyRowClassName"
          stripe
          style="width: 100%"
          max-height="360"
          :empty-text="t('nodeMonitorDetail.noRecentPolicyDeliveries')"
        >
          <el-table-column prop="policy_domain" :label="t('nodeMonitorDetail.domain')" width="140" />
          <el-table-column prop="policy_ref" :label="t('nodeMonitorDetail.policyRef')" width="160" />
          <el-table-column prop="command_status" :label="t('nodeMonitorDetail.status')" width="120">
            <template #default="{ row }">
              <el-tag :type="commandStatusTagType(row.command_status)" size="small">{{ commandStatusLabel(row.command_status, t) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="created_at" :label="t('nodeMonitorDetail.created')" width="180">
            <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column prop="last_error" :label="t('nodeMonitorDetail.error')" min-width="200">
            <template #default="{ row }">
              <span v-if="row.last_error" class="error-text">{{ row.last_error }}</span>
              <span v-else class="muted-text">—</span>
            </template>
          </el-table-column>
        </el-table>
      </el-card>

      <el-card ref="alertsSectionRef" class="table-card light-card" shadow="never">
        <template #header>
          <span class="header-title">{{ t('nodeMonitorDetail.activeAlerts') }}</span>
        </template>
        <el-table
          :data="node.active_alerts || []"
          :row-class-name="alertRowClassName"
          stripe
          style="width: 100%"
          max-height="320"
          :empty-text="t('nodeMonitorDetail.noActiveAlerts')"
        >
          <el-table-column prop="severity" :label="t('nodeMonitorDetail.severity')" width="120">
            <template #default="{ row }">
              <el-tag :type="alertSeverityType(row.severity)" size="small">{{ formatSeverity(row.severity) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="alert_type" :label="t('nodeMonitorDetail.type')" width="160">
            <template #default="{ row }">{{ formatEventType(row.alert_type) }}</template>
          </el-table-column>
          <el-table-column prop="title" :label="t('nodeMonitorDetail.titleColumn')" min-width="180">
            <template #default="{ row }">{{ formatMonitoringTitle(row) }}</template>
          </el-table-column>
          <el-table-column prop="message" :label="t('nodeMonitorDetail.message')" min-width="220" show-overflow-tooltip />
          <el-table-column prop="created_at" :label="t('nodeMonitorDetail.created')" width="180">
            <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
          </el-table-column>
        </el-table>
      </el-card>
    </template>

    <!-- Error state -->
    <el-empty v-if="!loading && !node" :description="t('nodeMonitorDetail.nodeLoadFailed')" />
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
import { useFocusedPolling } from '@/composables/useFocusedPolling'
import { isActiveNodeStatus } from '@/composables/usePolicyStatusApi'
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
    parts.push(`${t('nodeMonitorDetail.event')}: ${formatEventType(contextQuery.value.eventType)}`)
  }
  if (contextQuery.value.policyRef) {
    parts.push(`${t('nodeMonitorDetail.policy')}: ${contextQuery.value.policyRef}`)
  }
  if (contextQuery.value.commandId) {
    parts.push(`${t('nodeMonitorDetail.command')}: ${contextQuery.value.commandId}`)
  }
  if (contextQuery.value.alertId) {
    parts.push(`${t('nodeMonitorDetail.alert')}: ${contextQuery.value.alertId}`)
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
const hasActiveDetailStatus = computed(() => {
  if (!node.value) return false
  return isActiveNodeStatus(node.value) ||
    recentCommands.value.some((item: AnyRecord) => isPendingCommandStatus(item?.status)) ||
    recentPolicyDeliveries.value.some((item: AnyRecord) => isPendingCommandStatus(item?.command_status))
})

const formatEventType = (type?: string) => {
  if (!type) return ''
  const labels: Record<string, string> = {
    node_offline: t('monitoringPage.nodeOffline'),
    node_online: t('monitoringPage.nodeOnline'),
    certificate_expiring: t('monitoringPage.certificateExpiring'),
    certificate_expired: t('monitoringPage.certificateExpired'),
    certificate_renew_failed: t('monitoringPage.certificateRenewFailed'),
    certificate_renewed: t('monitoringPage.certificateRenewed'),
    sync_failed: t('monitoringPage.syncFailed'),
    policy_failed: t('monitoringPage.policyFailed'),
    command_completed: t('monitoringPage.commandCompleted'),
    command_failed: t('monitoringPage.commandFailed'),
    policy_delivered: t('monitoringPage.policyDelivered'),
    alert_created: t('monitoringPage.alertCreated'),
    alert_resolved: t('monitoringPage.alertResolved')
  }
  return labels[type] || type.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase())
}

const formatSeverity = (severity?: string) => {
  if (!severity) return t('common.info')
  const labels: Record<string, string> = {
    critical: t('common.critical'),
    warning: t('common.warning'),
    info: t('common.info')
  }
  return labels[severity] || severity
}

const formatMonitoringTitle = (record: AnyRecord = {}) => {
  const type = record.alert_type || record.event_type
  return type ? formatEventType(type) : (record.title || '')
}

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
      label: t('nodeMonitorDetail.commands'),
      value: recentCommands.value.length,
      status: failedCommands > 0
        ? t('nodeMonitorDetail.failedCount').replace('{count}', String(failedCommands))
        : t('nodeMonitorDetail.pendingCount').replace('{count}', String(pendingCommands)),
      type: failedCommands > 0 ? 'danger' : pendingCommands > 0 ? 'warning' : 'success',
      focus: 'commands'
    },
    {
      key: 'policies',
      label: t('nodeMonitorDetail.policies'),
      value: recentPolicyDeliveries.value.length,
      status: failedDeliveries > 0
        ? t('nodeMonitorDetail.failedCount').replace('{count}', String(failedDeliveries))
        : t('nodeMonitorDetail.pendingCount').replace('{count}', String(pendingDeliveries)),
      type: failedDeliveries > 0 ? 'danger' : pendingDeliveries > 0 ? 'warning' : 'success',
      focus: 'policies'
    },
    {
      key: 'alerts',
      label: t('nodeMonitorDetail.activeAlerts'),
      value: activeAlerts.value.length,
      status: activeAlerts.value.length > 0 ? t('nodeMonitorDetail.active') : t('nodeMonitorDetail.clear'),
      type: activeAlerts.value.length > 0 ? 'danger' : 'success',
      focus: 'alerts'
    },
    {
      key: 'certificate',
      label: t('nodeMonitorDetail.certificate'),
      value: certificateStatus,
      status: certificateActivity.value?.last_revoke_reason
        ? t('nodeMonitorDetail.revoked')
        : certificateActivity.value?.last_renew_failure
          ? t('nodeMonitorDetail.renewFailed')
          : certificateStatus,
      type: certificateBadgeType.value,
      focus: 'certificate'
    }
  ]
})

const loadNode = async (options: { silent?: boolean } = {}) => {
  try {
    if (!options.silent) {
      loading.value = true
    }
    node.value = await useMonitorApi.getNodeDetail(nodeId.value)
  } catch (e) {
    console.error('Failed to load node detail:', e)
    if (!options.silent) {
      node.value = null
    }
  } finally {
    if (!options.silent) {
      loading.value = false
    }
  }
}

const detailStatusPolling = useFocusedPolling({
  poll: () => loadNode({ silent: true }),
  hasActiveItems: () => hasActiveDetailStatus.value,
  intervalMs: 3000
})

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
  message: response.message || t('nodeMonitorDetail.commandQueuedForDelivery'),
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
    ElMessage.error(t('nodeMonitorDetail.missingCommandPermission'))
    return
  }
  if (!nodeId.value) {
    ElMessage.error(t('nodeMonitorDetail.missingNodeContext'))
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
    ElMessage.success(t('nodeMonitorDetail.commandQueued').replace('{command}', command))
    await loadNode()
    prependQueuedCommand(queuedCommand)
  } catch (e) {
    console.error(`Failed to queue ${command}:`, e)
    ElMessage.error(t('nodeMonitorDetail.commandQueueFailed').replace('{command}', command))
  } finally {
    contextCommandLoading.value = ''
  }
}

const resolveContextAlert = async () => {
  if (!hasPermission('commands:write')) {
    ElMessage.error(t('nodeMonitorDetail.missingCommandPermission'))
    return
  }
  if (!contextQuery.value.alertId) {
    ElMessage.error(t('nodeMonitorDetail.missingAlertContext'))
    return
  }

  resolvingContextAlert.value = true
  try {
    await useMonitorApi.resolveAlert(contextQuery.value.alertId, {
      source: 'node_monitor_detail',
      reason: 'Resolved from node monitoring detail',
      ...(contextQuery.value.commandId ? { command_id: contextQuery.value.commandId } : {})
    })
    ElMessage.success(t('nodeMonitorDetail.alertResolved'))
    await loadNode()
  } catch (e) {
    console.error('Failed to resolve context alert:', e)
    ElMessage.error(t('nodeMonitorDetail.alertResolveFailed'))
  } finally {
    resolvingContextAlert.value = false
  }
}

const askAIForContext = () => {
  if (!hasPermission('ai:use')) {
    ElMessage.error(t('nodeMonitorDetail.missingAiPermission'))
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

onMounted(async () => {
  await loadNode()
  await detailStatusPolling.trigger()
})

watch(() => route.fullPath, async () => {
  await loadNode()
  await detailStatusPolling.trigger()
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
