<template>
  <div class="roles-page">
    <div class="page-header">
      <h2>{{ t('roles.title') }}</h2>
      <el-button v-if="hasPermission('roles:write')" type="primary" @click="showCreateDialog">
        {{ t('roles.create') }}
      </el-button>
    </div>

    <el-table :data="roles" v-loading="loading" stripe>
      <el-table-column prop="name" :label="t('roles.roleName')" width="180" />
      <el-table-column prop="description" :label="t('roles.description')" min-width="200" />
      <el-table-column :label="t('roles.type')" width="100">
        <template #default="{ row }">
          <el-tag :type="row.is_system ? 'info' : 'success'" size="small">
            {{ row.is_system ? t('common.system') : t('common.custom') }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('roles.permissions')" min-width="400">
        <template #default="{ row }">
          <el-tag
            v-for="perm in row.permissions"
            :key="perm"
            size="small"
            class="perm-tag"
          >
            {{ perm }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('common.actions')" width="160" v-if="hasPermission('roles:write')">
        <template #default="{ row }">
          <el-button
            v-if="!row.is_system"
            type="primary"
            size="small"
            link
            @click="showEditDialog(row)"
          >
            {{ t('common.edit') }}
          </el-button>
          <el-button
            v-if="!row.is_system"
            type="danger"
            size="small"
            link
            @click="handleDelete(row)"
          >
            {{ t('common.delete') }}
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog
      v-model="dialogVisible"
      :title="editingRole ? t('roles.edit') : t('roles.create')"
      width="600px"
    >
      <el-form :model="form" label-width="80px">
        <el-form-item :label="t('common.name')">
          <el-input
            v-model="form.name"
            :disabled="!!editingRole"
            :placeholder="t('roles.inputRoleName')"
          />
        </el-form-item>
        <el-form-item :label="t('common.description')">
          <el-input v-model="form.description" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item :label="t('roles.permissions')">
          <div class="permission-matrix">
            <div
              v-for="group in permissionGroups"
              :key="group.key"
              class="perm-group"
            >
              <div class="perm-group-label">{{ t(group.labelKey) }}</div>
              <el-checkbox
                v-for="perm in group.items"
                :key="perm.value"
                v-model="form.permissionMap[perm.value]"
              >
                {{ t(perm.labelKey) }}
              </el-checkbox>
            </div>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="handleSave" :loading="saving">
          {{ t('common.save') }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '@/composables/useApi'
import { API_ENDPOINTS, getCurrentTenantId } from '@/config/api'
import { usePermission } from '@/composables/usePermission'
import { useTenantChangeReload } from '@/composables/useTenantChangeReload'
import { t } from '@/i18n'

const { hasPermission } = usePermission()

const roles = ref([])
const loading = ref(false)
const dialogVisible = ref(false)
const saving = ref(false)
const editingRole = ref(null)

const permissionGroups = [
  { key: 'nodes', labelKey: 'roles.groups.nodes', items: [
    { value: 'nodes:read', labelKey: 'roles.permissionsRead' },
    { value: 'nodes:write', labelKey: 'roles.permissionsWrite' }
  ]},
  { key: 'routes', labelKey: 'roles.groups.routes', items: [
    { value: 'routes:read', labelKey: 'roles.permissionsRead' },
    { value: 'routes:write', labelKey: 'roles.permissionsWrite' }
  ]},
  { key: 'acl', labelKey: 'roles.groups.acl', items: [
    { value: 'acls:read', labelKey: 'roles.permissionsRead' },
    { value: 'acls:write', labelKey: 'roles.permissionsWrite' }
  ]},
  { key: 'ip-groups', labelKey: 'roles.groups.ipGroups', items: [
    { value: 'ip-groups:read', labelKey: 'roles.permissionsRead' },
    { value: 'ip-groups:write', labelKey: 'roles.permissionsWrite' }
  ]},
  { key: 'qos', labelKey: 'roles.groups.qos', items: [
    { value: 'qos:read', labelKey: 'roles.permissionsRead' },
    { value: 'qos:write', labelKey: 'roles.permissionsWrite' }
  ]},
  { key: 'blacklist', labelKey: 'roles.groups.blacklist', items: [
    { value: 'blacklist:read', labelKey: 'roles.permissionsRead' },
    { value: 'blacklist:write', labelKey: 'roles.permissionsWrite' }
  ]},
  { key: 'monitoring', labelKey: 'roles.groups.monitoring', items: [
    { value: 'monitoring:read', labelKey: 'roles.permissionsRead' }
  ]},
  { key: 'commands', labelKey: 'roles.groups.commands', items: [
    { value: 'commands:write', labelKey: 'roles.permissionSendCommand' }
  ]},
  { key: 'token', labelKey: 'roles.groups.token', items: [
    { value: 'tokens:read', labelKey: 'roles.permissionsRead' },
    { value: 'tokens:write', labelKey: 'roles.permissionsWrite' }
  ]},
  { key: 'users', labelKey: 'roles.groups.users', items: [
    { value: 'users:read', labelKey: 'roles.permissionsRead' },
    { value: 'users:write', labelKey: 'roles.permissionsWrite' }
  ]},
  { key: 'roles', labelKey: 'roles.groups.roles', items: [
    { value: 'roles:read', labelKey: 'roles.permissionsRead' },
    { value: 'roles:write', labelKey: 'roles.permissionsWrite' }
  ]},
  { key: 'ai', labelKey: 'roles.groups.ai', items: [
    { value: 'ai:use', labelKey: 'roles.permissionUse' }
  ]},
  { key: 'settings', labelKey: 'roles.groups.settings', items: [
    { value: 'settings:read', labelKey: 'roles.permissionsRead' },
    { value: 'settings:write', labelKey: 'roles.permissionsWrite' }
  ]}
]

const allPermissions = permissionGroups.flatMap(g => g.items.map(i => i.value))

const form = reactive({
  name: '',
  description: '',
  permissionMap: {}
})

const loadRoles = async () => {
  const tenantId = getCurrentTenantId()
  if (!tenantId) {
    roles.value = []
    ElMessage.warning(t('roles.selectTenantFirst'))
    return
  }

  loading.value = true
  try {
    const response = await api.get(API_ENDPOINTS.TENANT.ROLES(tenantId))
    roles.value = response.data?.data || []
  } catch (error) {
    ElMessage.error(t('roles.loadFailed'))
  } finally {
    loading.value = false
  }
}

const showCreateDialog = () => {
  editingRole.value = null
  form.name = ''
  form.description = ''
  form.permissionMap = {}
  allPermissions.forEach(p => { form.permissionMap[p] = false })
  dialogVisible.value = true
}

const showEditDialog = (role) => {
  editingRole.value = role
  form.name = role.name
  form.description = role.description || ''
  form.permissionMap = {}
  allPermissions.forEach(p => { form.permissionMap[p] = false })
  ;(role.permissions || []).forEach(p => { form.permissionMap[p] = true })
  dialogVisible.value = true
}

const getSelectedPermissions = () => {
  return Object.entries(form.permissionMap)
    .filter(([_, checked]) => checked)
    .map(([perm]) => perm)
}

const handleSave = async () => {
  const tenantId = getCurrentTenantId()
  if (!tenantId) {
    ElMessage.warning(t('roles.selectTenantFirst'))
    return
  }

  const perms = getSelectedPermissions()
  if (perms.length === 0) {
    ElMessage.warning(t('roles.selectPermission'))
    return
  }

  saving.value = true
  try {
    if (editingRole.value) {
      await api.put(API_ENDPOINTS.TENANT.ROLE_DETAIL(tenantId, editingRole.value.id), {
        description: form.description,
        permissions: perms
      })
      ElMessage.success(t('roles.updateSuccess'))
    } else {
      if (!form.name.trim()) {
        ElMessage.warning(t('roles.nameRequired'))
        saving.value = false
        return
      }
      await api.post(API_ENDPOINTS.TENANT.ROLES(tenantId), {
        name: form.name,
        description: form.description,
        permissions: perms
      })
      ElMessage.success(t('roles.createSuccess'))
    }
    dialogVisible.value = false
    loadRoles()
  } catch (error) {
    ElMessage.error(error.response?.data?.message || t('roles.operationFailed'))
  } finally {
    saving.value = false
  }
}

const handleDelete = async (role) => {
  const tenantId = getCurrentTenantId()
  if (!tenantId) {
    ElMessage.warning(t('roles.selectTenantFirst'))
    return
  }

  try {
    await ElMessageBox.confirm(
      t('roles.deleteConfirm').replace('{name}', role.name),
      t('roles.deleteTitle'),
      { type: 'warning' }
    )
    await api.delete(API_ENDPOINTS.TENANT.ROLE_DETAIL(tenantId, role.id))
    ElMessage.success(t('roles.deleteSuccess'))
    loadRoles()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error(error.response?.data?.message || t('roles.deleteFailed'))
    }
  }
}

onMounted(loadRoles)
useTenantChangeReload(loadRoles)
</script>

<style scoped>
.roles-page {
  padding: 20px;
}
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}
.page-header h2 {
  margin: 0;
}
.perm-tag {
  margin: 2px;
}
.permission-matrix {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
}
.perm-group {
  min-width: 140px;
}
.perm-group-label {
  font-weight: 600;
  margin-bottom: 4px;
  color: #606266;
}
</style>
