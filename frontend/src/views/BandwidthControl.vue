<!-- src/views/BandwidthControl.vue -->
<template>
  <div class="policy-rule-page">
    <PageHeader
      :title="t('qos.title')"
      :subtitle="t('qos.subtitle')"
    >
      <template #actions>
        <el-button
          v-if="hasPermission('qos:write')"
          type="primary"
          @click="showAddDialog"
          :disabled="!selectedNodeId"
        >
          <el-icon><Plus /></el-icon>
          {{ t('qos.addRule') }}
        </el-button>
      </template>
    </PageHeader>

    <FilterBar>
      <template #filters>
        <el-select
          v-model="selectedNodeId"
          :placeholder="t('policyTerms.selectNodeRequired')"
          style="width: 240px;"
          @change="onNodeChange"
        >
          <el-option
            v-for="node in tenantNodes"
            :key="node.id"
            :label="node.hostname || node.id"
            :value="node.id"
          />
        </el-select>
      </template>
      <template #actions>
        <el-button @click="refreshData">
          <el-icon><Refresh /></el-icon>
          {{ t('common.refresh') }}
        </el-button>
      </template>
    </FilterBar>

    <el-alert
      :title="t('qos.modelTitle')"
      type="warning"
      :description="t('qos.modelDescription')"
      :closable="false"
      show-icon
    />

    <PolicyContextBanner
      v-if="hasRouteContext"
      :domain="'QoS'"
      :node-id="contextNodeId"
      :node-name="contextNodeName"
      :policy-ref="routeContext.policyRef"
      :command-id="routeContext.commandId"
      @clear="clearRouteContext"
      @open-node-detail="openContextNodeDetail"
      @open-policy-center="openPolicyCenterContext"
    />

    <DataPanel :title="t('qos.rules')" :subtitle="qosPanelSubtitle">
      <el-table
        :data="visibleRules"
        :row-class-name="ruleRowClassName"
        stripe
        v-loading="loading"
        :empty-text="qosEmptyText"
      >
        <el-table-column prop="description" :label="t('common.description')" min-width="160" />
        <el-table-column :label="t('qos.group')" min-width="180">
          <template #default="{ row }">
            <code>{{ formatGroupRef(row.group_id, row.runtime_group || row.group_cidr) }}</code>
          </template>
        </el-table-column>
        <el-table-column :label="t('qos.bandwidthLimit')" width="150">
          <template #default="{ row }">
            <el-tag type="danger" effect="plain">{{ row.bandwidth_mbps }} Mbps</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('qos.directionMode')" width="150">
          <template #default="{ row }">
            <div class="qos-runtime-cell">
              <el-tag size="small" effect="plain">{{ formatDirection(row.direction) }}</el-tag>
              <span>{{ formatMode(row.mode) }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column :label="t('policyTerms.runtime')" width="170">
          <template #default="{ row }">
            <div class="qos-runtime-cell">
              <span>{{ formatRate(row.rate_bps) }}</span>
              <small>{{ t('qos.burstBytes') }} {{ formatBytes(row.burst_bytes) }}</small>
            </div>
          </template>
        </el-table-column>
        <el-table-column :label="t('policyTerms.stats')" width="180">
          <template #default="{ row }">
            <div v-if="row.stats?.load_error" class="qos-runtime-cell">
              <span class="stats-error">{{ t('qos.statsReadFailed') }}</span>
              <small>{{ row.stats.load_error }}</small>
            </div>
            <div v-else class="qos-runtime-cell">
              <span>{{ t('policyTerms.pass') }} {{ formatBytes(row.stats?.passed_bytes) }}</span>
              <small>{{ t('policyTerms.drop') }} {{ formatBytes(row.stats?.dropped_bytes) }}</small>
              <small>{{ t('policyTerms.shape') }} {{ formatBytes(row.stats?.shaped_bytes) }}</small>
            </div>
          </template>
        </el-table-column>
        <el-table-column :label="t('policyTerms.deliveryStatus')" width="120">
          <template #default="{ row }">
            <el-tag size="small" :type="getPolicyTagType(row.policyStatus)">{{ formatPolicyStatus(row.policyStatus, t) }}</el-tag>
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
        <el-table-column v-if="hasPermission('qos:write')" :label="t('common.actions')" width="210" fixed="right">
          <template #default="{ row }">
            <div class="table-actions">
              <ActionIconButton
                v-if="canRetryPolicy(row)"
                :label="t('policyTerms.retry')"
                tone="primary"
                @click="handleRetry(row)"
              >
                <el-icon><Refresh /></el-icon>
              </ActionIconButton>
              <ActionIconButton :label="t('common.edit')" tone="primary" @click="handleEdit(row)">
                <el-icon><Edit /></el-icon>
              </ActionIconButton>
              <ActionIconButton :label="t('common.delete')" tone="danger" @click="handleDelete(row)">
                <el-icon><Delete /></el-icon>
              </ActionIconButton>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </DataPanel>

    <el-dialog
      v-model="dialogVisible"
      :title="dialogTitle"
      width="520px"
      @closed="resetForm"
    >
      <el-form :model="form" label-width="120px" ref="formRef" :rules="formRules">
        <el-form-item :label="t('common.description')" prop="description">
          <el-input v-model="form.description" :placeholder="t('qos.descriptionPlaceholder')" />
        </el-form-item>

        <el-form-item :label="t('qos.ipGroup')">
          <el-select v-model="form.group_id" clearable filterable :placeholder="t('qos.groupPlaceholder')" style="width: 100%">
            <el-option
              v-for="group in selectableGroups"
              :key="group.id"
              :label="formatGroupOption(group)"
              :value="group.id"
            />
          </el-select>
        </el-form-item>

        <el-form-item :label="t('qos.cidrShortcut')" prop="group_cidr">
          <el-input v-model="form.group_cidr" :disabled="Boolean(form.group_id)" :placeholder="t('qos.cidrPlaceholder')" />
          <div class="form-help">{{ t('qos.inlineGroupHelp') }}</div>
        </el-form-item>

        <el-form-item :label="t('qos.bandwidthLimit')" prop="bandwidth_mbps">
          <el-input-number v-model="form.bandwidth_mbps" :min="1" :max="10000" style="width: 100%" />
          <span class="unit-text">Mbps</span>
        </el-form-item>

        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item :label="t('policyTerms.direction')" prop="direction">
              <el-select v-model="form.direction" style="width: 100%">
                <el-option :label="`${t('policyTerms.ingress')} ingress`" value="ingress" />
                <el-option :label="`${t('policyTerms.egress')} egress`" value="egress" />
                <el-option :label="`${t('policyTerms.both')} both`" value="both" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item :label="t('qos.mode')" prop="mode">
              <el-select v-model="form.mode" style="width: 100%">
                <el-option :label="t('qos.modeAuto')" value="auto" />
                <el-option :label="t('qos.modeShaping')" value="shaping" />
                <el-option :label="t('qos.modePolicing')" value="policing" />
              </el-select>
              <div class="form-help">{{ t('qos.modeHelp') }}</div>
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item :label="t('qos.rateBps')">
              <el-input-number v-model="form.rate_bps" :min="0" :step="1000000" style="width: 100%" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item :label="t('qos.burstBytes')">
              <el-input-number v-model="form.burst_bytes" :min="0" :step="1500" style="width: 100%" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-form-item :label="t('policyTerms.priority')" prop="priority">
          <el-input-number v-model="form.priority" :min="0" :max="255" style="width: 100%" />
          <div class="form-help">{{ t('qos.priorityHelp') }}</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button v-if="hasPermission('qos:write')" type="primary" @click="handleSave" :loading="submitting">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, reactive, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Delete, Edit, Plus, Refresh } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import ActionIconButton from '@/components/ui/ActionIconButton.vue'
import DataPanel from '@/components/ui/DataPanel.vue'
import FilterBar from '@/components/ui/FilterBar.vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import PolicyContextBanner from '@/components/policy/PolicyContextBanner.vue'
import { useQosApi } from '@/composables/useQosApi'
import { useIpGroupApi } from '@/composables/useIpGroupApi'
import { useTenantApi } from '@/composables/useTenantApi'
import { usePermission } from '@/composables/usePermission'
import { useTenantChangeReload } from '@/composables/useTenantChangeReload'
import { t } from '@/i18n'
import {
  isRetryablePolicyStatus,
  policyStatusLabel as formatPolicyStatus,
  policyStatusTagType as getPolicyTagType
} from '@/utils/controlLoopStatus'

const { hasPermission } = usePermission()
const route = useRoute() || { query: {}, fullPath: '' }
const router = useRouter()

type RuleId = string | number
type FormRef = {
  validate: () => Promise<void>
  resetFields: () => void
}

interface TenantNodeOption {
  id: string
  hostname?: string
}

interface IPGroupOption {
  id: string
  kind?: string
  name: string
  members?: Array<{ cidr?: string }>
}

interface QoSRuleRow extends Record<string, any> {
  id?: RuleId
  node_id?: string
}

interface QoSFormState extends Record<string, any> {
  id: RuleId | null
  description: string
  group_id: string
  group_cidr: string
  bandwidth_mbps: number
  direction: string
  rate_bps: number
  burst_bytes: number
  priority: number
  mode: string
  enabled: boolean
}

interface EditingOriginal {
  bandwidth_mbps: number
  rate_bps: number
  burst_bytes: number
}

const errorMessage = (error: unknown, fallback = t('policyTerms.unknownError')): string =>
  error instanceof Error ? error.message : (typeof error === 'string' ? error : fallback)

const loading = ref(false)
const submitting = ref(false)
const dialogVisible = ref(false)
const selectedNodeId = ref('')
const tenantNodes = ref<TenantNodeOption[]>([])
const ipGroups = ref<IPGroupOption[]>([])
const rules = ref<QoSRuleRow[]>([])
const formRef = ref<FormRef | null>(null)
const editingOriginal = ref<EditingOriginal | null>(null)
const selectedNodeName = computed(() => {
  const node = tenantNodes.value.find(item => item.id === selectedNodeId.value)
  return node?.hostname || node?.id || ''
})
const contextNodeId = computed(() => routeContext.value.nodeId || selectedNodeId.value)
const contextNodeName = computed(() => {
  const node = tenantNodes.value.find(item => item.id === contextNodeId.value)
  return node?.hostname || node?.id || contextNodeId.value
})
const dialogTitle = computed(() => form.id ? t('qos.editTitle') : t('qos.createTitle'))
const qosEmptyText = computed(() => {
  if (!selectedNodeId.value) return t('policyTerms.selectNodeFirst')
  if (routeContext.value.policyRef && visibleRules.value.length === 0) {
    return t('qos.emptyForContext')
  }
  return t('qos.emptyForNode').replace('{node}', selectedNodeName.value || t('policyTerms.currentNode'))
})
const qosPanelSubtitle = computed(() => {
  if (!selectedNodeId.value) {
    return t('qos.panelSelectNode')
  }
  return `${selectedNodeName.value || t('policyTerms.currentNode')} · ${t('policyTerms.rulesCount').replace('{count}', String(visibleRules.value.length))}`
})
const routeQuery = computed(() => route.query || {})
const queryString = (...keys: string[]) => {
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

const clearRouteContext = () => {
  router.push({ name: 'BandwidthControl' })
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
      policyDomain: 'qos'
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
      kind: 'qos'
    }
  })
}

const form = reactive<QoSFormState>({
  id: null,
  description: '',
  group_id: '',
  group_cidr: '',
  bandwidth_mbps: 100,
  direction: 'egress',
  rate_bps: 0,
  burst_bytes: 0,
  priority: 100,
  mode: 'auto',
  enabled: true
})

const formRules = {
  description: [{ required: true, message: t('qos.descriptionRequired'), trigger: 'blur' }],
  bandwidth_mbps: [{ required: true, message: t('qos.bandwidthRequired'), trigger: 'blur' }]
}

const selectableGroups = computed<IPGroupOption[]>(() => ipGroups.value.filter((group) => group.kind !== 'inline'))

const groupById = computed(() => {
  const result = new Map<string, IPGroupOption>()
  ipGroups.value.forEach((group) => result.set(group.id, group))
  return result
})

const normalizeContextValue = (value: unknown) => String(value || '').trim().toLowerCase()

const ruleMatchesPolicyContext = (row?: QoSRuleRow) => {
  const policyRef = normalizeContextValue(routeContext.value.policyRef)
  const commandId = normalizeContextValue(routeContext.value.commandId)
  if (!policyRef && !commandId) {
    return true
  }

  const policyHaystack = [
    row?.policy_ref,
    row?.id,
    row?.description,
    row?.group_id,
    row?.group_cidr,
    row?.runtime_group
  ].map(normalizeContextValue).filter(Boolean)
  const commandHaystack = [
    row?.last_delivery_command_id,
    row?.lastDeliveryCommandId,
    row?.last_delivery?.command_id,
    row?.lastDelivery?.command_id
  ].map(normalizeContextValue).filter(Boolean)

  if (policyRef) {
    return policyHaystack.includes(policyRef)
  }
  return commandId ? commandHaystack.includes(commandId) : true
}

const visibleRules = computed(() => rules.value.filter(ruleMatchesPolicyContext))

const loadNodes = async () => {
  try {
    tenantNodes.value = await useTenantApi.getTenantNodes()
    if (tenantNodes.value.length > 0) {
      const contextNode = tenantNodes.value.find((node) => node.id === routeContext.value.nodeId)
      selectedNodeId.value = contextNode?.id || selectedNodeId.value || tenantNodes.value[0].id
      refreshData()
    } else {
      selectedNodeId.value = ''
      rules.value = []
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

const refreshData = async () => {
  if (!selectedNodeId.value) return

  loading.value = true
  try {
    rules.value = await useQosApi.getQoSRulesByNode(selectedNodeId.value)
  } catch (error) {
    ElMessage.error(t('qos.loadRulesFailed'))
  } finally {
    loading.value = false
  }
}

const onNodeChange = () => {
  refreshData()
}

const formatDirection = (direction?: string) => {
  const map: Record<string, string> = { ingress: t('policyTerms.ingress'), egress: t('policyTerms.egress'), both: t('policyTerms.both') }
  return map[direction || ''] || direction || t('policyTerms.egress')
}

const formatMode = (mode?: string) => {
  const map: Record<string, string> = { auto: 'Auto', policing: 'Policing', shaping: 'Shaping' }
  return map[mode || ''] || mode || 'Auto'
}

const formatRate = (rateBps: unknown) => {
  const value = Number(rateBps || 0)
  if (value >= 1000000000) return `${(value / 1000000000).toFixed(2)} Gbps`
  if (value >= 1000000) return `${(value / 1000000).toFixed(1)} Mbps`
  if (value >= 1000) return `${(value / 1000).toFixed(1)} Kbps`
  return `${value} bps`
}

const formatBytes = (bytes: unknown) => {
  const value = Number(bytes || 0)
  if (value >= 1048576) return `${(value / 1048576).toFixed(1)} MB`
  if (value >= 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${value} B`
}

const shortCommandId = (commandId?: string) => {
  if (!commandId) return '-'
  return String(commandId).slice(0, 8)
}

const ruleRowClassName = ({ row }: { row?: QoSRuleRow }) => (
  row && !ruleMatchesPolicyContext(row) ? '' : routeContext.value.policyRef || routeContext.value.commandId ? 'context-match-row' : ''
)

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

const showAddDialog = () => {
  resetForm()
  dialogVisible.value = true
}

const resetForm = () => {
  Object.assign(form, {
    id: null,
    description: '',
    group_id: '',
    group_cidr: '',
    bandwidth_mbps: 100,
    direction: 'egress',
    rate_bps: 0,
    burst_bytes: 0,
    priority: 100,
    mode: 'auto',
    enabled: true
  })
  editingOriginal.value = null
  if (formRef.value) formRef.value.resetFields()
}

const directGroupCidrForEdit = (row: QoSRuleRow) => {
  if (row.group_id) return ''
  const direction = row.direction || 'egress'
  const cidr = row.group_cidr ||
    (direction === 'ingress' ? row.src_cidr : row.dst_cidr) ||
    row.src_cidr ||
    row.dst_cidr ||
    ''
  return cidr === 'any' ? '' : cidr
}

const handleEdit = (row: QoSRuleRow) => {
  const bandwidthMbps = Number(row.bandwidth_mbps ?? row.bandwidth ?? 100)
  const rateBps = Number(row.rate_bps || 0)
  const burstBytes = Number(row.burst_bytes || 0)

  Object.assign(form, {
    id: row.id,
    description: row.description || '',
    group_id: row.group_id || '',
    group_cidr: directGroupCidrForEdit(row),
    bandwidth_mbps: bandwidthMbps,
    direction: row.direction || 'egress',
    rate_bps: rateBps,
    burst_bytes: burstBytes,
    priority: Number(row.priority ?? 100),
    mode: row.mode || 'auto',
    enabled: row.enabled !== false
  })
  editingOriginal.value = {
    bandwidth_mbps: bandwidthMbps,
    rate_bps: rateBps,
    burst_bytes: burstBytes
  }
  dialogVisible.value = true
}

const buildSavePayload = (): QoSRuleRow => {
  const payload = { ...form } as QoSRuleRow
  const original = editingOriginal.value
  if (
    form.id &&
    original &&
    Number(payload.bandwidth_mbps) !== Number(original.bandwidth_mbps)
  ) {
    if (Number(payload.rate_bps || 0) === Number(original.rate_bps || 0)) {
      payload.rate_bps = 0
    }
    if (Number(payload.burst_bytes || 0) === Number(original.burst_bytes || 0)) {
      payload.burst_bytes = 0
    }
  }
  return payload
}

const handleSave = async () => {
  if (!hasPermission('qos:write')) {
    ElMessage.error(t('qos.missingPermission'))
    return
  }
  if (!formRef.value) return

  try {
    await formRef.value.validate()
    submitting.value = true

    const nodeId = selectedNodeId.value
    const payload = buildSavePayload()
    const ruleId = form.id
    let result
    if (ruleId) {
      result = await useQosApi.updateQoSRule(nodeId, ruleId, payload as Parameters<typeof useQosApi.updateQoSRule>[2])
      ElMessage.success(t('qos.updateQueued'))
    } else {
      result = await useQosApi.createQoSRule(nodeId, payload as Parameters<typeof useQosApi.createQoSRule>[1])
      ElMessage.success(t('qos.createQueued'))
    }
    dialogVisible.value = false
    await refreshData()
    openCommandTrace(nodeId, { ...payload, id: ruleId || (result as QoSRuleRow | undefined)?.id, node_id: nodeId }, result as Record<string, any> | undefined)
  } catch (error) {
    if (error !== false) {
      const action = form.id ? t('qos.updateFailed') : t('qos.createFailed')
      ElMessage.error(`${action}: ${errorMessage(error)}`)
    }
  } finally {
    submitting.value = false
  }
}

const handleDelete = async (row: QoSRuleRow) => {
  if (!hasPermission('qos:write')) {
    ElMessage.error(t('qos.missingPermission'))
    return
  }

  try {
    await ElMessageBox.confirm(t('qos.deleteConfirm'), t('qos.deleteTitle'), {
      type: 'warning',
      confirmButtonText: t('qos.confirmDelete'),
      cancelButtonText: t('common.cancel')
    })

    const nodeId = selectedNodeId.value || row.node_id
    if (!nodeId || !row.id) return
    const result = await useQosApi.deleteQoSRule(nodeId, row.id)
    ElMessage.success(t('qos.deleteQueued'))
    await refreshData()
    openCommandTrace(nodeId, row, result as Record<string, any>)
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error(t('qos.deleteFailed'))
    }
  }
}

const canRetryPolicy = (row?: QoSRuleRow) => {
  return isRetryablePolicyStatus(row?.policyStatus || row?.policy_status)
}

const commandIdForMutationResult = (result?: Record<string, any>) => {
  return result?.last_delivery_command_id ||
    result?.lastDeliveryCommandId ||
    result?.last_delivery?.command_id ||
    result?.lastDelivery?.command_id ||
    ''
}

const policyRefForRule = (row?: QoSRuleRow, result?: Record<string, any>) => {
  return result?.policy_ref ||
    result?.policyRef ||
    row?.policy_ref ||
    row?.id ||
    ''
}

const openCommandTrace = (nodeId: string, row: QoSRuleRow, result?: Record<string, any>) => {
  const commandId = commandIdForMutationResult(result)
  if (!commandId || !nodeId) {
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
      policyDomain: 'qos'
    }
  })
}

const handleRetry = async (row: QoSRuleRow) => {
  if (!hasPermission('qos:write')) {
    ElMessage.error(t('qos.missingPermission'))
    return
  }

  try {
    const nodeId = selectedNodeId.value || row.node_id
    if (!nodeId) return
    const result = await useQosApi.retryQoSPolicySync(nodeId, row)
    ElMessage.success(t('policyTerms.retryQueued'))
    await refreshData()
    openCommandTrace(nodeId, row, result as Record<string, any>)
  } catch (error) {
    ElMessage.error(`${t('policyTerms.retryFailed')}: ${errorMessage(error)}`)
  }
}

const reloadTenantScopedData = async () => {
  selectedNodeId.value = ''
  tenantNodes.value = []
  ipGroups.value = []
  rules.value = []
  await Promise.all([loadIPGroups(), loadNodes()])
}

onMounted(() => {
  reloadTenantScopedData()
})

useTenantChangeReload(reloadTenantScopedData)

watch(() => route.fullPath || '', async () => {
  if (!routeContext.value.nodeId || routeContext.value.nodeId === selectedNodeId.value) {
    return
  }
  const contextNode = tenantNodes.value.find((node) => node.id === routeContext.value.nodeId)
  if (!contextNode) {
    return
  }
  selectedNodeId.value = contextNode.id
  await refreshData()
})
</script>

<style scoped>
.policy-rule-page {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 20px;
}

.table-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.unit-text { margin-left: 10px; color: #606266; }
.form-help { font-size: 12px; color: #909399; margin-top: 5px; }
.qos-runtime-cell { display: flex; flex-direction: column; gap: 4px; line-height: 1.25; }
.qos-runtime-cell small { color: var(--aria-text-muted, #8a93a6); }
.stats-error { color: var(--aria-danger, #f56c6c); }
.policy-rule-page :deep(.context-match-row) {
  background: var(--aria-info-soft, #ecf5ff);
}
code { background: #f4f4f5; padding: 2px 4px; border-radius: 4px; color: #cf9236; font-family: monospace; }
</style>
