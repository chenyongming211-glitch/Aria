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
            <el-button v-if="contextQuery.policyRef" size="small" @click="openPolicyCenter">
              Open Policy
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

      <!-- Three-State Panel -->
      <el-card ref="certificateSectionRef" class="state-card light-card" shadow="never">
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

      <el-card class="state-card light-card" shadow="never">
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
              <el-tag :type="cmdStatusType(row.status)" size="small">{{ row.status }}</el-tag>
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
              <el-tag :type="cmdStatusType(row.command_status)" size="small">{{ row.command_status }}</el-tag>
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

<script setup>
import { ref, computed, onMounted, nextTick, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft } from '@element-plus/icons-vue'
import { useMonitorApi } from '@/composables/useMonitorApi'

const route = useRoute()
const router = useRouter()

const loading = ref(false)
const node = ref(null)
const certificateSectionRef = ref(null)
const commandsSectionRef = ref(null)
const policiesSectionRef = ref(null)
const alertsSectionRef = ref(null)

const nodeId = computed(() => route.params.nodeId)
const contextQuery = computed(() => ({
  focus: typeof route.query.focus === 'string' ? route.query.focus : '',
  commandId: typeof route.query.commandId === 'string' ? route.query.commandId : '',
  policyRef: typeof route.query.policyRef === 'string' ? route.query.policyRef : '',
  policyDomain: typeof route.query.policyDomain === 'string' ? route.query.policyDomain : '',
  alertId: typeof route.query.alertId === 'string' ? route.query.alertId : '',
  eventType: typeof route.query.eventType === 'string' ? route.query.eventType : ''
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

const scrollToFocusSection = async () => {
  await nextTick()
  const focus = contextQuery.value.focus
  const targetRef = focus === 'commands'
    ? commandsSectionRef.value
    : focus === 'certificate'
      ? certificateSectionRef.value
    : focus === 'policies'
      ? policiesSectionRef.value
      : focus === 'alerts'
        ? alertsSectionRef.value
        : null
  const target = targetRef?.$el || targetRef
  if (typeof target?.scrollIntoView === 'function') {
    target.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }
}

const formatTime = (iso) => {
  if (!iso) return ''
  return new Date(iso).toLocaleString()
}

const cmdStatusType = (status) => {
  switch (status) {
    case 'completed': return 'success'
    case 'failed': return 'danger'
    case 'pending': case 'queued': return 'warning'
    default: return 'info'
  }
}

const alertSeverityType = (severity) => {
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

const commandRowClassName = ({ row }) => {
  if (!contextQuery.value.commandId) return ''
  return String(row.id) === contextQuery.value.commandId ? 'context-match-row' : ''
}

const policyRowClassName = ({ row }) => {
  if (contextQuery.value.commandId && row.command_id === contextQuery.value.commandId) {
    return 'context-match-row'
  }
  if (contextQuery.value.policyRef && row.policy_ref === contextQuery.value.policyRef) {
    return 'context-match-row'
  }
  return ''
}

const alertRowClassName = ({ row }) => {
  if (!contextQuery.value.alertId) return ''
  return row.id === contextQuery.value.alertId ? 'context-match-row' : ''
}

const openPolicyCenter = () => {
  router.push({
    name: 'Policies',
    query: {
      nodeId: nodeId.value,
      ...(contextQuery.value.policyRef ? { policyRef: contextQuery.value.policyRef } : {}),
      ...(contextQuery.value.policyDomain ? { kind: contextQuery.value.policyDomain } : {})
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
  letter-spacing: 0.5px;
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
  .node-hostname {
    font-size: 18px;
  }
}
</style>
