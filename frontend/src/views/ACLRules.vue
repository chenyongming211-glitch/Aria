<template>
  <div class="policy-rule-page">
    <PageHeader
      :title="t('acl.title')"
      :subtitle="t('acl.subtitle')"
    >
      <template #actions>
        <el-button v-if="hasPermission('acls:write')" type="primary" @click="handleCreate">
          <el-icon><Plus /></el-icon>
          {{ t('acl.newRule') }}
        </el-button>
      </template>
    </PageHeader>

    <FilterBar>
      <template #filters>
        <el-select v-model="filters.node_id" :placeholder="t('policyTerms.selectNodeRequired')" style="width: 220px" @change="handleNodeChange">
          <el-option
            v-for="node in tenantNodes"
            :key="node.id"
            :label="node.hostname || node.public_key || node.id"
            :value="node.id"
          />
        </el-select>

        <el-input
          v-model="filters.name"
          :placeholder="t('acl.searchRuleName')"
          style="width: 200px"
          @input="debouncedSearch"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>

        <el-select v-model="filters.action" :placeholder="t('policyTerms.action')" clearable @change="loadRules">
          <el-option :label="t('policyTerms.allow')" value="allow" />
          <el-option :label="t('policyTerms.deny')" value="deny" />
        </el-select>

        <el-select v-model="filters.enabled" :placeholder="t('common.status')" clearable @change="loadRules">
          <el-option :label="t('policyTerms.enabled')" :value="true" />
          <el-option :label="t('policyTerms.disabled')" :value="false" />
        </el-select>
      </template>
      <template #actions>
        <el-button @click="loadRules">
          <el-icon><Search /></el-icon>
          {{ t('common.search') }}
        </el-button>
      </template>
    </FilterBar>

    <PolicyContextBanner
      v-if="hasRouteContext"
      :domain="'ACL'"
      :node-id="contextNodeId"
      :node-name="contextNodeName"
      :policy-ref="routeContext.policyRef"
      :command-id="routeContext.commandId"
      @clear="clearRouteContext"
      @open-node-detail="openContextNodeDetail"
      @open-policy-center="openPolicyCenterContext"
    />

    <DataPanel :title="t('acl.rules')" :subtitle="aclPanelSubtitle">
      <el-table
        :data="paginatedRules"
        :row-class-name="ruleRowClassName"
        v-loading="loading"
        style="width: 100%"
      >
      <el-table-column prop="node_name" :label="t('policyTerms.node')" width="160" />
      <el-table-column prop="priority" :label="t('policyTerms.priority')" width="80" sortable />
      <el-table-column prop="name" :label="t('common.name')" width="150" />
      <el-table-column :label="t('acl.srcGroupCidr')" min-width="170">
        <template #default="{ row }">
          <code>{{ formatGroupRef(row.src_group_id, row.runtime_src_group || row.src_cidr) }}</code>
        </template>
      </el-table-column>
      <el-table-column :label="t('acl.dstGroupCidr')" min-width="170">
        <template #default="{ row }">
          <code>{{ formatGroupRef(row.dst_group_id, row.runtime_dst_group || row.dst_cidr) }}</code>
        </template>
      </el-table-column>
      <el-table-column :label="t('acl.directionPorts')" min-width="160">
        <template #default="{ row }">
          <div class="acl-runtime-cell">
            <el-tag size="small" effect="plain">{{ formatDirection(row.direction) }}</el-tag>
            <span>{{ row.runtime_ports || row.ports || 'all' }}</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column prop="protocol" :label="t('policyTerms.protocol')" width="80">
        <template #default="{ row }">
          {{ getProtocolName(row.protocol) }}
        </template>
      </el-table-column>
      <el-table-column prop="action" :label="t('policyTerms.action')" width="80">
        <template #default="{ row }">
          <el-tag :type="getActionType(row.action)">
            {{ getActionName(row.action) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="enabled" :label="t('common.status')" width="80">
        <template #default="{ row }">
          <el-switch
            v-model="row.enabled"
            :disabled="!hasPermission('acls:write')"
            @change="handleToggleEnabled(row)"
          />
        </template>
      </el-table-column>
      <el-table-column :label="t('policyTerms.deliveryStatus')" width="120">
        <template #default="{ row }">
          <el-tag size="small" :type="getPolicyTagType(row.policy_status)">
            {{ formatPolicyStatus(row.policy_status, t) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="pending_cmds" :label="t('policyTerms.pendingCommands')" width="90" />
      <el-table-column :label="t('policyTerms.stats')" width="150">
        <template #default="{ row }">
          <div class="acl-runtime-cell">
            <span>{{ formatNumber(row.stats?.packets) }} {{ t('policyTerms.packets') }} / {{ formatBytes(row.stats?.bytes) }}</span>
            <small>{{ t('policyTerms.drop') }} {{ formatNumber(row.stats?.dropped_packets) }} / {{ formatBytes(row.stats?.dropped_bytes) }}</small>
          </div>
        </template>
      </el-table-column>
      <el-table-column :label="t('policyTerms.lastCommand')" width="150">
        <template #default="{ row }">
          <el-tooltip
            v-if="row.last_delivery_command_id"
            :content="row.last_delivery_command_id"
            placement="top"
          >
            <span>{{ shortCommandId(row.last_delivery_command_id) }}</span>
          </el-tooltip>
          <span v-else>-</span>
        </template>
      </el-table-column>
      <el-table-column prop="last_command_error" :label="t('policyTerms.failureReason')" min-width="180" show-overflow-tooltip />
      <el-table-column :label="t('common.actions')" width="210" fixed="right">
        <template #default="{ row }">
          <div class="table-actions">
            <ActionIconButton
              v-if="hasPermission('acls:write') && canRetryPolicy(row)"
              :label="t('policyTerms.retry')"
              tone="primary"
              @click="handleRetry(row)"
            >
              <el-icon><Refresh /></el-icon>
            </ActionIconButton>
            <ActionIconButton v-if="hasPermission('acls:write')" :label="t('common.edit')" tone="primary" @click="handleEdit(row)">
              <el-icon><Edit /></el-icon>
            </ActionIconButton>
            <ActionIconButton v-if="hasPermission('acls:write')" :label="t('common.delete')" tone="danger" @click="handleDelete(row)">
              <el-icon><Delete /></el-icon>
            </ActionIconButton>
          </div>
        </template>
      </el-table-column>
      </el-table>

      <div class="pagination-section">
        <el-pagination
          v-model:current-page="pagination.page"
          v-model:page-size="pagination.pageSize"
          :page-sizes="[10, 20, 50, 100]"
          :total="visibleRules.length"
          layout="total, sizes, prev, pager, next, jumper"
          @size-change="loadRules"
          @current-change="loadRules"
        />
      </div>
    </DataPanel>

    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="600px" @closed="resetForm">
      <el-form ref="formRef" :model="form" :rules="formRules" label-width="120px">
        <el-form-item :label="t('policyTerms.targetNode')" prop="node_id">
          <el-select v-model="form.node_id" :placeholder="t('policyTerms.selectNode')" style="width: 100%">
            <el-option
              v-for="node in tenantNodes"
              :key="node.id"
              :label="node.hostname || node.public_key || node.id"
              :value="node.id"
            />
          </el-select>
        </el-form-item>

        <el-form-item :label="t('acl.ruleName')" prop="name">
          <el-input v-model="form.name" :placeholder="t('acl.nameRequired')" />
        </el-form-item>
        
        <el-form-item :label="t('acl.srcIpGroup')">
          <el-select v-model="form.src_group_id" clearable filterable :placeholder="t('acl.srcGroupPlaceholder')" style="width: 100%">
            <el-option
              v-for="group in selectableGroups"
              :key="group.id"
              :label="formatGroupOption(group)"
              :value="group.id"
            />
          </el-select>
        </el-form-item>

        <el-form-item :label="t('acl.srcCidr')" prop="src_cidr">
          <el-input v-model="form.src_cidr" :disabled="Boolean(form.src_group_id)" :placeholder="t('acl.srcCidrPlaceholder')" />
          <div class="form-help">{{ t('acl.inlineGroupHelp') }}</div>
        </el-form-item>

        <el-form-item :label="t('acl.dstIpGroup')">
          <el-select v-model="form.dst_group_id" clearable filterable :placeholder="t('acl.dstGroupPlaceholder')" style="width: 100%">
            <el-option
              v-for="group in selectableGroups"
              :key="group.id"
              :label="formatGroupOption(group)"
              :value="group.id"
            />
          </el-select>
        </el-form-item>

        <el-form-item :label="t('acl.dstCidr')" prop="dst_cidr">
          <el-input v-model="form.dst_cidr" :disabled="Boolean(form.dst_group_id)" :placeholder="t('acl.dstCidrPlaceholder')" />
          <div class="form-help">{{ t('acl.inlineGroupHelp') }}</div>
        </el-form-item>
        
        <el-form-item :label="t('policyTerms.protocol')" prop="protocol">
          <el-select v-model="form.protocol">
            <el-option :label="t('policyTerms.any')" :value="0" />
            <el-option label="TCP" :value="6" />
            <el-option label="UDP" :value="17" />
            <el-option label="ICMP" :value="1" />
          </el-select>
        </el-form-item>
        
        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item :label="t('acl.dstPort')" prop="dst_port">
              <el-input-number v-model="form.dst_port" :min="0" :max="65535" style="width: 100%" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item :label="t('policyTerms.direction')" prop="direction">
              <el-select v-model="form.direction" style="width: 100%">
                <el-option :label="`${t('policyTerms.ingress')} ingress`" value="ingress" />
                <el-option :label="`${t('policyTerms.egress')} egress`" value="egress" />
                <el-option :label="`${t('policyTerms.both')} both`" value="both" />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>

        <el-form-item :label="t('acl.ports')">
          <el-input v-model="form.ports" :placeholder="t('acl.portsPlaceholder')" />
          <div class="form-help">{{ t('acl.portsHelp') }}</div>
        </el-form-item>
        
        <el-form-item :label="t('policyTerms.action')" prop="action">
          <el-select v-model="form.action">
            <el-option :label="`${t('policyTerms.allow')} (allow)`" value="allow" />
            <el-option :label="`${t('policyTerms.deny')} (deny)`" value="deny" />
          </el-select>
        </el-form-item>
        
        <el-form-item :label="t('policyTerms.priority')" prop="priority">
          <el-input-number v-model="form.priority" :min="1" :max="10000" style="width: 100%" />
          <div class="form-help">{{ t('acl.priorityHelp') }}</div>
        </el-form-item>
        
        <el-form-item :label="t('policyTerms.enabled')">
          <el-switch v-model="form.enabled" />
        </el-form-item>
        
        <el-form-item :label="t('common.description')">
          <el-input v-model="form.description" type="textarea" :rows="3" :placeholder="t('acl.descriptionPlaceholder')" />
        </el-form-item>
      </el-form>
      
      <template #footer>
        <el-button @click="dialogVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button
          v-if="hasPermission('acls:write')"
          type="primary"
          :loading="submitting"
          @click="handleSubmit"
        >
          {{ t('common.confirm') }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, onUnmounted, computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Edit, Plus, Refresh, Search } from '@element-plus/icons-vue'
import ActionIconButton from '@/components/ui/ActionIconButton.vue'
import DataPanel from '@/components/ui/DataPanel.vue'
import FilterBar from '@/components/ui/FilterBar.vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import PolicyContextBanner from '@/components/policy/PolicyContextBanner.vue'
import { useAclApi } from '@/composables/useAclApi'
import { useIpGroupApi } from '@/composables/useIpGroupApi'
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
  isRetryablePolicyStatus,
  policyStatusLabel as formatPolicyStatus,
  policyStatusTagType as getPolicyTagType
} from '@/utils/controlLoopStatus'
import {
  hasPolicyContext,
  nodeDetailRouteFromContext,
  policyCenterRouteFromContext,
  policyContextFromQuery,
  policyRowMatchesContext
} from '@/utils/policyContext'

const { hasPermission } = usePermission()
const route = useRoute() || { query: {}, fullPath: '' }
const router = useRouter()

type RuleId = string | number
type TimerHandle = ReturnType<typeof setTimeout>
type FormRef = {
  validate: () => Promise<void>
  resetFields: () => void
}

interface TenantNodeOption {
  id: string
  hostname?: string
  public_key?: string
}

interface IPGroupOption {
  id: string
  kind?: string
  name: string
  members?: Array<{ cidr?: string }>
}

interface ACLRuleRow extends Record<string, any> {
  id?: RuleId
  node_id?: string
  name?: string
  enabled?: boolean
}

interface ACLFilters {
  node_id?: string
  name: string
  action: string
  enabled?: boolean
}

interface ACLFormState extends Record<string, any> {
  node_id: string
  node_name: string
  id: RuleId | null
  name: string
  src_group_id: string
  dst_group_id: string
  src_cidr: string
  dst_cidr: string
  protocol: number
  dst_port: number
  direction: string
  ports: string
  action: string
  enabled: boolean
  priority: number
  description: string
}

const errorMessage = (error: unknown, fallback = t('policyTerms.unknownError')): string =>
  error instanceof Error ? error.message : (typeof error === 'string' ? error : fallback)

const loading = ref(false)
const rules = ref<ACLRuleRow[]>([])
const tenantNodes = ref<TenantNodeOption[]>([])
const ipGroups = ref<IPGroupOption[]>([])
const dialogVisible = ref(false)
const submitting = ref(false)
const formRef = ref<FormRef | null>(null)

const filters = reactive<ACLFilters>({
  node_id: '',
  name: '',
  action: '',
  enabled: undefined
})

const pagination = reactive({
  page: 1,
  pageSize: 50,
  total: 0
})

const form = reactive<ACLFormState>({
  node_id: '',
  node_name: '',
  id: null,
  name: '',
  src_group_id: '',
  dst_group_id: '',
  src_cidr: '',
  dst_cidr: '',
  protocol: 6,
  dst_port: 0,
  direction: 'ingress',
  ports: '',
  action: 'allow',
  enabled: true,
  priority: 100,
  description: ''
})

const formRules = {
  node_id: [
    { required: true, message: t('acl.nodeRequired'), trigger: 'change' }
  ],
  name: [
    { required: true, message: t('acl.nameRequired'), trigger: 'blur' },
    { min: 1, max: 50, message: t('acl.nameLength'), trigger: 'blur' }
  ],
  src_cidr: [],
  dst_cidr: [],
  dst_port: [
    { required: true, message: t('acl.portRequired'), trigger: 'blur' }
  ],
  protocol: [
    { required: true, message: t('acl.protocolRequired'), trigger: 'change' }
  ],
  action: [
    { required: true, message: t('acl.actionRequired'), trigger: 'change' }
  ],
  priority: [
    { required: true, message: t('acl.priorityRequired'), trigger: 'blur' }
  ]
}

const dialogTitle = computed(() => form.id ? t('acl.editTitle') : t('acl.createTitle'))
const selectedNodeName = computed(() => {
  const node = tenantNodes.value.find((item) => item.id === filters.node_id)
  return node?.hostname || node?.public_key || node?.id || ''
})
const contextNodeId = computed(() => routeContext.value.nodeId || filters.node_id)
const contextNodeName = computed(() => {
  const node = tenantNodes.value.find((item) => item.id === contextNodeId.value)
  return node?.hostname || node?.public_key || node?.id || contextNodeId.value
})
const aclPanelSubtitle = computed(() => {
  if (!filters.node_id) {
    return t('acl.panelSelectNode')
  }
  return `${selectedNodeName.value || t('policyTerms.currentNode')} · ${t('policyTerms.rulesCount').replace('{count}', String(visibleRules.value.length))}`
})

const routeContext = computed(() => ({
  ...policyContextFromQuery(route.query || {})
}))
const hasRouteContext = computed(() => hasPolicyContext(routeContext.value))

const clearRouteContext = () => {
  router.push({ name: 'ACLRules' })
}

const openContextNodeDetail = () => {
  const target = nodeDetailRouteFromContext({ ...routeContext.value, nodeId: contextNodeId.value, policyDomain: 'acl' })
  if (target) router.push(target)
}

const openPolicyCenterContext = () => {
  router.push(policyCenterRouteFromContext({ ...routeContext.value, nodeId: contextNodeId.value, policyDomain: 'acl' }))
}

const selectableGroups = computed<IPGroupOption[]>(() => ipGroups.value.filter((group) => group.kind !== 'inline'))

const groupById = computed(() => {
  const result = new Map<string, IPGroupOption>()
  ipGroups.value.forEach((group) => result.set(group.id, group))
  return result
})

const ruleMatchesPolicyContext = (row?: ACLRuleRow) => {
  return policyRowMatchesContext(row || {}, routeContext.value)
}

const visibleRules = computed(() => rules.value.filter(ruleMatchesPolicyContext))

const activePolicyRefs = computed(() => rules.value
  .filter((row) => isActivePolicyStatus(row))
  .map((row) => ({
    node_id: row.node_id || filters.node_id,
    policy_domain: 'acl',
    policy_ref: policyRefFromRow(row)
  }))
  .filter((item) => item.node_id && item.policy_ref))

const policyStatusKey = (nodeId?: string, domain?: string, policyRef?: string) => (
  `${nodeId || ''}|${domain || ''}|${policyRef || ''}`
)

const refreshFocusedPolicyStatuses = async () => {
  const refs = activePolicyRefs.value
  if (refs.length === 0) return

  const statuses = await usePolicyStatusApi.getPolicyDeliveryStatuses(refs)
  const byKey = new Map(statuses.map((item) => [
    policyStatusKey(item.node_id, item.policy_domain, item.policy_ref),
    item
  ]))
  rules.value.forEach((row) => {
    const status = byKey.get(policyStatusKey(row.node_id || filters.node_id, 'acl', policyRefFromRow(row)))
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

const paginatedRules = computed(() => {
  const start = (pagination.page - 1) * pagination.pageSize
  return visibleRules.value.slice(start, start + pagination.pageSize)
})

const loadNodes = async () => {
  try {
    tenantNodes.value = await useTenantApi.getTenantNodes()
    const contextNode = tenantNodes.value.find((node) => node.id === routeContext.value.nodeId)
    if (contextNode) {
      filters.node_id = contextNode.id
      await loadRules()
    } else if (!filters.node_id && tenantNodes.value.length > 0) {
      filters.node_id = tenantNodes.value[0].id
      await loadRules()
    } else if (tenantNodes.value.length === 0) {
      filters.node_id = ''
      rules.value = []
      pagination.total = 0
    }
  } catch (error) {
    console.error('Failed to load nodes:', error)
  }
}

const loadIPGroups = async () => {
  try {
    ipGroups.value = await useIpGroupApi.listIPGroups()
  } catch (error) {
    console.error('Failed to load IP Groups:', error)
    ipGroups.value = []
  }
}

const loadRules = async () => {
  if (!filters.node_id) {
    rules.value = []
    pagination.total = 0
    return
  }

  loading.value = true
  try {
    const nodeId = filters.node_id
    const f = {
      name: filters.name,
      action: filters.action,
      enabled: filters.enabled
    }
    
    const response = await useAclApi.getACLRulesByNode(nodeId, f)
    rules.value = response
    pagination.total = visibleRules.value.length
    await policyStatusPolling.trigger()
  } catch (error) {
    ElMessage.error(`${t('acl.loadFailed')}: ${errorMessage(error)}`)
  } finally {
    loading.value = false
  }
}

const handleNodeChange = () => {
  pagination.page = 1
  loadRules()
}

let searchTimer: TimerHandle | null = null
const debouncedSearch = () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    pagination.page = 1
    loadRules()
  }, 500)
}

const handleCreate = () => {
  resetForm()
  dialogVisible.value = true
}

const handleEdit = (row: ACLRuleRow) => {
  Object.assign(form, {
    ...row,
    node_id: row.node_id,
    src_group_id: row.src_group_id || '',
    dst_group_id: row.dst_group_id || '',
    src_cidr: row.src_cidr || row.src_net || '',
    dst_cidr: row.dst_cidr || row.dst_net || '',
    dst_port: row.dst_port ?? row.max_port ?? 0,
    direction: row.direction || 'ingress',
    ports: row.ports || ''
  })
  dialogVisible.value = true
}

const handleDelete = async (row: ACLRuleRow) => {
  try {
    await ElMessageBox.confirm(t('acl.deleteConfirm').replace('{name}', row.name || ''), t('acl.deleteTitle'), {
      confirmButtonText: t('common.confirm'),
      cancelButtonText: t('common.cancel'),
      type: 'warning'
    })
    
    if (!row.id) return
    const result = await useAclApi.deleteACLRule(row.id, row.node_id)
    ElMessage.success(t('acl.deleteQueued'))
    await loadRules()
    openCommandTrace(row, result as Record<string, any>)
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error(`${t('acl.deleteFailed')}: ${errorMessage(error)}`)
    }
  }
}

const canRetryPolicy = (row?: ACLRuleRow) => {
  return isRetryablePolicyStatus(row?.policy_status || row?.policyStatus)
}

const commandIdForMutationResult = (result?: Record<string, any>) => {
  return result?.last_delivery_command_id ||
    result?.lastDeliveryCommandId ||
    result?.last_delivery?.command_id ||
    result?.lastDelivery?.command_id ||
    ''
}

const policyRefForRule = (row?: ACLRuleRow, result?: Record<string, any>) => {
  return result?.policy_ref ||
    result?.policyRef ||
    row?.policy_ref ||
    row?.id ||
    ''
}

const openCommandTrace = (row: ACLRuleRow, result?: Record<string, any>) => {
  const commandId = commandIdForMutationResult(result)
  if (!commandId) {
    return
  }

  const nodeId = result?.node_id || result?.nodeId || row?.node_id || filters.node_id
  if (!nodeId) {
    return
  }

  const policyRef = policyRefForRule(row, result)
  router.push({
    name: 'NodeMonitorDetail',
    params: { nodeId },
    query: {
      commandId,
      focus: 'commands',
      ...(policyRef ? { policyRef: String(policyRef) } : {}),
      policyDomain: 'acl'
    }
  })
}

const handleRetry = async (row: ACLRuleRow) => {
  if (!hasPermission('acls:write')) {
    ElMessage.error(t('acl.missingPermission'))
    return
  }

  try {
    const result = await useAclApi.retryACLPolicySync(row)
    ElMessage.success(t('policyTerms.retryQueued'))
    await loadRules()
    openCommandTrace(row, result as Record<string, any>)
  } catch (error) {
    ElMessage.error(`${t('policyTerms.retryFailed')}: ${errorMessage(error)}`)
  }
}

const handleToggleEnabled = async (row: ACLRuleRow) => {
  try {
    if (!row.id) return
    const result = await useAclApi.updateACLRule(row.id, { ...row, enabled: row.enabled, node_id: row.node_id })
    ElMessage.success(row.enabled ? t('acl.enabledSuccess') : t('acl.disabledSuccess'))
    await loadRules()
    openCommandTrace(row, result as Record<string, any>)
  } catch (error) {
    row.enabled = !row.enabled
    ElMessage.error(`${t('acl.operationFailed')}: ${errorMessage(error)}`)
  }
}

const handleSubmit = async () => {
  if (!formRef.value) return
  
  try {
    await formRef.value.validate()
    submitting.value = true
    
    const data = { ...form } as ACLRuleRow
    
    if (data.dst_port < 0 || data.dst_port > 65535) {
      ElMessage.error(t('acl.invalidPort'))
      return
    }
    
    let result
    if (form.id) {
      result = await useAclApi.updateACLRule(form.id, data as Parameters<typeof useAclApi.updateACLRule>[1])
      ElMessage.success(t('acl.updateSuccess'))
    } else {
      result = await useAclApi.createACLRule(data as Parameters<typeof useAclApi.createACLRule>[0])
      ElMessage.success(t('acl.createSuccess'))
    }
    
    dialogVisible.value = false
    await loadRules()
    openCommandTrace(data, result as Record<string, any>)
  } catch (error) {
    if (error !== false) {
      ElMessage.error(`${t('acl.operationFailed')}: ${errorMessage(error)}`)
    }
  } finally {
    submitting.value = false
  }
}

const resetForm = () => {
  Object.assign(form, {
    node_id: filters.node_id || tenantNodes.value[0]?.id || '',
    node_name: '',
    id: null, name: '', src_group_id: '', dst_group_id: '', src_cidr: '', dst_cidr: '',
    protocol: 6, dst_port: 0, direction: 'ingress', ports: '',
    action: 'allow', enabled: true, priority: 100, description: ''
  })
  if (formRef.value) formRef.value.resetFields()
}

const getProtocolName = (protocol: string | number) => {
  const map: Record<string, string> = { 0: 'Any', 6: 'TCP', 17: 'UDP', 1: 'ICMP' }
  return map[String(protocol)] || 'Unknown'
}

const getActionName = (action?: string) => {
  const map: Record<string, string> = { allow: t('policyTerms.allow'), deny: t('policyTerms.deny') }
  return map[action || ''] || action
}

const getActionType = (action?: string) => {
  const map: Record<string, string> = { allow: 'success', deny: 'danger' }
  return map[action || ''] || ''
}

const formatDirection = (direction?: string) => {
  const map: Record<string, string> = { ingress: t('policyTerms.ingress'), egress: t('policyTerms.egress'), both: t('policyTerms.both') }
  return map[direction || ''] || direction || t('policyTerms.ingress')
}

const formatGroupOption = (group: IPGroupOption) => {
  const cidrs = Array.isArray(group.members) ? group.members.map((member) => member.cidr).join(', ') : ''
  return cidrs ? `${group.name} (${cidrs})` : group.name
}

const formatGroupRef = (groupId?: string, fallback?: string) => {
  if (groupId) {
    const group = groupById.value.get(groupId)
    if (group) return group.name
    if (fallback && fallback !== groupId) return fallback
    return t('policyTerms.unknownIpGroup')
  }
  return fallback || 'any'
}

const shortCommandId = (commandId?: string) => {
  if (!commandId) {
    return '-'
  }
  return commandId.slice(0, 8)
}

const ruleRowClassName = ({ row }: { row?: ACLRuleRow }) => (
  row && !ruleMatchesPolicyContext(row) ? '' : routeContext.value.policyRef || routeContext.value.commandId ? 'context-match-row' : ''
)

const formatNumber = (value: unknown) => {
  const number = Number(value || 0)
  if (number >= 1000000) return `${(number / 1000000).toFixed(1)}M`
  if (number >= 1000) return `${(number / 1000).toFixed(1)}K`
  return String(number)
}

const formatBytes = (value: unknown) => {
  const bytes = Number(value || 0)
  if (bytes >= 1073741824) return `${(bytes / 1073741824).toFixed(1)} GB`
  if (bytes >= 1048576) return `${(bytes / 1048576).toFixed(1)} MB`
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${bytes} B`
}

const reloadTenantScopedData = async () => {
  rules.value = []
  tenantNodes.value = []
  ipGroups.value = []
  filters.node_id = ''
  filters.name = ''
  pagination.page = 1
  pagination.total = 0
  await Promise.all([loadIPGroups(), loadNodes()])
}

onMounted(() => {
  reloadTenantScopedData()
})

onUnmounted(() => {
  if (searchTimer) clearTimeout(searchTimer)
})

useTenantChangeReload(reloadTenantScopedData)

watch(() => route.fullPath || '', async () => {
  pagination.page = 1
  if (routeContext.value.nodeId) {
    filters.node_id = routeContext.value.nodeId
  }
  await loadRules()
})
</script>

<style scoped>
.policy-rule-page {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 20px;
}

.pagination-section {
  display: flex;
  justify-content: flex-end;
  padding: 14px 16px;
  border-top: 1px solid var(--aria-border-primary);
}

.table-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.form-help { font-size: 12px; color: #909399; margin-top: 5px; }
.acl-runtime-cell { display: flex; flex-direction: column; gap: 4px; line-height: 1.25; }
.acl-runtime-cell small { color: var(--aria-text-muted, #8a93a6); }
.policy-rule-page :deep(.context-match-row) {
  background: var(--aria-info-soft, #ecf5ff);
}
code { background: #f4f4f5; padding: 2px 4px; border-radius: 4px; color: #cf9236; font-family: monospace; }
</style>
