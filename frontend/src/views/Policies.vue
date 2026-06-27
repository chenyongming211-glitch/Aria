<template>
  <div class="policy-center page-shell">
    <section class="page-hero">
      <div class="page-hero-main">
        <div class="page-eyebrow">Policy Operations</div>
        <h1 class="page-heading">Policy Center</h1>
        <p class="page-description">统一查看租户内 ACL、QoS 和 Route 策略，以及它们最近的交付状态。</p>
      </div>
      <div class="page-actions">
        <el-button @click="goToKind('acl')">
          <el-icon><Lock /></el-icon>
          ACL 规则
        </el-button>
        <el-button @click="goToKind('qos')">
          <el-icon><Histogram /></el-icon>
          流量控制
        </el-button>
        <el-button @click="goToKind('route')">
          <el-icon><Connection /></el-icon>
          路由管理
        </el-button>
        <el-button type="primary" @click="refreshData">
          <el-icon><Refresh /></el-icon>
          刷新
        </el-button>
      </div>
    </section>

    <el-card class="policy-card table-card" shadow="never">
      <template #header>
        <div class="card-header panel-header">
          <div>
            <h3 class="panel-title">Unified policy inventory</h3>
            <p class="panel-subtitle">具体写操作仍保留在 ACL、QoS、Route 各自专页。</p>
          </div>
        </div>
      </template>

      <el-alert
        title="统一策略视图"
        type="info"
        :closable="false"
        description="这一页不再把 ACL、QoS、Route 当成几套平行概念，而是统一成租户作用域下的策略资源。具体写操作仍然保留在各自专页。"
        style="margin-bottom: 20px;"
      />

      <el-alert
        v-if="hasRouteContext"
        title="Monitoring Context"
        type="warning"
        :closable="false"
        style="margin-bottom: 20px;"
      >
        <div class="context-alert-body">
          <span>{{ routeContextSummary }}</span>
          <div class="context-alert-actions">
            <el-button v-if="selectedPolicy" size="small" @click="openNodeDetail(selectedPolicy)">
              打开节点详情
            </el-button>
            <el-button v-if="selectedPolicy" size="small" type="primary" @click="showDetails(selectedPolicy)">
              聚焦交付历史
            </el-button>
          </div>
        </div>
      </el-alert>

      <el-row :gutter="16" class="stats-row">
        <el-col :xs="12" :sm="8" :lg="4">
          <el-card class="stat-card kpi-card" shadow="never">
            <div class="stat-value">{{ stats.total }}</div>
            <div class="stat-label">总策略数</div>
          </el-card>
        </el-col>
        <el-col :xs="12" :sm="8" :lg="4">
          <el-card class="stat-card kpi-card" shadow="never">
            <div class="stat-value">{{ stats.acl }}</div>
            <div class="stat-label">ACL</div>
          </el-card>
        </el-col>
        <el-col :xs="12" :sm="8" :lg="4">
          <el-card class="stat-card kpi-card" shadow="never">
            <div class="stat-value">{{ stats.qos }}</div>
            <div class="stat-label">QoS</div>
          </el-card>
        </el-col>
        <el-col :xs="12" :sm="8" :lg="4">
          <el-card class="stat-card kpi-card" shadow="never">
            <div class="stat-value">{{ stats.route }}</div>
            <div class="stat-label">Route</div>
          </el-card>
        </el-col>
        <el-col :xs="12" :sm="8" :lg="4">
          <el-card class="stat-card kpi-card" shadow="never">
            <div class="stat-value">{{ stats.applied }}</div>
            <div class="stat-label">已应用</div>
          </el-card>
        </el-col>
        <el-col :xs="12" :sm="8" :lg="4">
          <el-card class="stat-card kpi-card" shadow="never">
            <div class="stat-value">{{ stats.pending }}</div>
            <div class="stat-label">待下发</div>
          </el-card>
        </el-col>
      </el-row>

      <div class="filters">
        <el-input
          v-model="filters.keyword"
          placeholder="按名称、节点或 policy_ref 搜索"
          clearable
          class="filter-item keyword-filter"
        />
        <el-select v-model="filters.kind" clearable placeholder="策略类型" class="filter-item">
          <el-option label="ACL" value="acl" />
          <el-option label="QoS" value="qos" />
          <el-option label="Route" value="route" />
        </el-select>
        <el-select v-model="filters.status" clearable placeholder="交付状态" class="filter-item">
          <el-option label="已应用" value="applied" />
          <el-option label="待下发" value="pending" />
          <el-option label="下发中" value="in_progress" />
          <el-option label="已过期" value="stale" />
          <el-option label="失败" value="error" />
          <el-option label="空闲" value="idle" />
        </el-select>
        <el-select v-model="filters.nodeId" clearable placeholder="目标节点" class="filter-item">
          <el-option
            v-for="node in nodeOptions"
            :key="node.id"
            :label="node.name"
            :value="node.id"
          />
        </el-select>
      </div>

      <el-table
        :data="filteredPolicies"
        :row-class-name="policyRowClassName"
        stripe
        style="width: 100%"
        v-loading="loading"
      >
        <el-table-column prop="kind" label="类型" width="110">
          <template #default="{ row }">
            <el-tag :type="kindTagType(row.kind)">
              {{ kindLabel(row.kind) }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="name" label="名称" min-width="200" show-overflow-tooltip />

        <el-table-column prop="nodeName" label="目标节点" width="180" show-overflow-tooltip />

        <el-table-column label="摘要" min-width="320" show-overflow-tooltip>
          <template #default="{ row }">
            {{ summarizePolicy(row) }}
          </template>
        </el-table-column>

        <el-table-column label="启用" width="90">
          <template #default="{ row }">
            <el-tag size="small" :type="row.enabled ? 'success' : 'info'">
              {{ row.enabled ? '启用' : '禁用' }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="priority" label="优先级" width="90" />

        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <el-tag size="small" :type="statusTagType(row.status)">
              {{ statusLabel(row.status, t) }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="Convergence" width="120">
          <template #default="{ row }">
            <el-tag size="small" :type="convergenceTagType(row.stateConvergence)">
              {{ convergenceLabel(row.stateConvergence) }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="pendingCmds" label="待执行" width="90" />

        <el-table-column label="最近命令" width="150">
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

        <el-table-column prop="lastDeliveryError" label="失败原因" min-width="180" show-overflow-tooltip />

        <el-table-column label="操作" width="260">
          <template #default="{ row }">
            <el-button
              v-if="canRetryPolicy(row)"
              size="small"
              type="warning"
              @click="retryPolicyDelivery(row)"
            >
              重试
            </el-button>
            <el-button size="small" @click="showDetails(row)">
              <el-icon><View /></el-icon>
              详情
            </el-button>
            <el-button size="small" @click="openNodeDetail(row)">
              节点详情
            </el-button>
            <el-button size="small" type="primary" @click="goToKind(row.kind, row)">
              前往专页
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-drawer v-model="detailVisible" title="策略详情" size="45%">
      <template v-if="selectedPolicy">
        <el-descriptions :column="1" border>
          <el-descriptions-item label="策略 ID">{{ selectedPolicy.policyId }}</el-descriptions-item>
          <el-descriptions-item label="策略类型">{{ kindLabel(selectedPolicy.kind) }}</el-descriptions-item>
          <el-descriptions-item label="目标节点">{{ selectedPolicy.nodeName }}</el-descriptions-item>
          <el-descriptions-item label="交付状态">{{ statusLabel(selectedPolicy.status, t) }}</el-descriptions-item>
          <el-descriptions-item label="最近命令 ID">{{ selectedPolicy.lastDeliveryCommandId || '-' }}</el-descriptions-item>
          <el-descriptions-item label="版本">{{ selectedPolicy.version || '-' }}</el-descriptions-item>
          <el-descriptions-item label="Desired Version">{{ selectedPolicy.desiredStateVersion || '-' }}</el-descriptions-item>
          <el-descriptions-item label="Applied Version">{{ selectedPolicy.appliedStateVersion || '-' }}</el-descriptions-item>
          <el-descriptions-item label="Observed State">{{ statusLabel(selectedPolicy.observedState, t) }}</el-descriptions-item>
          <el-descriptions-item label="Convergence">{{ convergenceLabel(selectedPolicy.stateConvergence) }}</el-descriptions-item>
          <el-descriptions-item label="Observed Message" :span="1">{{ selectedPolicy.observedMessage || '-' }}</el-descriptions-item>
          <el-descriptions-item label="Observed At">{{ formatTimestamp(selectedPolicy.observedAt) }}</el-descriptions-item>
        </el-descriptions>

        <h4 class="section-title">Spec</h4>
        <pre class="spec-block">{{ prettySpec(selectedPolicy.spec) }}</pre>

        <h4 class="section-title">交付历史</h4>
        <div v-if="selectedPolicy.deliveryHistory.length === 0" class="empty-history">
          暂无交付记录
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
          <el-button @click="openNodeDetail(selectedPolicy)">打开节点监控</el-button>
          <el-button v-if="canRetryPolicy(selectedPolicy)" type="warning" @click="retryPolicyDelivery(selectedPolicy)">重试下发</el-button>
          <el-button type="primary" @click="goToKind(selectedPolicy.kind, selectedPolicy)">前往专页</el-button>
        </div>
      </template>
    </el-drawer>
  </div>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Connection, Histogram, Lock, Refresh, View } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { usePolicyApi } from '@/composables/usePolicyApi'
import { usePermission } from '@/composables/usePermission'
import { useTenantChangeReload } from '@/composables/useTenantChangeReload'
import { t } from '@/i18n'
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
const policies = ref([])
const detailVisible = ref(false)
const selectedPolicy = ref(null)
const autoFocusedPolicyId = ref('')
const filters = ref({
  keyword: '',
  kind: '',
  status: '',
  nodeId: ''
})

const kindLabel = (kind) => {
  switch (kind) {
    case 'acl': return 'ACL'
    case 'qos': return 'QoS'
    case 'route': return 'Route'
    default: return kind || 'Unknown'
  }
}

const kindTagType = (kind) => {
  switch (kind) {
    case 'acl': return 'danger'
    case 'qos': return 'warning'
    case 'route': return 'success'
    default: return 'info'
  }
}

const convergenceLabel = (value) => {
  const labels = {
    converged: 'Converged',
    pending: 'Pending',
    diverged: 'Diverged',
    idle: 'Idle'
  }
  return labels[value] || value || 'Unknown'
}

const convergenceTagType = (value) => {
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

const shortCommandId = (value) => value ? value.slice(0, 8) : '-'

const prettySpec = (spec) => JSON.stringify(spec || {}, null, 2)

const formatTimestamp = (value) => {
  if (!value) {
    return '-'
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return String(value)
  }
  return date.toLocaleString()
}

const routeContext = computed(() => ({
  nodeId: typeof route.query.nodeId === 'string' ? route.query.nodeId : '',
  policyRef: typeof route.query.policyRef === 'string' ? route.query.policyRef : '',
  kind: typeof route.query.kind === 'string' ? route.query.kind : '',
  commandId: typeof route.query.commandId === 'string' ? route.query.commandId : ''
}))

const hasRouteContext = computed(() => Object.values(routeContext.value).some(Boolean))

const routeContextSummary = computed(() => {
  const parts = []
  if (routeContext.value.kind) {
    parts.push(`Kind: ${routeContext.value.kind}`)
  }
  if (routeContext.value.policyRef) {
    parts.push(`Policy: ${routeContext.value.policyRef}`)
  }
  if (routeContext.value.commandId) {
    parts.push(`Command: ${routeContext.value.commandId}`)
  }
  if (routeContext.value.nodeId) {
    parts.push(`Node: ${routeContext.value.nodeId}`)
  }
  return parts.join(' | ')
})

const summarizePolicy = (policy) => {
  const spec = policy.spec || {}
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

const nodeOptions = computed(() => {
  const seen = new Map()
  for (const policy of policies.value) {
    if (!seen.has(policy.nodeId)) {
      seen.set(policy.nodeId, { id: policy.nodeId, name: policy.nodeName })
    }
  }
  return Array.from(seen.values())
})

const filteredPolicies = computed(() => {
  const keyword = filters.value.keyword.trim().toLowerCase()
  return policies.value.filter((policy) => {
    if (filters.value.kind && policy.kind !== filters.value.kind) {
      return false
    }
    if (filters.value.status && policy.status !== filters.value.status) {
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

const findContextPolicy = () => {
  if (!hasRouteContext.value) {
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
    if (routeContext.value.commandId && !policy.deliveryHistory.some((item) => item.command_id === routeContext.value.commandId)) {
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
    pending: current.filter((item) => ['pending', 'in_progress', 'sent', 'acknowledged'].includes(item.status)).length
  }
})

const fetchPolicies = async () => {
  loading.value = true
  try {
    policies.value = await usePolicyApi.listPolicies()
    syncSelectedPolicy()
    focusPolicyFromRoute()
  } catch (error) {
    console.error('Failed to fetch unified policies:', error)
    ElMessage.error(`获取统一策略视图失败: ${error.message || error}`)
  } finally {
    loading.value = false
  }
}

const refreshData = async () => {
  await fetchPolicies()
}

const syncFiltersFromRoute = () => {
  filters.value.nodeId = typeof route.query.nodeId === 'string' ? route.query.nodeId : ''
  filters.value.keyword = typeof route.query.policyRef === 'string' ? route.query.policyRef : ''
  filters.value.kind = typeof route.query.kind === 'string' ? route.query.kind : ''
}

const syncSelectedPolicy = () => {
  if (!selectedPolicy.value) {
    return
  }
  const fresh = policies.value.find((policy) => policy.policyId === selectedPolicy.value.policyId)
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
  if (autoFocusedPolicyId.value !== matched.policyId || !detailVisible.value) {
    detailVisible.value = true
    autoFocusedPolicyId.value = matched.policyId
  }
}

const policyPageQuery = (policy) => {
  const query = {}
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

const goToKind = (kind, policy = selectedPolicy.value) => {
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

const showDetails = (policy) => {
  selectedPolicy.value = policy
  detailVisible.value = true
}

const commandIdForPolicy = (policy) => {
  if (!policy) {
    return ''
  }
  return policy.lastDeliveryCommandId ||
    policy.last_delivery_command_id ||
    policy.lastDelivery?.command_id ||
    policy.last_delivery?.command_id ||
    policy.deliveryHistory?.find((item) => item.command_id)?.command_id ||
    policy.delivery_history?.find((item) => item.command_id)?.command_id ||
    ''
}

const openNodeDetail = (policy, options = {}) => {
  if (!policy?.nodeId) {
    ElMessage.warning('该策略没有目标节点')
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

const policyWritePermission = (kind) => {
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

const canRetryPolicy = (policy) => {
  if (!policy) return false
  const permission = policyWritePermission(policy.kind)
  if (!permission || !hasPermission(permission)) return false
  return isRetryablePolicyStatus(policy.status || policy.observedState)
}

const retryPolicyDelivery = async (policy) => {
  if (!canRetryPolicy(policy)) {
    ElMessage.warning('当前策略状态或权限不允许重试')
    return
  }

  try {
    const response = await usePolicyApi.retryPolicySync({
      nodeId: policy.nodeId,
      kind: policy.kind,
      policyRef: policy.policyRef,
      policyName: policy.name
    })
    ElMessage.success('重试下发已排队')
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
    ElMessage.error(`重试失败: ${error.message || '未知错误'}`)
  }
}

const isDeliveryMatch = (delivery) => {
  if (!delivery) {
    return false
  }
  if (routeContext.value.commandId && delivery.command_id === routeContext.value.commandId) {
    return true
  }
  if (routeContext.value.policyRef && delivery.policy_ref === routeContext.value.policyRef) {
    return true
  }
  return false
}

const policyRowClassName = ({ row }) => {
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

.policy-card {
  border-radius: var(--aria-radius-lg);
}

.header-subtitle {
  margin: 8px 0 0;
  color: var(--aria-text-secondary);
  font-size: 13px;
}

.header-actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}

.stats-row {
  margin-bottom: 20px;
}

.stat-card {
  min-height: 86px;
  text-align: center;
  justify-content: center;
}

.stat-value {
  font-size: 24px;
  font-weight: 700;
  color: var(--aria-text-primary);
}

.stat-label {
  margin-top: 6px;
  color: var(--aria-text-secondary);
  font-size: 13px;
}

.filters {
  display: flex;
  gap: 12px;
  margin-bottom: 20px;
  flex-wrap: wrap;
  padding: 14px 16px;
  background: var(--aria-content-bg-tertiary);
  border: 1px solid var(--aria-border-primary);
  border-radius: var(--aria-radius-lg);
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
