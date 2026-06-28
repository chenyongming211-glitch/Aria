<template>
  <div class="ip-groups-container">
    <div class="page-header">
      <div>
        <h2>{{ t('ipGroups.title') }}</h2>
        <p>{{ t('ipGroups.subtitle') }}</p>
      </div>
      <div class="header-actions">
        <el-button @click="loadGroups">
          <el-icon><Refresh /></el-icon>
          {{ t('common.refresh') }}
        </el-button>
        <el-button v-if="hasPermission('ip-groups:write')" type="primary" @click="handleCreate">
          <el-icon><Plus /></el-icon>
          {{ t('ipGroups.newGroup') }}
        </el-button>
      </div>
    </div>

    <PolicyContextBanner
      v-if="hasRouteContext"
      :domain="'IP Group'"
      :node-id="routeContext.nodeId"
      :policy-ref="routeContext.policyRef"
      :command-id="routeContext.commandId"
      @clear="clearRouteContext"
      @open-node-detail="openContextNodeDetail"
      @open-policy-center="openPolicyCenterContext"
    />

    <el-table :data="groups" v-loading="loading" style="width: 100%" :empty-text="t('ipGroups.emptyText')">
      <el-table-column prop="name" :label="t('common.name')" min-width="180">
        <template #default="{ row }">
          <div class="group-name-cell">
            <strong>{{ row.name }}</strong>
            <small>{{ row.id }}</small>
          </div>
        </template>
      </el-table-column>
      <el-table-column :label="t('ipGroups.kind')" width="120">
        <template #default="{ row }">
          <el-tag :type="row.kind === 'inline' ? 'info' : 'primary'" effect="plain">
            {{ formatKind(row.kind) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="description" :label="t('common.description')" min-width="180" show-overflow-tooltip />
      <el-table-column :label="t('ipGroups.cidrMembers')" min-width="260">
        <template #default="{ row }">
          <div class="cidr-list">
            <el-tag
              v-for="member in groupMembers(row)"
              :key="member.id || member.cidr"
              size="small"
              effect="plain"
            >
              {{ member.cidr }}
            </el-tag>
            <span v-if="!groupMembers(row).length">-</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column :label="t('ipGroups.overlapWarnings')" min-width="220">
        <template #default="{ row }">
          <div v-if="groupWarnings(row).length" class="warning-list">
            <el-tag
              v-for="warning in groupWarnings(row)"
              :key="warning.cidr + warning.overlaps_cidr"
              type="warning"
              size="small"
              effect="plain"
            >
              {{ warning.cidr }} ↔ {{ warning.overlaps_cidr }}
            </el-tag>
          </div>
          <span v-else>-</span>
        </template>
      </el-table-column>
      <el-table-column v-if="hasPermission('ip-groups:write')" :label="t('common.actions')" width="160" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" :disabled="row.kind === 'inline'" @click="handleEdit(row)">{{ t('common.edit') }}</el-button>
          <el-button link type="danger" :disabled="row.kind === 'inline'" @click="handleDelete(row)">{{ t('common.delete') }}</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="640px" @closed="resetForm">
      <el-form ref="formRef" :model="form" :rules="formRules" label-width="110px">
        <el-form-item :label="t('common.name')" prop="name">
          <el-input v-model="form.name" :placeholder="t('ipGroups.namePlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('common.description')">
          <el-input v-model="form.description" type="textarea" :rows="2" :placeholder="t('ipGroups.optionalPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('ipGroups.cidrMembers')" prop="members">
          <div class="member-editor">
            <div v-for="(member, index) in form.members" :key="index" class="member-row">
              <el-input v-model="member.cidr" :placeholder="t('ipGroups.cidrPlaceholder')" />
              <el-input v-model="member.note" :placeholder="t('ipGroups.notePlaceholder')" />
              <el-button :icon="Delete" @click="removeMember(index)" />
            </div>
            <el-button @click="addMember">
              <el-icon><Plus /></el-icon>
              {{ t('ipGroups.addCidr') }}
            </el-button>
          </div>
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button
          v-if="hasPermission('ip-groups:write')"
          type="primary"
          :loading="submitting"
          @click="handleSubmit"
        >
          {{ t('common.save') }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Plus, Refresh } from '@element-plus/icons-vue'
import PolicyContextBanner from '@/components/policy/PolicyContextBanner.vue'
import { useIpGroupApi } from '@/composables/useIpGroupApi'
import { usePermission } from '@/composables/usePermission'
import { useTenantChangeReload } from '@/composables/useTenantChangeReload'
import { t } from '@/i18n'

const { hasPermission } = usePermission()
const route = useRoute() || { query: {} }
const router = useRouter()

const loading = ref(false)
const submitting = ref(false)
const dialogVisible = ref(false)
const formRef = ref(null)
const groups = ref([])

const form = reactive({
  id: '',
  name: '',
  description: '',
  kind: 'custom',
  members: [{ cidr: '', note: '' }]
})

const formRules = {
  name: [{ required: true, message: t('ipGroups.nameRequired'), trigger: 'blur' }],
  members: [
    {
      validator: (_rule, value, callback) => {
        const cidrs = Array.isArray(value) ? value.map((member) => String(member.cidr || '').trim()).filter(Boolean) : []
        if (cidrs.length === 0) {
          callback(new Error(t('ipGroups.membersRequired')))
          return
        }
        callback()
      },
      trigger: 'change'
    }
  ]
}

const dialogTitle = computed(() => form.id ? t('ipGroups.editTitle') : t('ipGroups.createTitle'))
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
  policyDomain: queryString('kind', 'policyDomain', 'policy_domain'),
  commandId: queryString('commandId', 'command_id')
}))
const hasRouteContext = computed(() => Boolean(
  routeContext.value.nodeId || routeContext.value.policyRef || routeContext.value.policyDomain || routeContext.value.commandId
))

const contextQuery = computed(() => ({
  ...(routeContext.value.nodeId ? { nodeId: routeContext.value.nodeId } : {}),
  ...(routeContext.value.policyRef ? { policyRef: routeContext.value.policyRef } : {}),
  ...(routeContext.value.policyDomain ? { kind: routeContext.value.policyDomain } : {}),
  ...(routeContext.value.commandId ? { commandId: routeContext.value.commandId } : {})
}))

const clearRouteContext = () => {
  router.push({ name: 'IPGroups' })
}

const openPolicyCenterContext = () => {
  router.push({ name: 'Policies', query: contextQuery.value })
}

const nodeDetailFocus = () => {
  if (routeContext.value.commandId) return 'commands'
  if (routeContext.value.policyRef || routeContext.value.policyDomain) return 'policies'
  return ''
}

const openContextNodeDetail = () => {
  if (!routeContext.value.nodeId) {
    return
  }
  const focus = nodeDetailFocus()
  router.push({
    name: 'NodeMonitorDetail',
    params: { nodeId: routeContext.value.nodeId },
    query: {
      ...(focus ? { focus } : {}),
      ...(routeContext.value.policyRef ? { policyRef: routeContext.value.policyRef } : {}),
      ...(routeContext.value.policyDomain ? { policyDomain: routeContext.value.policyDomain } : {}),
      ...(routeContext.value.commandId ? { commandId: routeContext.value.commandId } : {})
    }
  })
}

const loadGroups = async () => {
  loading.value = true
  try {
    groups.value = await useIpGroupApi.listIPGroups()
  } catch (error) {
    ElMessage.error(`${t('ipGroups.loadFailed')}: ${error.message || t('policyTerms.unknownError')}`)
  } finally {
    loading.value = false
  }
}

const handleCreate = () => {
  resetForm()
  dialogVisible.value = true
}

const handleEdit = (row) => {
  Object.assign(form, {
    id: row.id,
    name: row.name,
    description: row.description || '',
    kind: row.kind || 'custom',
    members: row.members.length ? row.members.map((member) => ({ cidr: member.cidr, note: member.note || '' })) : [{ cidr: '', note: '' }]
  })
  dialogVisible.value = true
}

const handleDelete = async (row) => {
  try {
    await ElMessageBox.confirm(t('ipGroups.deleteConfirm').replace('{name}', row.name), t('ipGroups.deleteTitle'), {
      type: 'warning',
      confirmButtonText: t('common.confirm'),
      cancelButtonText: t('common.cancel')
    })
    await useIpGroupApi.deleteIPGroup(row.id)
    ElMessage.success(t('ipGroups.deleteSuccess'))
    await loadGroups()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error(`${t('ipGroups.deleteFailed')}: ${error.message || t('policyTerms.unknownError')}`)
    }
  }
}

const handleSubmit = async () => {
  if (!formRef.value) return

  try {
    await formRef.value.validate()
    submitting.value = true
    const payload = {
      name: form.name,
      description: form.description,
      kind: 'custom',
      members: form.members
    }
    if (form.id) {
      await useIpGroupApi.updateIPGroup(form.id, payload)
      ElMessage.success(t('ipGroups.updateSuccess'))
    } else {
      await useIpGroupApi.createIPGroup(payload)
      ElMessage.success(t('ipGroups.createSuccess'))
    }
    dialogVisible.value = false
    await loadGroups()
  } catch (error) {
    if (error !== false) {
      ElMessage.error(`${t('ipGroups.saveFailed')}: ${error.message || t('policyTerms.unknownError')}`)
    }
  } finally {
    submitting.value = false
  }
}

const addMember = () => {
  form.members.push({ cidr: '', note: '' })
}

const removeMember = (index) => {
  form.members.splice(index, 1)
  if (form.members.length === 0) {
    addMember()
  }
}

const resetForm = () => {
  Object.assign(form, {
    id: '',
    name: '',
    description: '',
    kind: 'custom',
    members: [{ cidr: '', note: '' }]
  })
  if (formRef.value) formRef.value.resetFields()
}

const formatKind = (kind) => {
  const map = { custom: t('ipGroups.custom'), inline: t('ipGroups.inline'), system: t('ipGroups.system') }
  return map[kind] || kind
}

const groupMembers = (group) => Array.isArray(group?.members) ? group.members : []

const groupWarnings = (group) => Array.isArray(group?.warnings) ? group.warnings : []

onMounted(loadGroups)
useTenantChangeReload(loadGroups)
</script>

<style scoped>
.ip-groups-container { padding: 20px; }
.page-header { display: flex; justify-content: space-between; align-items: flex-start; gap: 20px; margin-bottom: 20px; }
.policy-context-banner { margin-bottom: 14px; }
.page-header h2 { margin: 0 0 6px; font-size: 20px; }
.page-header p { margin: 0; color: var(--aria-text-muted, #8a93a6); }
.header-actions { display: flex; gap: 10px; }
.group-name-cell { display: flex; flex-direction: column; gap: 4px; line-height: 1.25; }
.group-name-cell small { color: var(--aria-text-muted, #8a93a6); font-family: monospace; }
.cidr-list, .warning-list { display: flex; gap: 6px; flex-wrap: wrap; }
.member-editor { display: flex; flex-direction: column; gap: 10px; width: 100%; }
.member-row { display: grid; grid-template-columns: minmax(0, 1.2fr) minmax(0, 1fr) 36px; gap: 8px; align-items: center; }
</style>
