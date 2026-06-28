<template>
  <div class="routing">
    <el-card>
      <template #header>
        <div class="card-header">
          <h3>{{ t('routing.title') }}</h3>
          <div class="header-actions">
            <el-input
              v-model="searchQuery"
              :placeholder="t('routing.searchPlaceholder')"
              style="width: 240px"
              clearable
            />
            <el-button type="primary" @click="loadRoutes">
              <el-icon><Refresh /></el-icon>
              {{ t('common.refresh') }}
            </el-button>
            <el-button v-if="hasPermission('routes:write')" type="primary" @click="showAddRouteDialog">
              <el-icon><Plus /></el-icon>
              {{ t('routing.addRoute') }}
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
        <el-table-column prop="nodeName" :label="t('routing.nodeName')" width="180" />
        <el-table-column prop="publicIp" :label="t('policyTerms.publicIp')" width="160" />
        <el-table-column prop="region" :label="t('policyTerms.region')" width="120" />
        <el-table-column prop="cidr" :label="t('routing.cidr')" min-width="220" />
        <el-table-column :label="t('policyTerms.deliveryStatus')" width="120">
          <template #default="{ row }">
            <el-tag size="small" :type="getPolicyTagType(row.policyStatus)">
              {{ formatPolicyStatus(row.policyStatus, t) }}
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
        <el-table-column prop="lastCommandError" :label="t('policyTerms.failureReason')" min-width="220" show-overflow-tooltip />
        <el-table-column v-if="hasPermission('routes:write')" :label="t('common.actions')" width="180" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="primary" @click="showEditRouteDialog(row)">{{ t('common.edit') }}</el-button>
            <el-button size="small" type="danger" @click="showDeleteRouteDialog(row)">{{ t('common.delete') }}</el-button>
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
      :title="dialogMode === 'add' ? t('routing.addRoute') : t('routing.editRoute')"
      width="520px"
      :before-close="closeRouteDialog"
    >
      <el-form :model="currentRoute" label-width="100px">
        <el-form-item :label="t('policyTerms.targetNode')">
          <el-select v-model="currentRoute.nodeId" :placeholder="t('routing.selectNodePlaceholder')" style="width: 100%" :disabled="dialogMode === 'edit'">
            <el-option
              v-for="node in tenantNodes"
              :key="node.id"
              :label="`${node.hostname || node.public_key || node.id} (${node.region || t('status.unknown')})`"
              :value="node.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('routing.cidr')">
          <el-input
            v-model="currentRoute.cidr"
            :placeholder="t('routing.cidrPlaceholder')"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="closeRouteDialog">{{ t('common.cancel') }}</el-button>
          <el-button v-if="hasPermission('routes:write')" type="primary" @click="confirmRouteAction">
            {{ dialogMode === 'add' ? t('common.add') : t('common.update') }}
          </el-button>
        </span>
      </template>
    </el-dialog>

    <el-dialog
      v-model="deleteDialogVisible"
      :title="t('routing.deleteRoute')"
      width="420px"
    >
      <p>
        {{ t('routing.deleteConfirmBefore') }}
        <strong>{{ currentDeleteRoute?.nodeName }}</strong>
        {{ t('routing.deleteConfirmMiddle') }}
        <strong>{{ currentDeleteRoute?.cidr }}</strong>
        {{ t('routing.deleteConfirmAfter') }}
      </p>
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="deleteDialogVisible = false">{{ t('common.cancel') }}</el-button>
          <el-button v-if="hasPermission('routes:write')" type="primary" @click="confirmDeleteRoute">{{ t('common.confirm') }}</el-button>
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
import { useFocusedPolling } from '@/composables/useFocusedPolling'
import {
  isActivePolicyStatus,
  patchPolicyStatusRow,
  policyRefFromRow,
  usePolicyStatusApi
} from '@/composables/usePolicyStatusApi'
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

const nodeDetailFocus = () => {
  if (routeContext.value.commandId) return 'commands'
  if (routeContext.value.policyRef) return 'policies'
  return ''
}

const openContextNodeDetail = () => {
  if (!contextNodeId.value) {
    return
  }
  const focus = nodeDetailFocus()
  router.push({
    name: 'NodeMonitorDetail',
    params: { nodeId: contextNodeId.value },
    query: {
      ...(focus ? { focus } : {}),
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

const activePolicyRefs = computed(() => allRoutes.value
  .filter((row) => isActivePolicyStatus(row))
  .map((row) => ({
    node_id: row.nodeId || row.node_id,
    policy_domain: 'route',
    policy_ref: policyRefFromRow(row)
  }))
  .filter((item) => item.node_id && item.policy_ref))

const policyStatusKey = (nodeId, domain, policyRef) => `${nodeId || ''}|${domain || ''}|${policyRef || ''}`

const refreshFocusedPolicyStatuses = async () => {
  const refs = activePolicyRefs.value
  if (refs.length === 0) return

  const statuses = await usePolicyStatusApi.getPolicyDeliveryStatuses(refs)
  const byKey = new Map(statuses.map((item) => [
    policyStatusKey(item.node_id, item.policy_domain, item.policy_ref),
    item
  ]))
  allRoutes.value.forEach((row) => {
    const status = byKey.get(policyStatusKey(row.nodeId || row.node_id, 'route', policyRefFromRow(row)))
    if (status) {
      patchPolicyStatusRow(row, status)
    }
  })
}

const policyStatusPolling = useFocusedPolling({
  poll: refreshFocusedPolicyStatuses,
  hasActiveItems: () => activePolicyRefs.value.length > 0,
  intervalMs: 3000
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
    await policyStatusPolling.trigger()
    const contextNode = nodes.find((node) => node.id === routeContext.value.nodeId)
    if (contextNode) {
      currentRoute.value.nodeId = contextNode.id
    } else if (!currentRoute.value.nodeId && nodes.length > 0) {
      currentRoute.value.nodeId = nodes[0].id
    }
  } catch (error) {
    console.error('[Routing] Failed to load routes:', error)
    ElMessage.error(`${t('routing.loadFailed')}: ${error.message || t('policyTerms.unknownError')}`)
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
    ElMessage.error(t('routing.missingPermission'))
    return
  }

  if (!currentRoute.value.nodeId || !currentRoute.value.cidr) {
    ElMessage.error(t('routing.incomplete'))
    return
  }

  const cidrPattern = /^(\d{1,3}\.){3}\d{1,3}\/\d{1,2}$/
  if (!cidrPattern.test(currentRoute.value.cidr)) {
    ElMessage.error(t('routing.invalidCidr'))
    return
  }

  try {
    let result
    if (dialogMode.value === 'add') {
      result = await useRouteApi.addRoute(currentRoute.value.nodeId, currentRoute.value.cidr)
      ElMessage.success(t('routing.addSuccess'))
    } else {
      result = await useRouteApi.updateRoute(
        currentRoute.value.nodeId,
        currentRoute.value.originalCidr,
        currentRoute.value.cidr
      )
      ElMessage.success(t('routing.updateSuccess'))
    }

    const traceRoute = { ...currentRoute.value }
    await loadRoutes()
    closeRouteDialog()
    openCommandTrace(traceRoute, result)
  } catch (error) {
    console.error('[Routing] Route operation failed:', error)
    const errorMsg = error.response?.data?.message || error.message || t('routing.operationFailed')
    ElMessage.error(errorMsg)
  }
}

const confirmDeleteRoute = async () => {
  if (!hasPermission('routes:write')) {
    ElMessage.error(t('routing.missingPermission'))
    return
  }

  try {
    const routeToDelete = currentDeleteRoute.value
    const result = await useRouteApi.deleteRoute(routeToDelete.nodeId, routeToDelete.id)
    ElMessage.success(t('routing.deleteSuccess'))
    await loadRoutes()
    deleteDialogVisible.value = false
    currentDeleteRoute.value = null
    openCommandTrace(routeToDelete, result)
  } catch (error) {
    console.error('[Routing] Failed to delete route:', error)
    const errorMsg = error.response?.data?.message || error.message || t('routing.deleteFailed')
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
