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
      :domain="t('ipGroups.productName')"
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
      <el-table-column :label="t('common.actions')" width="230" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="openReferences(row)">{{ t('ipGroups.viewReferences') }}</el-button>
          <el-button v-if="hasPermission('ip-groups:write')" link type="primary" :disabled="row.kind === 'inline'" @click="handleEdit(row)">{{ t('common.edit') }}</el-button>
          <el-button
            v-if="hasPermission('ip-groups:write')"
            link
            type="danger"
            :disabled="row.kind === 'inline'"
            @click="handleDelete(row)"
          >
            {{ t('common.delete') }}
          </el-button>
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

    <el-drawer
      v-model="referencesDrawerVisible"
      :title="referencesTitle"
      size="560px"
      destroy-on-close
    >
      <el-alert
        v-if="referencesTotal > 0"
        class="references-alert"
        type="info"
        :closable="false"
        :title="t('ipGroups.referencesHint')"
      />
      <el-table
        :data="references"
        v-loading="referencesLoading"
        :empty-text="t('ipGroups.noReferences')"
        size="small"
      >
        <el-table-column :label="t('ipGroups.domain')" width="88">
          <template #default="{ row }">
            <el-tag size="small" effect="plain">{{ formatReferenceDomain(row.domain) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('ipGroups.ruleName')" min-width="180">
          <template #default="{ row }">
            <el-button link type="primary" @click="openReference(row)">
              {{ row.rule_name || row.rule_id }}
            </el-button>
            <div class="reference-meta">{{ row.node_name || row.node_id }}</div>
          </template>
        </el-table-column>
        <el-table-column :label="t('ipGroups.latestDelivery')" min-width="140">
          <template #default="{ row }">
            <div class="reference-delivery">
              <el-tag size="small" :type="deliveryTagType(row.latest_delivery?.status)">
                {{ row.latest_delivery?.status || '-' }}
              </el-tag>
              <small v-if="row.latest_delivery?.last_error">{{ row.latest_delivery.last_error }}</small>
            </div>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="referencesTotal > referencesLimit" class="references-pagination">
        <el-pagination
          layout="prev, pager, next, total"
          :total="referencesTotal"
          :page-size="referencesLimit"
          :current-page="Math.floor(referencesOffset / referencesLimit) + 1"
          @current-change="handleReferencePageChange"
        />
      </div>
    </el-drawer>
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
import {
  hasPolicyContext,
  nodeDetailRouteFromContext,
  policyCenterRouteFromContext,
  policyContextFromQuery
} from '@/utils/policyContext'

const { hasPermission } = usePermission()
const route = useRoute() || { query: {} }
const router = useRouter()

const loading = ref(false)
const submitting = ref(false)
const dialogVisible = ref(false)
const referencesDrawerVisible = ref(false)
const referencesLoading = ref(false)
const formRef = ref(null)
const groups = ref([])
const references = ref([])
const referencesTotal = ref(0)
const referencesOffset = ref(0)
const referencesLimit = 20
const selectedReferenceGroup = ref(null)

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
const referencesTitle = computed(() => {
  const name = selectedReferenceGroup.value?.name || ''
  return name ? `${t('ipGroups.referencesTitle')}: ${name}` : t('ipGroups.referencesTitle')
})
const routeContext = computed(() => policyContextFromQuery(route.query || {}))
const hasRouteContext = computed(() => hasPolicyContext(routeContext.value))

const errorMessage = (error, fallback = t('policyTerms.unknownError')) => {
  if (error instanceof Error) return error.message
  if (typeof error === 'string') return error
  return fallback
}

const clearRouteContext = () => {
  router.push({ name: 'IPGroups' })
}

const openPolicyCenterContext = () => {
  router.push(policyCenterRouteFromContext(routeContext.value))
}

const openContextNodeDetail = () => {
  const target = nodeDetailRouteFromContext(routeContext.value)
  if (target) router.push(target)
}

const loadGroups = async () => {
  loading.value = true
  try {
    groups.value = await useIpGroupApi.listIPGroups()
  } catch (error) {
    ElMessage.error(`${t('ipGroups.loadFailed')}: ${errorMessage(error)}`)
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

const loadReferences = async (row, offset = 0, options = {}) => {
  selectedReferenceGroup.value = row
  referencesLoading.value = true
  try {
    const page = await useIpGroupApi.listIPGroupReferences(row.id, { limit: referencesLimit, offset })
    references.value = page.items
    referencesTotal.value = page.total
    referencesOffset.value = page.offset
    return page
  } catch (error) {
    ElMessage.error(`${t('ipGroups.loadReferencesFailed')}: ${errorMessage(error)}`)
    if (options.failClosed) {
      throw error
    }
    return { items: [], total: 0, limit: referencesLimit, offset, has_more: false }
  } finally {
    referencesLoading.value = false
  }
}

const openReferences = async (row) => {
  referencesDrawerVisible.value = true
  await loadReferences(row, 0)
}

const handleReferencePageChange = async (page) => {
  if (!selectedReferenceGroup.value) return
  await loadReferences(selectedReferenceGroup.value, (page - 1) * referencesLimit)
}

const openReference = (reference) => {
  const routeInfo = reference?.route || {}
  const query = routeInfo.query || {}
  if (routeInfo.name) {
    router.push({ name: routeInfo.name, query })
    return
  }
  if (routeInfo.path) {
    router.push({ path: routeInfo.path, query })
  }
}

const handleDelete = async (row) => {
  try {
    const referencePage = await loadReferences(row, 0, { failClosed: true })
    if (referencePage.total > 0) {
      referencesDrawerVisible.value = true
      ElMessage.warning(t('ipGroups.deleteBlockedByReferences').replace('{count}', String(referencePage.total)))
      return
    }
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
      ElMessage.error(`${t('ipGroups.deleteFailed')}: ${errorMessage(error)}`)
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
      ElMessage.error(`${t('ipGroups.saveFailed')}: ${errorMessage(error)}`)
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

const formatReferenceDomain = (domain) => {
  const map = { acl: t('nav.accessControl'), qos: t('nav.bandwidthControl') }
  return map[domain] || domain
}

const deliveryTagType = (status) => {
  const map = { completed: 'success', failed: 'danger', pending: 'warning', sent: 'warning', acknowledged: 'warning', stale: 'info' }
  return map[status] || 'info'
}

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
.references-alert { margin-bottom: 12px; }
.reference-meta { color: var(--aria-text-muted, #8a93a6); font-size: 12px; margin-top: 3px; }
.reference-delivery { display: flex; flex-direction: column; gap: 4px; }
.reference-delivery small { color: var(--aria-danger, #f56c6c); }
.references-pagination { display: flex; justify-content: flex-end; padding-top: 14px; }
</style>
