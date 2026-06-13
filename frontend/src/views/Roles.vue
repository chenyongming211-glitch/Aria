<template>
  <div class="roles-page">
    <div class="page-header">
      <h2>角色管理</h2>
      <el-button v-if="hasPermission('roles:write')" type="primary" @click="showCreateDialog">
        创建角色
      </el-button>
    </div>

    <el-table :data="roles" v-loading="loading" stripe>
      <el-table-column prop="name" label="角色名称" width="180" />
      <el-table-column prop="description" label="描述" min-width="200" />
      <el-table-column label="类型" width="100">
        <template #default="{ row }">
          <el-tag :type="row.is_system ? 'info' : 'success'" size="small">
            {{ row.is_system ? '系统' : '自定义' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="权限" min-width="400">
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
      <el-table-column label="操作" width="160" v-if="hasPermission('roles:write')">
        <template #default="{ row }">
          <el-button
            v-if="!row.is_system"
            type="primary"
            size="small"
            link
            @click="showEditDialog(row)"
          >
            编辑
          </el-button>
          <el-button
            v-if="!row.is_system"
            type="danger"
            size="small"
            link
            @click="handleDelete(row)"
          >
            删除
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog
      v-model="dialogVisible"
      :title="editingRole ? '编辑角色' : '创建角色'"
      width="600px"
    >
      <el-form :model="form" label-width="80px">
        <el-form-item label="名称">
          <el-input
            v-model="form.name"
            :disabled="!!editingRole"
            placeholder="输入角色名称"
          />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.description" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item label="权限">
          <div class="permission-matrix">
            <div
              v-for="group in permissionGroups"
              :key="group.label"
              class="perm-group"
            >
              <div class="perm-group-label">{{ group.label }}</div>
              <el-checkbox
                v-for="perm in group.items"
                :key="perm.value"
                v-model="form.permissionMap[perm.value]"
              >
                {{ perm.label }}
              </el-checkbox>
            </div>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleSave" :loading="saving">
          保存
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

const { hasPermission } = usePermission()

const roles = ref([])
const loading = ref(false)
const dialogVisible = ref(false)
const saving = ref(false)
const editingRole = ref(null)

const permissionGroups = [
  { label: '节点', items: [
    { value: 'nodes:read', label: '查看' },
    { value: 'nodes:write', label: '管理' }
  ]},
  { label: '路由', items: [
    { value: 'routes:read', label: '查看' },
    { value: 'routes:write', label: '管理' }
  ]},
  { label: 'ACL', items: [
    { value: 'acls:read', label: '查看' },
    { value: 'acls:write', label: '管理' }
  ]},
  { label: 'IP Group', items: [
    { value: 'ip-groups:read', label: '查看' },
    { value: 'ip-groups:write', label: '管理' }
  ]},
  { label: 'QoS', items: [
    { value: 'qos:read', label: '查看' },
    { value: 'qos:write', label: '管理' }
  ]},
  { label: '黑名单', items: [
    { value: 'blacklist:read', label: '查看' },
    { value: 'blacklist:write', label: '管理' }
  ]},
  { label: '监控', items: [
    { value: 'monitoring:read', label: '查看' }
  ]},
  { label: '远程命令', items: [
    { value: 'commands:write', label: '发送命令' }
  ]},
  { label: 'Token', items: [
    { value: 'tokens:read', label: '查看' },
    { value: 'tokens:write', label: '管理' }
  ]},
  { label: '用户', items: [
    { value: 'users:read', label: '查看' },
    { value: 'users:write', label: '管理' }
  ]},
  { label: '角色', items: [
    { value: 'roles:read', label: '查看' },
    { value: 'roles:write', label: '管理' }
  ]},
  { label: 'AI', items: [
    { value: 'ai:use', label: '使用' }
  ]},
  { label: '设置', items: [
    { value: 'settings:read', label: '查看' },
    { value: 'settings:write', label: '管理' }
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
    ElMessage.warning('请先选择租户')
    return
  }

  loading.value = true
  try {
    const response = await api.get(API_ENDPOINTS.TENANT.ROLES(tenantId))
    roles.value = response.data?.data || []
  } catch (error) {
    ElMessage.error('加载角色失败')
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
    ElMessage.warning('请先选择租户')
    return
  }

  const perms = getSelectedPermissions()
  if (perms.length === 0) {
    ElMessage.warning('请至少选择一个权限')
    return
  }

  saving.value = true
  try {
    if (editingRole.value) {
      await api.put(API_ENDPOINTS.TENANT.ROLE_DETAIL(tenantId, editingRole.value.id), {
        description: form.description,
        permissions: perms
      })
      ElMessage.success('角色更新成功')
    } else {
      if (!form.name.trim()) {
        ElMessage.warning('请输入角色名称')
        saving.value = false
        return
      }
      await api.post(API_ENDPOINTS.TENANT.ROLES(tenantId), {
        name: form.name,
        description: form.description,
        permissions: perms
      })
      ElMessage.success('角色创建成功')
    }
    dialogVisible.value = false
    loadRoles()
  } catch (error) {
    ElMessage.error(error.response?.data?.message || '操作失败')
  } finally {
    saving.value = false
  }
}

const handleDelete = async (role) => {
  const tenantId = getCurrentTenantId()
  if (!tenantId) {
    ElMessage.warning('请先选择租户')
    return
  }

  try {
    await ElMessageBox.confirm(
      `确定删除角色 "${role.name}" 吗？`,
      '确认删除',
      { type: 'warning' }
    )
    await api.delete(API_ENDPOINTS.TENANT.ROLE_DETAIL(tenantId, role.id))
    ElMessage.success('角色已删除')
    loadRoles()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error(error.response?.data?.message || '删除失败')
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
