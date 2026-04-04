<template>
  <div class="policy-center">
    <el-card>
      <template #header>
        <div class="card-header">
          <div>
            <h3>Policy Center</h3>
            <p class="header-subtitle">统一查看租户内的 ACL、QoS 和 Route 策略，以及它们最近的交付状态。</p>
          </div>
          <div class="header-actions">
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
        </div>
      </template>

      <el-alert
        title="统一策略视图"
        type="info"
        :closable="false"
        description="这一页不再把 ACL、QoS、Route 当成几套平行概念，而是统一成租户作用域下的策略资源。具体写操作仍然保留在各自专页。"
        style="margin-bottom: 20px;"
      />

      <el-row :gutter="16" class="stats-row">
        <el-col :span="4">
          <el-card class="stat-card">
            <div class="stat-value">{{ stats.total }}</div>
            <div class="stat-label">总策略数</div>
          </el-card>
        </el-col>
        <el-col :span="4">
          <el-card class="stat-card">
            <div class="stat-value">{{ stats.acl }}</div>
            <div class="stat-label">ACL</div>
          </el-card>
        </el-col>
        <el-col :span="4">
          <el-card class="stat-card">
            <div class="stat-value">{{ stats.qos }}</div>
            <div class="stat-label">QoS</div>
          </el-card>
        </el-col>
        <el-col :span="4">
          <el-card class="stat-card">
            <div class="stat-value">{{ stats.route }}</div>
            <div class="stat-label">Route</div>
          </el-card>
        </el-col>
        <el-col :span="4">
          <el-card class="stat-card">
            <div class="stat-value">{{ stats.applied }}</div>
            <div class="stat-label">已应用</div>
          </el-card>
        </el-col>
        <el-col :span="4">
          <el-card class="stat-card">
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
              {{ statusLabel(row.status) }}
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

        <el-table-column label="操作" width="180">
          <template #default="{ row }">
            <el-button size="small" @click="showDetails(row)">
              <el-icon><View /></el-icon>
              详情
            </el-button>
            <el-button size="small" type="primary" @click="goToKind(row.kind)">
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
          <el-descriptions-item label="交付状态">{{ statusLabel(selectedPolicy.status) }}</el-descriptions-item>
          <el-descriptions-item label="最近命令 ID">{{ selectedPolicy.lastDeliveryCommandId || '-' }}</el-descriptions-item>
          <el-descriptions-item label="版本">{{ selectedPolicy.version || '-' }}</el-descriptions-item>
          <el-descriptions-item label="Desired Version">{{ selectedPolicy.desiredStateVersion || '-' }}</el-descriptions-item>
          <el-descriptions-item label="Applied Version">{{ selectedPolicy.appliedStateVersion || '-' }}</el-descriptions-item>
          <el-descriptions-item label="Observed State">{{ statusLabel(selectedPolicy.observedState) }}</el-descriptions-item>
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
          <div v-for="item in selectedPolicy.deliveryHistory" :key="item.id" class="delivery-item">
            <div class="delivery-main">
              <el-tag size="small" :type="statusTagType(item.command_status)">
                {{ statusLabel(item.command_status) }}
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
      </template>
    </el-drawer>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Connection, Histogram, Lock, Refresh, View } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { usePolicyApi } from '@/composables/usePolicyApi'

const router = useRouter()

const loading = ref(false)
const policies = ref([])
const detailVisible = ref(false)
const selectedPolicy = ref(null)
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

const statusLabel = (status) => {
  const labels = {
    applied: '已应用',
    healthy: 'Healthy',
    pending: '待下发',
    in_progress: '下发中',
    error: '失败',
    idle: '空闲',
    sent: '已发送',
    acknowledged: '已确认',
    completed: '已完成',
    failed: '失败'
  }
  return labels[status] || status || '未知'
}

const statusTagType = (status) => {
  switch (status) {
    case 'applied':
    case 'completed':
      return 'success'
    case 'pending':
    case 'sent':
    case 'acknowledged':
    case 'in_progress':
      return 'warning'
    case 'error':
    case 'failed':
      return 'danger'
    default:
      return 'info'
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

const summarizePolicy = (policy) => {
  const spec = policy.spec || {}
  switch (policy.kind) {
    case 'acl':
      return `${spec.src_net || '*'} -> ${spec.dst_net || '*'} / proto ${spec.protocol ?? 0} / ports ${spec.min_port ?? 0}-${spec.max_port ?? 65535}`
    case 'qos':
      return `${spec.category || 'service'} / ${spec.src_ip || '*'} -> ${spec.dst_ip || '*'} / ${spec.bandwidth_mbps || 0} Mbps`
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

const goToKind = (kind) => {
  switch (kind) {
    case 'acl':
      router.push({ name: 'ACLRules' })
      break
    case 'qos':
      router.push({ name: 'BandwidthControl' })
      break
    case 'route':
      router.push({ name: 'Routing' })
      break
    default:
      break
  }
}

const showDetails = (policy) => {
  selectedPolicy.value = policy
  detailVisible.value = true
}

onMounted(() => {
  fetchPolicies()
})
</script>

<style scoped>
.policy-center {
  padding: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 20px;
}

.header-subtitle {
  margin: 8px 0 0;
  color: #909399;
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
  text-align: center;
}

.stat-value {
  font-size: 24px;
  font-weight: 700;
  color: #303133;
}

.stat-label {
  margin-top: 6px;
  color: #909399;
  font-size: 13px;
}

.filters {
  display: flex;
  gap: 12px;
  margin-bottom: 20px;
  flex-wrap: wrap;
}

.filter-item {
  width: 180px;
}

.keyword-filter {
  width: 320px;
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
  border-radius: 8px;
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
  border: 1px solid #ebeef5;
  border-radius: 8px;
  background: #fafafa;
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
  color: #909399;
  font-size: 12px;
}

.delivery-command {
  font-family: Menlo, Monaco, Consolas, "Courier New", monospace;
  font-size: 12px;
}

.delivery-error {
  margin-top: 8px;
  color: #f56c6c;
  font-size: 12px;
}

.empty-history {
  color: #909399;
}
</style>
