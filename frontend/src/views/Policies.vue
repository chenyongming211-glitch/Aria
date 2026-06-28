<template>
  <div class="policy-center page-shell">
    <PageHeader
      :title="t('policies.title')"
      :subtitle="t('policies.subtitle')"
    >
      <template #actions>
        <el-button v-if="hasPermission('ip-groups:read')" @click="goToIpGroups">
          <el-icon><Collection /></el-icon>
          {{ t('nav.ipGroupManagement') }}
        </el-button>
        <el-button v-if="hasPermission('acls:read')" @click="goToKind('acl')">
          <el-icon><Lock /></el-icon>
          {{ t('policies.aclRules') }}
        </el-button>
        <el-button v-if="hasPermission('qos:read')" @click="goToKind('qos')">
          <el-icon><Histogram /></el-icon>
          {{ t('policies.bandwidthControl') }}
        </el-button>
        <el-button v-if="hasPermission('routes:read')" @click="goToKind('route')">
          <el-icon><Connection /></el-icon>
          {{ t('policies.routingManagement') }}
        </el-button>
        <el-button type="primary" @click="refreshData">
          <el-icon><Refresh /></el-icon>
          {{ t('common.refresh') }}
        </el-button>
      </template>
    </PageHeader>

    <MetricStrip :metrics="policyMetricItems" @select="handleMetricSelect" />

    <FilterBar>
      <template #filters>
        <el-input
          v-model="filters.keyword"
          :placeholder="t('policies.keywordPlaceholder')"
          clearable
          class="keyword-filter"
        />
        <el-select v-model="filters.kind" clearable :placeholder="t('policies.policyType')" class="filter-item">
          <el-option label="ACL" value="acl" />
          <el-option label="QoS" value="qos" />
          <el-option :label="t('policies.route')" value="route" />
        </el-select>
        <el-select v-model="filters.status" clearable :placeholder="t('policyTerms.deliveryStatus')" class="filter-item" @change="filters.statusGroup = ''">
          <el-option :label="t('policies.applied')" value="applied" />
          <el-option :label="t('policies.pending')" value="pending" />
          <el-option :label="t('policies.inProgress')" value="in_progress" />
          <el-option :label="t('policies.stale')" value="stale" />
          <el-option :label="t('policies.failed')" value="error" />
          <el-option :label="t('policies.idle')" value="idle" />
        </el-select>
        <el-select v-model="filters.nodeId" clearable :placeholder="t('policies.targetNode')" class="filter-item">
          <el-option
            v-for="node in nodeOptions"
            :key="node.id"
            :label="node.name"
            :value="node.id"
          />
        </el-select>
      </template>
    </FilterBar>

    <DataPanel
      class="policy-inventory-panel"
      :title="t('policies.panelTitle')"
      :subtitle="t('policies.panelSubtitle')"
      v-loading="loading"
    >
      <div class="policy-panel-body">
        <el-alert
          :title="t('policies.infoTitle')"
          type="info"
          :closable="false"
          :description="t('policies.infoDescription')"
          style="margin-bottom: 20px;"
        />

        <el-alert
          v-if="hasRouteContext"
          :title="t('policies.monitoringContext')"
          type="warning"
          :closable="false"
          style="margin-bottom: 20px;"
        >
          <div class="context-alert-body">
            <span>{{ routeContextSummary }}</span>
            <div class="context-alert-actions">
              <el-button v-if="selectedPolicy" size="small" @click="openNodeDetail(selectedPolicy)">
                {{ t('policies.openNodeDetail') }}
              </el-button>
              <el-button v-if="selectedPolicy" size="small" type="primary" @click="showDetails(selectedPolicy)">
                {{ t('policies.focusDeliveryHistory') }}
              </el-button>
            </div>
          </div>
        </el-alert>

        <el-table
          :data="filteredPolicies"
          :row-class-name="policyRowClassName"
          stripe
          style="width: 100%"
        >
          <el-table-column prop="kind" :label="t('common.type')" width="110">
            <template #default="{ row }">
              <el-tag :type="kindTagType(row.kind)">
                {{ kindLabel(row.kind) }}
              </el-tag>
            </template>
          </el-table-column>

          <el-table-column prop="name" :label="t('common.name')" min-width="200" show-overflow-tooltip />

          <el-table-column prop="nodeName" :label="t('policies.targetNode')" width="180" show-overflow-tooltip />

          <el-table-column :label="t('policies.summary')" min-width="320" show-overflow-tooltip>
            <template #default="{ row }">
              {{ summarizePolicy(row) }}
            </template>
          </el-table-column>

          <el-table-column :label="t('policyTerms.enabled')" width="90">
            <template #default="{ row }">
              <el-tag size="small" :type="row.enabled ? 'success' : 'info'">
                {{ row.enabled ? t('policyTerms.enabled') : t('policyTerms.disabled') }}
              </el-tag>
            </template>
          </el-table-column>

          <el-table-column prop="priority" :label="t('policyTerms.priority')" width="90" />

          <el-table-column :label="t('common.status')" width="110">
            <template #default="{ row }">
              <el-tag size="small" :type="statusTagType(row.status)">
                {{ statusLabel(row.status, t) }}
              </el-tag>
            </template>
          </el-table-column>

          <el-table-column :label="t('policies.convergence')" width="120">
            <template #default="{ row }">
              <el-tag size="small" :type="convergenceTagType(row.stateConvergence)">
                {{ convergenceLabel(row.stateConvergence) }}
              </el-tag>
            </template>
          </el-table-column>

          <el-table-column prop="pendingCmds" :label="t('policyTerms.pendingCommands')" width="90" />

          <el-table-column :label="t('policyTerms.lastCommand')" width="150">
            <template #default="{ row }">
              <el-tooltip
                v-if="row.lastDeliveryCommandId"
                :content="row.lastDeliveryCommandId"
                placement="top"
              >
                <span>{{ shortCommandId(row.lastDeliveryCommandId) }}</span>
              </el-tooltip>
              <span v-else>-</span>
            </template>
          </el-table-column>

          <el-table-column prop="lastDeliveryError" :label="t('policyTerms.failureReason')" min-width="180" show-overflow-tooltip />

          <el-table-column :label="t('common.actions')" width="260">
            <template #default="{ row }">
              <el-button
                v-if="canRetryPolicy(row)"
                size="small"
                type="warning"
                @click="retryPolicyDelivery(row)"
              >
                {{ t('policyTerms.retry') }}
              </el-button>
              <el-button size="small" @click="showDetails(row)">
                <el-icon><View /></el-icon>
                {{ t('policies.details') }}
              </el-button>
              <el-button size="small" @click="openNodeDetail(row)">
                {{ t('policies.openNodeDetail') }}
              </el-button>
              <el-button size="small" type="primary" @click="goToKind(row.kind, row)">
                {{ t('policies.goToDomain') }}
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </DataPanel>

    <el-drawer v-model="detailVisible" :title="t('policies.policyDetails')" size="45%">
      <template v-if="selectedPolicy">
        <el-descriptions :column="1" border>
          <el-descriptions-item :label="t('policies.policyId')">{{ selectedPolicy.policyId }}</el-descriptions-item>
          <el-descriptions-item :label="t('policies.policyKind')">{{ kindLabel(selectedPolicy.kind) }}</el-descriptions-item>
          <el-descriptions-item :label="t('policies.targetNode')">{{ selectedPolicy.nodeName }}</el-descriptions-item>
          <el-descriptions-item :label="t('policyTerms.deliveryStatus')">{{ statusLabel(selectedPolicy.status, t) }}</el-descriptions-item>
          <el-descriptions-item :label="t('policies.commandId')">{{ selectedPolicy.lastDeliveryCommandId || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('policies.version')">{{ selectedPolicy.version || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('policies.desiredVersion')">{{ selectedPolicy.desiredStateVersion || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('policies.appliedVersion')">{{ selectedPolicy.appliedStateVersion || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('policies.observedState')">{{ statusLabel(selectedPolicy.observedState, t) }}</el-descriptions-item>
          <el-descriptions-item :label="t('policies.convergence')">{{ convergenceLabel(selectedPolicy.stateConvergence) }}</el-descriptions-item>
          <el-descriptions-item :label="t('policies.observedMessage')" :span="1">{{ selectedPolicy.observedMessage || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="t('policies.observedAt')">{{ formatTimestamp(selectedPolicy.observedAt) }}</el-descriptions-item>
        </el-descriptions>

        <h4 class="section-title">{{ t('policies.spec') }}</h4>
        <pre class="spec-block">{{ prettySpec(selectedPolicy.spec) }}</pre>

        <h4 class="section-title">{{ t('policies.deliveryHistory') }}</h4>
        <div v-if="selectedPolicy.deliveryHistory.length === 0" class="empty-history">
          {{ t('policies.noDeliveryHistory') }}
        </div>
        <div v-else class="delivery-history">
          <div
            v-for="item in selectedPolicy.deliveryHistory"
            :key="item.id"
            class="delivery-item"
            :class="{ 'delivery-item-match': isDeliveryMatch(item) }"
          >
            <div class="delivery-main">
              <el-tag size="small" :type="commandStatusTagType(item.command_status)">
                {{ commandStatusLabel(item.command_status, t) }}
              </el-tag>
              <span class="delivery-command">{{ item.command_id }}</span>
            </div>
            <div class="delivery-meta">
              <span>{{ item.action || '-' }}</span>
              <span>{{ formatTimestamp(item.updated_at) }}</span>
            </div>
            <div v-if="item.last_error" class="delivery-error">{{ item.last_error }}</div>
          </div>
        </div>

        <div class="drawer-actions">
          <el-button @click="openNodeDetail(selectedPolicy)">{{ t('policies.openNodeMonitor') }}</el-button>
          <el-button v-if="canRetryPolicy(selectedPolicy)" type="warning" @click="retryPolicyDelivery(selectedPolicy)">{{ t('policies.retryDelivery') }}</el-button>
          <el-button type="primary" @click="goToKind(selectedPolicy.kind, selectedPolicy)">{{ t('policies.goToDomain') }}</el-button>
        </div>
      </template>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Collection, Connection, Histogram, Lock, Refresh, View } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import DataPanel from '@/components/ui/DataPanel.vue'
import FilterBar from '@/components/ui/FilterBar.vue'
import MetricStrip from '@/components/ui/MetricStrip.vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import { usePolicyApi } from '@/composables/usePolicyApi'
import { usePermission } from '@/composables/usePermission'
import { useTenantChangeReload } from '@/composables/useTenantChangeReload'
import { t } from '@/i18n'
import type { NormalizedPolicy, PolicyDelivery, PolicyKind } from '@/types'
import {
  commandStatusLabel,
  commandStatusTagType,
  isRetryablePolicyStatus,
  policyStatusLabel as statusLabel,
  policyStatusTagType as statusTagType
} from '@/utils/controlLoopStatus'

const router = useRouter()
const route = useRoute()
const { hasPermission } = usePermission()

const loading = ref(false)
const policies = ref<NormalizedPolicy[]>([])
const detailVisible = ref(false)
const selectedPolicy = ref<NormalizedPolicy | null>(null)
const autoFocusedPolicyId = ref('')

interface PolicyFilters {
  keyword: string
  kind: string
  status: string
  statusGroup: string
  nodeId: string
}

interface RouteContext {
  nodeId: string
  policyRef: string
  kind: string
  commandId: string
}

interface MetricFilter {
  kind?: string
  status?: string
  statusGroup?: string
}

interface PolicyMetric {
  key: string
  label: string
  value: number
  meta: string
  status: string
  clickable: boolean
  filter: MetricFilter
}

interface NodeOption {
  id?: string
  name?: string
}

interface NodeDetailOptions {
  commandId?: string
  focus?: string
}

const filters = ref<PolicyFilters>({
  keyword: '',
  kind: '',
  status: '',
  statusGroup: '',
  nodeId: ''
})

const activePolicyStatuses = new Set(['pending', 'in_progress', 'sent', 'acknowledged'])
const failedPolicyStatuses = new Set(['error', 'failed', 'timeout', 'timed_out'])
const policyMatchesStatusGroup = (policy: NormalizedPolicy, group: string) => {
  switch (group) {
    case 'active':
      return activePolicyStatuses.has(policy.status || '')
    case 'failed':
      return failedPolicyStatuses.has(policy.status || '') || failedPolicyStatuses.has(policy.observedState || '')
    default:
      return true
  }
}

const stringValue = (value: unknown): string => typeof value === 'string' ? value : ''

const kindLabel = (kind?: string) => {
  switch (kind) {
    case 'acl': return 'ACL'
    case 'qos': return 'QoS'
    case 'route': return t('policies.route')
    default: return kind || t('policies.kindUnknown')
  }
}

const kindTagType = (kind?: string) => {
  switch (kind) {
    case 'acl': return 'danger'
    case 'qos': return 'warning'
    case 'route': return 'success'
    default: return 'info'
  }
}

const convergenceLabel = (value?: string) => {
  const labels: Record<string, string> = {
    converged: t('policies.converged'),
    pending: t('policies.pending'),
    diverged: t('policies.diverged'),
    idle: t('policies.idle')
  }
  return labels[value || ''] || value || t('policies.kindUnknown')
}

const convergenceTagType = (value?: string) => {
  switch (value) {
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

const shortCommandId = (value?: string) => value ? value.slice(0, 8) : '-'

const prettySpec = (spec?: Record<string, unknown>) => JSON.stringify(spec || {}, null, 2)

const formatTimestamp = (value?: string | null) => {
  if (!value) {
    return '-'
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return String(value)
  }
  return date.toLocaleString()
}

const queryString = (...keys: string[]) => {
  for (const key of keys) {
    const value = route.query[key]
    if (typeof value === 'string' && value.trim()) {
      return value
    }
  }
  return ''
}

const routeContext = computed<RouteContext>(() => ({
  nodeId: queryString('nodeId', 'node_id'),
  policyRef: queryString('policyRef', 'policy_ref'),
  kind: queryString('kind', 'policyDomain', 'policy_domain'),
  commandId: queryString('commandId', 'command_id')
}))

const hasRouteContext = computed(() => Object.values(routeContext.value).some(Boolean))

const routeContextSummary = computed(() => {
  const parts = []
  if (routeContext.value.kind) {
    parts.push(`${t('policies.kind')}: ${routeContext.value.kind}`)
  }
  if (routeContext.value.policyRef) {
    parts.push(`${t('policies.policy')}: ${routeContext.value.policyRef}`)
  }
  if (routeContext.value.commandId) {
    parts.push(`${t('policies.command')}: ${routeContext.value.commandId}`)
  }
  if (routeContext.value.nodeId) {
    parts.push(`${t('policies.node')}: ${routeContext.value.nodeId}`)
  }
  return parts.join(' | ')
})

const summarizePolicy = (policy: NormalizedPolicy) => {
  const spec = (policy.spec || {}) as Record<string, unknown>
  switch (policy.kind) {
    case 'acl':
      return `${spec.src_net || '*'} -> ${spec.dst_net || '*'} / proto ${spec.protocol ?? 0} / ports ${spec.min_port ?? 0}-${spec.max_port ?? 65535}`
    case 'qos':
      return `${spec.direction || 'egress'} / group ${spec.dst_cidr || spec.src_cidr || 'any'} / ${spec.rate_bps || 0} bps / ${spec.mode || 'auto'}`
    case 'route':
      return spec.cidr || policy.policyRef || '-'
    default:
      return policy.policyRef || '-'
  }
}

const nodeOptions = computed<NodeOption[]>(() => {
  const seen = new Map<string | undefined, NodeOption>()
  for (const policy of policies.value) {
    if (!seen.has(policy.nodeId)) {
      seen.set(policy.nodeId, { id: policy.nodeId, name: policy.nodeName })
    }
  }
  return Array.from(seen.values())
})

const filteredPolicies = computed<NormalizedPolicy[]>(() => {
  const keyword = filters.value.keyword.trim().toLowerCase()
  return policies.value.filter((policy) => {
    if (filters.value.kind && policy.kind !== filters.value.kind) {
      return false
    }
    if (filters.value.status && policy.status !== filters.value.status) {
      return false
    }
    if (!filters.value.status && filters.value.statusGroup && !policyMatchesStatusGroup(policy, filters.value.statusGroup)) {
      return false
    }
    if (filters.value.nodeId && policy.nodeId !== filters.value.nodeId) {
      return false
    }
    if (keyword) {
      const haystack = [
        policy.name,
        policy.nodeName,
        policy.policyRef,
        summarizePolicy(policy)
      ].join(' ').toLowerCase()
      if (!haystack.includes(keyword)) {
        return false
      }
    }
    return true
  })
})

const findContextPolicy = (): NormalizedPolicy | null => {
  if (!hasRouteContext.value) {
    return null
  }
  if (!routeContext.value.policyRef && !routeContext.value.commandId) {
    return null
  }
  return filteredPolicies.value.find((policy) => {
    if (routeContext.value.nodeId && policy.nodeId !== routeContext.value.nodeId) {
      return false
    }
    if (routeContext.value.kind && policy.kind !== routeContext.value.kind) {
      return false
    }
    if (routeContext.value.policyRef && policy.policyRef !== routeContext.value.policyRef) {
      return false
    }
    if (!routeContext.value.policyRef && routeContext.value.commandId && !policy.deliveryHistory.some((item) => item.command_id === routeContext.value.commandId)) {
      return false
    }
    return true
  }) || null
}

const stats = computed(() => {
  const current = filteredPolicies.value
  return {
    total: current.length,
    acl: current.filter((item) => item.kind === 'acl').length,
    qos: current.filter((item) => item.kind === 'qos').length,
    route: current.filter((item) => item.kind === 'route').length,
    applied: current.filter((item) => item.status === 'applied').length,
    pending: current.filter((item) => ['pending', 'in_progress', 'sent', 'acknowledged'].includes(item.status)).length,
    failed: current.filter((item) => policyMatchesStatusGroup(item, 'failed')).length
  }
})

const policyMetricItems = computed<PolicyMetric[]>(() => [
  {
    key: 'total',
    label: t('policies.total'),
    value: stats.value.total,
    meta: t('policies.totalMeta'),
    status: 'info',
    clickable: true,
    filter: { kind: '', status: '' }
  },
  {
    key: 'acl',
    label: 'ACL',
    value: stats.value.acl,
    meta: t('policies.aclMeta'),
    status: 'danger',
    clickable: true,
    filter: { kind: 'acl' }
  },
  {
    key: 'qos',
    label: 'QoS',
    value: stats.value.qos,
    meta: t('policies.qosMeta'),
    status: 'warning',
    clickable: true,
    filter: { kind: 'qos' }
  },
  {
    key: 'route',
    label: 'Route',
    value: stats.value.route,
    meta: t('policies.routeMeta'),
    status: 'success',
    clickable: true,
    filter: { kind: 'route' }
  },
  {
    key: 'applied',
    label: t('policies.applied'),
    value: stats.value.applied,
    meta: t('policies.appliedMeta'),
    status: 'success',
    clickable: true,
    filter: { status: 'applied' }
  },
  {
    key: 'pending',
    label: t('policies.pending'),
    value: stats.value.pending,
    meta: t('policies.pendingMeta'),
    status: 'warning',
    clickable: true,
    filter: { status: '', statusGroup: 'active' }
  },
  {
    key: 'failed',
    label: t('policies.failed'),
    value: stats.value.failed,
    meta: t('policies.failedMeta'),
    status: 'danger',
    clickable: true,
    filter: { status: '', statusGroup: 'failed' }
  }
])

const handleMetricSelect = (metric?: PolicyMetric) => {
  const filter = metric?.filter || {}
  if (Object.prototype.hasOwnProperty.call(filter, 'kind')) {
    filters.value.kind = filter.kind || ''
  }
  if (Object.prototype.hasOwnProperty.call(filter, 'status')) {
    filters.value.status = filter.status || ''
  }
  if (Object.prototype.hasOwnProperty.call(filter, 'statusGroup')) {
    filters.value.statusGroup = filter.statusGroup || ''
  } else if (Object.prototype.hasOwnProperty.call(filter, 'status')) {
    filters.value.statusGroup = ''
  }
}

const errorMessage = (error: unknown, fallback = t('policyTerms.unknownError')): string =>
  error instanceof Error ? error.message : (typeof error === 'string' ? error : fallback)

const fetchPolicies = async () => {
  loading.value = true
  try {
    policies.value = await usePolicyApi.listPolicies()
    syncSelectedPolicy()
    focusPolicyFromRoute()
  } catch (error) {
    console.error('Failed to fetch unified policies:', error)
    ElMessage.error(`${t('policies.fetchFailed')}: ${errorMessage(error, String(error || t('policyTerms.unknownError')))}`)
  } finally {
    loading.value = false
  }
}

const refreshData = async () => {
  await fetchPolicies()
}

const syncFiltersFromRoute = () => {
  filters.value.nodeId = routeContext.value.nodeId
  filters.value.keyword = routeContext.value.policyRef
  filters.value.kind = routeContext.value.kind
  filters.value.statusGroup = ''
}

const syncSelectedPolicy = () => {
  if (!selectedPolicy.value) {
    return
  }
  const currentPolicyId = selectedPolicy.value.policyId
  const fresh = policies.value.find((policy) => policy.policyId === currentPolicyId)
  if (fresh) {
    selectedPolicy.value = fresh
  }
}

const focusPolicyFromRoute = () => {
  if (!hasRouteContext.value) {
    autoFocusedPolicyId.value = ''
    return
  }
  const matched = findContextPolicy()
  if (!matched) {
    return
  }
  selectedPolicy.value = matched
  const matchedPolicyId = matched.policyId || ''
  if (autoFocusedPolicyId.value !== matchedPolicyId || !detailVisible.value) {
    detailVisible.value = true
    autoFocusedPolicyId.value = matchedPolicyId
  }
}

const policyPageQuery = (policy?: NormalizedPolicy | null): Record<string, string> => {
  const query: Record<string, string> = {}
  const nodeId = policy?.nodeId || routeContext.value.nodeId
  const policyRef = policy?.policyRef || routeContext.value.policyRef
  const commandId = commandIdForPolicy(policy) || routeContext.value.commandId

  if (nodeId) {
    query.nodeId = nodeId
  }
  if (policyRef) {
    query.policyRef = policyRef
  }
  if (commandId) {
    query.commandId = commandId
  }
  return query
}

const goToKind = (kind: PolicyKind | string, policy: NormalizedPolicy | null = selectedPolicy.value) => {
  const query = policyPageQuery(policy)
  switch (kind) {
    case 'acl':
      router.push({ name: 'ACLRules', query })
      break
    case 'qos':
      router.push({ name: 'BandwidthControl', query })
      break
    case 'route':
      router.push({ name: 'Routing', query })
      break
    default:
      break
  }
}

const goToIpGroups = () => {
  const query: Record<string, string> = {}
  if (routeContext.value.nodeId) query.nodeId = routeContext.value.nodeId
  if (routeContext.value.policyRef) query.policyRef = routeContext.value.policyRef
  if (routeContext.value.kind) query.kind = routeContext.value.kind
  if (routeContext.value.commandId) query.commandId = routeContext.value.commandId
  router.push({ name: 'IPGroups', query })
}

const showDetails = (policy: NormalizedPolicy) => {
  selectedPolicy.value = policy
  detailVisible.value = true
}

const commandIdForPolicy = (policy?: NormalizedPolicy | null): string => {
  if (!policy) {
    return ''
  }
  return policy.lastDeliveryCommandId ||
    policy.last_delivery_command_id ||
    policy.lastDelivery?.command_id ||
    policy.last_delivery?.command_id ||
    policy.deliveryHistory?.find((item: PolicyDelivery) => item.command_id)?.command_id ||
    policy.delivery_history?.find((item: PolicyDelivery) => item.command_id)?.command_id ||
    ''
}

const openNodeDetail = (policy: NormalizedPolicy, options: NodeDetailOptions = {}) => {
  if (!policy?.nodeId) {
    ElMessage.warning(t('policies.noTargetNode'))
    return
  }
  const commandId = options.commandId || routeContext.value.commandId
  const focus = options.focus || (commandId ? 'policies' : '')
  router.push({
    name: 'NodeMonitorDetail',
    params: { nodeId: policy.nodeId },
    query: {
      ...(commandId ? { commandId } : {}),
      ...(focus ? { focus } : {}),
      ...(policy.policyRef ? { policyRef: policy.policyRef } : {}),
      ...(policy.kind ? { policyDomain: policy.kind } : {})
    }
  })
}

const policyWritePermission = (kind?: string) => {
  switch (kind) {
    case 'acl':
      return 'acls:write'
    case 'qos':
      return 'qos:write'
    case 'route':
      return 'routes:write'
    default:
      return ''
  }
}

const canRetryPolicy = (policy?: NormalizedPolicy | null) => {
  if (!policy) return false
  const permission = policyWritePermission(policy.kind)
  if (!permission || !hasPermission(permission)) return false
  return isRetryablePolicyStatus(policy.status) || isRetryablePolicyStatus(policy.observedState)
}

const retryPolicyDelivery = async (policy: NormalizedPolicy) => {
  if (!canRetryPolicy(policy)) {
    ElMessage.warning(t('policies.retryNotAllowed'))
    return
  }

  try {
    const response = await usePolicyApi.retryPolicySync({
      nodeId: policy.nodeId,
      kind: policy.kind,
      policyRef: policy.policyRef,
      policyName: policy.name
    })
    ElMessage.success(t('policyTerms.retryQueued'))
    await fetchPolicies()
    const commandId = commandIdForPolicy(response)
    if (commandId) {
      openNodeDetail({
        ...policy,
        ...response,
        nodeId: response.nodeId || policy.nodeId,
        policyRef: response.policyRef || policy.policyRef,
        kind: response.kind || policy.kind
      }, { commandId, focus: 'commands' })
    }
  } catch (error) {
    ElMessage.error(`${t('policyTerms.retryFailed')}: ${errorMessage(error)}`)
  }
}

const isDeliveryMatch = (delivery?: PolicyDelivery | null) => {
  if (!delivery) {
    return false
  }
  if (routeContext.value.commandId && delivery.command_id === routeContext.value.commandId) {
    return true
  }
  const deliveryRecord = delivery as PolicyDelivery & { policy_ref?: string }
  if (routeContext.value.policyRef && deliveryRecord.policy_ref === routeContext.value.policyRef) {
    return true
  }
  return false
}

const policyRowClassName = ({ row }: { row?: NormalizedPolicy }) => {
  if (!row) {
    return ''
  }
  const matched = findContextPolicy()
  return matched?.policyId === row.policyId ? 'context-match-row' : ''
}

onMounted(() => {
  syncFiltersFromRoute()
  fetchPolicies()
})

useTenantChangeReload(async () => {
  policies.value = []
  selectedPolicy.value = null
  detailVisible.value = false
  await fetchPolicies()
})

watch(() => route.fullPath, () => {
  syncFiltersFromRoute()
  syncSelectedPolicy()
  focusPolicyFromRoute()
})
</script>

<style scoped>
.policy-center {
  padding: 0;
}

.policy-panel-body {
  padding: 14px 16px 16px;
}

.filter-item {
  width: 180px;
}

.keyword-filter {
  width: 320px;
}

.context-alert-body {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.context-alert-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.section-title {
  margin: 24px 0 12px;
  font-size: 15px;
  font-weight: 600;
}

.spec-block {
  margin: 0;
  padding: 12px;
  background: #0f172a;
  color: #e2e8f0;
  border-radius: var(--aria-radius);
  overflow-x: auto;
  font-size: 12px;
  line-height: 1.5;
}

.delivery-history {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.delivery-item {
  padding: 12px;
  border: 1px solid var(--aria-border-primary);
  border-radius: var(--aria-radius);
  background: var(--aria-content-bg-tertiary);
}

.delivery-item-match {
  border-color: rgba(59, 130, 246, 0.45);
  background: rgba(59, 130, 246, 0.08);
}

.delivery-main,
.delivery-meta {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.delivery-meta {
  margin-top: 8px;
  color: var(--aria-text-secondary);
  font-size: 12px;
}

.delivery-command {
  font-family: Menlo, Monaco, Consolas, "Courier New", monospace;
  font-size: 12px;
}

.delivery-error {
  margin-top: 8px;
  color: var(--aria-danger);
  font-size: 12px;
}

.empty-history {
  color: var(--aria-text-muted);
}

.drawer-actions {
  margin-top: 20px;
  display: flex;
  gap: 12px;
}

:deep(.context-match-row > td) {
  background: rgba(59, 130, 246, 0.10) !important;
}

@media (max-width: 768px) {
  .filter-item,
  .keyword-filter {
    width: 100%;
  }
}
</style>
