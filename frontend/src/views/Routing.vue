<template>
  <div class="routing">
    <el-card>
      <template #header>
        <div class="card-header">
          <h3>路由管理</h3>
          <div class="header-actions">
            <el-input
              v-model="searchQuery"
              placeholder="搜索节点、区域或 CIDR"
              style="width: 240px"
              clearable
            />
            <el-button type="primary" @click="loadRoutes">
              <el-icon><Refresh /></el-icon>
              刷新
            </el-button>
            <el-button v-if="hasPermission('routes:write')" type="primary" @click="showAddRouteDialog">
              <el-icon><Plus /></el-icon>
              添加路由
            </el-button>
          </div>
        </div>
      </template>

      <PolicyContextBanner
        v-if="hasRouteContext"
        class="routing-context-banner"
        :domain="'Route'"
        :node-id="contextNodeId"
        :node-name="contextNodeName"
        :policy-ref="routeContext.policyRef"
        :command-id="routeContext.commandId"
        @clear="clearRouteContext"
        @open-node-detail="openContextNodeDetail"
        @open-policy-center="openPolicyCenterContext"
      />

      <el-table
        :data="paginatedRoutes"
        :row-class-name="routeRowClassName"
        stripe
        style="width: 100%"
        v-loading="loading"
      >
        <el-table-column prop="nodeName" label="节点名称" width="180" />
        <el-table-column prop="publicIp" label="公网IP" width="160" />
        <el-table-column prop="region" label="区域" width="120" />
        <el-table-column prop="cidr" label="路由网段" min-width="220" />
        <el-table-column label="下发状态" width="120">
          <template #default="{ row }">
            <el-tag size="small" :type="getPolicyTagType(row.policyStatus)">
              {{ formatPolicyStatus(row.policyStatus, t) }}
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
        <el-table-column prop="lastCommandError" label="失败原因" min-width="220" show-overflow-tooltip />
        <el-table-column v-if="hasPermission('routes:write')" label="操作" width="180" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="primary" @click="showEditRouteDialog(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="showDeleteRouteDialog(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination-container">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :page-sizes="[10, 20, 50, 100]"
          :total="filteredRoutes.length"
          layout="sizes, prev, pager, next, jumper, ->, total"
          @size-change="handleSizeChange"
          @current-change="handleCurrentChange"
        />
      </div>
    </el-card>

    <el-dialog
      v-model="routeDialogVisible"
      :title="dialogMode === 'add' ? '添加路由' : '编辑路由'"
      width="520px"
      :before-close="closeRouteDialog"
    >
      <el-form :model="currentRoute" label-width="100px">
        <el-form-item label="目标节点">
          <el-select v-model="currentRoute.nodeId" placeholder="请选择节点" style="width: 100%" :disabled="dialogMode === 'edit'">
            <el-option
              v-for="node in tenantNodes"
              :key="node.id"
              :label="`${node.hostname || node.public_key || node.id} (${node.region || 'unknown'})`"
              :value="node.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="路由网段">
          <el-input
            v-model="currentRoute.cidr"
            placeholder="请输入 CIDR 格式，如 192.168.1.0/24"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="closeRouteDialog">取消</el-button>
          <el-button v-if="hasPermission('routes:write')" type="primary" @click="confirmRouteAction">
            {{ dialogMode === 'add' ? '添加' : '更新' }}
          </el-button>
        </span>
      </template>
    </el-dialog>

    <el-dialog
      v-model="deleteDialogVisible"
      title="删除路由"
      width="420px"
    >
      <p>
        确定要从节点
        <strong>{{ currentDeleteRoute?.nodeName }}</strong>
        删除路由
        <strong>{{ currentDeleteRoute?.cidr }}</strong>
        吗？
      </p>
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="deleteDialogVisible = false">取消</el-button>
          <el-button v-if="hasPermission('routes:write')" type="primary" @click="confirmDeleteRoute">确定</el-button>
        </span>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Refresh, Plus } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import PolicyContextBanner from '@/components/policy/PolicyContextBanner.vue'
import { useRouteApi } from '@/composables/useRouteApi'
import { useTenantApi } from '@/composables/useTenantApi'
import { usePermission } from '@/composables/usePermission'
import { useTenantChangeReload } from '@/composables/useTenantChangeReload'
import { t } from '@/i18n'
import {
  policyStatusLabel as formatPolicyStatus,
  policyStatusTagType as getPolicyTagType
} from '@/utils/controlLoopStatus'

const { hasPermission } = usePermission()
const route = useRoute() || { query: {}, fullPath: '' }
const router = useRouter()

const loading = ref(false)
const searchQuery = ref('')
const currentPage = ref(1)
const pageSize = ref(10)

const allRoutes = ref([])
const tenantNodes = ref([])
const routeDialogVisible = ref(false)
const deleteDialogVisible = ref(false)
const dialogMode = ref('add')

const currentRoute = ref({
  nodeId: '',
  cidr: '',
  originalCidr: ''
})

const currentDeleteRoute = ref(null)

const routeQuery = computed(() => route.query || {})
const queryString = (...keys) => {
  for (const key of keys) {
    const value = routeQuery.value[key]
    if (typeof value === 'string' && value.trim()) {
      return value
    }
  }
  return ''
}
const routeContext = computed(() => ({
  nodeId: queryString('nodeId', 'node_id'),
  policyRef: queryString('policyRef', 'policy_ref'),
  commandId: queryString('commandId', 'command_id')
}))
const hasRouteContext = computed(() => Boolean(
  routeContext.value.nodeId || routeContext.value.policyRef || routeContext.value.commandId
))
const contextNodeId = computed(() => routeContext.value.nodeId || currentRoute.value.nodeId)
const contextNodeName = computed(() => {
  const node = tenantNodes.value.find((item) => item.id === contextNodeId.value)
  return node?.hostname || node?.public_key || node?.id || contextNodeId.value
})

const clearRouteContext = () => {
  router.push({ name: 'Routing' })
}

const openContextNodeDetail = () => {
  if (!contextNodeId.value) {
    return
  }
  router.push({
    name: 'NodeMonitorDetail',
    params: { nodeId: contextNodeId.value },
    query: {
      ...(routeContext.value.commandId ? { commandId: routeContext.value.commandId } : {}),
      ...(routeContext.value.policyRef ? { policyRef: routeContext.value.policyRef } : {}),
      policyDomain: 'route'
    }
  })
}

const openPolicyCenterContext = () => {
  router.push({
    name: 'Policies',
    query: {
      ...(contextNodeId.value ? { nodeId: contextNodeId.value } : {}),
      ...(routeContext.value.commandId ? { commandId: routeContext.value.commandId } : {}),
      ...(routeContext.value.policyRef ? { policyRef: routeContext.value.policyRef } : {}),
      kind: 'route'
    }
  })
}

const normalizeContextValue = (value) => String(value || '').trim().toLowerCase()

const routeMatchesPolicyContext = (item = {}) => {
  const policyRef = normalizeContextValue(routeContext.value.policyRef)
  const commandId = normalizeContextValue(routeContext.value.commandId)
  if (!policyRef && !commandId) {
    return true
  }

  const policyHaystack = [
    item.policyRef,
    item.policy_ref,
    item.id,
    item.cidr
  ].map(normalizeContextValue).filter(Boolean)
  const commandHaystack = [
    item.lastDeliveryCommandId,
    item.last_delivery_command_id,
    item.lastDelivery?.command_id,
    item.last_delivery?.command_id
  ].map(normalizeContextValue).filter(Boolean)

  if (policyRef) {
    return policyHaystack.includes(policyRef)
  }
  return commandId ? commandHaystack.includes(commandId) : true
}

const filteredRoutes = computed(() => {
  const keyword = searchQuery.value.toLowerCase()
  return allRoutes.value.filter((item) => {
    if (routeContext.value.nodeId && item.nodeId !== routeContext.value.nodeId) {
      return false
    }
    if (!routeMatchesPolicyContext(item)) {
      return false
    }
    if (!keyword) {
      return true
    }
    return String(item.nodeName || '').toLowerCase().includes(keyword) ||
      String(item.publicIp || '').toLowerCase().includes(keyword) ||
      String(item.region || '').toLowerCase().includes(keyword) ||
      String(item.cidr || '').toLowerCase().includes(keyword)
  })
})

const paginatedRoutes = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  return filteredRoutes.value.slice(start, start + pageSize.value)
})

const loadRoutes = async () => {
  loading.value = true
  try {
    const [routes, nodes] = await Promise.all([
      useRouteApi.getRoutes(),
      useTenantApi.getTenantNodes()
    ])
    allRoutes.value = routes
    tenantNodes.value = nodes
    const contextNode = nodes.find((node) => node.id === routeContext.value.nodeId)
    if (contextNode) {
      currentRoute.value.nodeId = contextNode.id
    } else if (!currentRoute.value.nodeId && nodes.length > 0) {
      currentRoute.value.nodeId = nodes[0].id
    }
  } catch (error) {
    console.error('[Routing] 加载路由失败:', error)
    ElMessage.error('加载路由失败: ' + (error.message || '未知错误'))
    allRoutes.value = []
  } finally {
    loading.value = false
  }
}

const showAddRouteDialog = () => {
  dialogMode.value = 'add'
  currentRoute.value = {
    nodeId: currentRoute.value.nodeId || tenantNodes.value[0]?.id || '',
    cidr: '',
    originalCidr: ''
  }
  routeDialogVisible.value = true
}

const showEditRouteDialog = (route) => {
  dialogMode.value = 'edit'
  currentRoute.value = {
    nodeId: route.nodeId,
    cidr: route.cidr,
    originalCidr: route.cidr
  }
  routeDialogVisible.value = true
}

const showDeleteRouteDialog = (route) => {
  currentDeleteRoute.value = route
  deleteDialogVisible.value = true
}

const confirmRouteAction = async () => {
  if (!hasPermission('routes:write')) {
    ElMessage.error('缺少路由管理权限')
    return
  }

  if (!currentRoute.value.nodeId || !currentRoute.value.cidr) {
    ElMessage.error('请填写完整的路由信息')
    return
  }

  const cidrPattern = /^(\d{1,3}\.){3}\d{1,3}\/\d{1,2}$/
  if (!cidrPattern.test(currentRoute.value.cidr)) {
    ElMessage.error('请输入有效的 CIDR 格式，如 192.168.1.0/24')
    return
  }

  try {
    let result
    if (dialogMode.value === 'add') {
      result = await useRouteApi.addRoute(currentRoute.value.nodeId, currentRoute.value.cidr)
      ElMessage.success('路由添加成功')
    } else {
      result = await useRouteApi.updateRoute(
        currentRoute.value.nodeId,
        currentRoute.value.originalCidr,
        currentRoute.value.cidr
      )
      ElMessage.success('路由更新成功')
    }

    const traceRoute = { ...currentRoute.value }
    await loadRoutes()
    closeRouteDialog()
    openCommandTrace(traceRoute, result)
  } catch (error) {
    console.error('[Routing] 路由操作失败:', error)
    const errorMsg = error.response?.data?.message || error.message || '路由操作失败'
    ElMessage.error(errorMsg)
  }
}

const confirmDeleteRoute = async () => {
  if (!hasPermission('routes:write')) {
    ElMessage.error('缺少路由管理权限')
    return
  }

  try {
    const routeToDelete = currentDeleteRoute.value
    const result = await useRouteApi.deleteRoute(routeToDelete.nodeId, routeToDelete.id)
    ElMessage.success('路由删除成功')
    await loadRoutes()
    deleteDialogVisible.value = false
    currentDeleteRoute.value = null
    openCommandTrace(routeToDelete, result)
  } catch (error) {
    console.error('[Routing] 删除路由失败:', error)
    const errorMsg = error.response?.data?.message || error.message || '删除路由失败'
    ElMessage.error(errorMsg)
  }
}

const closeRouteDialog = () => {
  routeDialogVisible.value = false
  currentRoute.value = {
    nodeId: currentRoute.value.nodeId || tenantNodes.value[0]?.id || '',
    cidr: '',
    originalCidr: ''
  }
}

const handleSizeChange = (size) => {
  pageSize.value = size
  currentPage.value = 1
}

const handleCurrentChange = (page) => {
  currentPage.value = page
}

const reloadTenantScopedData = async () => {
  currentPage.value = 1
  allRoutes.value = []
  tenantNodes.value = []
  routeDialogVisible.value = false
  deleteDialogVisible.value = false
  currentDeleteRoute.value = null
  currentRoute.value = {
    nodeId: '',
    cidr: '',
    originalCidr: ''
  }
  await loadRoutes()
}

const shortCommandId = (commandId) => {
  if (!commandId) {
    return '-'
  }
  return commandId.slice(0, 8)
}

const commandIdForMutationResult = (result) => {
  return result?.last_delivery_command_id ||
    result?.lastDeliveryCommandId ||
    result?.last_delivery?.command_id ||
    result?.lastDelivery?.command_id ||
    ''
}

const policyRefForRoute = (row, result) => {
  return result?.policy_ref ||
    result?.policyRef ||
    result?.cidr ||
    row?.policyRef ||
    row?.cidr ||
    row?.id ||
    ''
}

const routeRowClassName = ({ row }) => (
  row && !routeMatchesPolicyContext(row) ? '' : routeContext.value.policyRef || routeContext.value.commandId ? 'context-match-row' : ''
)

const openCommandTrace = (row, result) => {
  const commandId = commandIdForMutationResult(result)
  if (!commandId) {
    return
  }

  const nodeId = result?.node_id || result?.nodeId || row?.nodeId || row?.node_id || currentRoute.value.nodeId
  if (!nodeId) {
    return
  }

  const policyRef = policyRefForRoute(row, result)
  router.push({
    name: 'NodeMonitorDetail',
    params: { nodeId },
    query: {
      commandId,
      focus: 'commands',
      ...(policyRef ? { policyRef: String(policyRef) } : {}),
      policyDomain: 'route'
    }
  })
}

onMounted(() => {
  loadRoutes()
})

useTenantChangeReload(reloadTenantScopedData)

watch(() => route.fullPath || '', async () => {
  currentPage.value = 1
  if (routeContext.value.nodeId) {
    currentRoute.value.nodeId = routeContext.value.nodeId
  }
  await loadRoutes()
})
</script>

<style scoped>
.routing {
  padding: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-actions {
  display: flex;
  gap: 10px;
  align-items: center;
}

.routing-context-banner {
  margin-bottom: 12px;
}

.pagination-container {
  margin-top: 20px;
  display: flex;
  justify-content: flex-end;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

.routing :deep(.context-match-row) {
  background: var(--aria-info-soft, #ecf5ff);
}
</style>
