<!-- src/views/TenantManagement.vue -->
<template>
  <div class="tenant-management">
    <el-card>
      <template #header>
        <div class="card-header">
          <h3>{{ t('tenantManagement.title') }}</h3>
          <div class="header-actions">
            <el-input
              v-model="searchQuery"
              :placeholder="t('tenantManagement.searchPlaceholder')"
              style="width: 200px; margin-right: 10px;"
              clearable
            >
              <template #prefix>
                <el-icon><Search /></el-icon>
              </template>
            </el-input>
            <el-button type="primary" @click="refreshTenants">
              <el-icon><Refresh /></el-icon>
              {{ t('common.refresh') }}
            </el-button>
            <el-button type="primary" @click="createTenant">
              <el-icon><Plus /></el-icon>
              {{ t('tenantManagement.create') }}
            </el-button>
          </div>
        </div>
      </template>

      <el-table
        :data="paginatedTenants"
        stripe
        style="width: 100%"
        v-loading="loading"
        row-key="id"
        :tree-props="{children: 'children', hasChildren: 'hasChildren'}"
      >
        <el-table-column prop="name" :label="t('tenantManagement.name')" width="200">
          <template #default="{ row }">
            <div class="tenant-name-cell">
              <el-icon v-if="row.icon" :class="row.icon" class="tenant-icon" />
              <span>{{ row.name }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="code" :label="t('tenantManagement.coding')" width="150" />
        <el-table-column prop="status" :label="t('tenantManagement.status')" width="120">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)">
              {{ getStatusText(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="nodeCount" :label="t('tenantManagement.nodeCount')" width="100" />
        <el-table-column prop="tokenCount" :label="t('tenantManagement.tokenCount')" width="100" />
        <el-table-column prop="createdAt" :label="t('tenantManagement.createdAt')" width="180" />
        <el-table-column prop="description" :label="t('tenantManagement.description')" show-overflow-tooltip />
        <el-table-column :label="t('tenantManagement.actions')" width="280">
          <template #default="{ row }">
            <el-button size="small" @click="viewTenantDetails(row)">{{ t('common.view') }}</el-button>
            <el-button size="small" type="primary" @click="editTenant(row)">{{ t('common.edit') }}</el-button>
            <el-button size="small" type="info" @click="manageAccess(row)">{{ t('tenantManagement.manageAccess') }}</el-button>
            <el-popconfirm
              :title="getToggleConfirm(row)"
              @confirm="toggleTenantStatus(row)"
            >
              <template #reference>
                <el-button size="small" :type="row.status === 'active' ? 'warning' : 'success'">
                  {{ row.status === 'active' ? t('tenantManagement.suspend') : t('tenantManagement.activate') }}
                </el-button>
              </template>
            </el-popconfirm>
            <el-popconfirm
              :title="t('tenantManagement.deleteConfirm')"
              @confirm="deleteTenant(row.id)"
            >
              <template #reference>
                <el-button size="small" type="danger">{{ t('common.delete') }}</el-button>
              </template>
            </el-popconfirm>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination-container">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :page-sizes="[10, 20, 50, 100]"
          :total="filteredTenants.length"
          layout="sizes, prev, pager, next, jumper, ->, total"
          @size-change="handleSizeChange"
          @current-change="handleCurrentChange"
        />
      </div>
    </el-card>

    <!-- Tenant Detail Dialog -->
    <el-dialog
      v-model="detailDialogVisible"
      :title="dialogTitle"
      width="50%"
      :before-close="closeDetailDialog"
    >
      <el-form
        v-if="editingTenant"
        :model="editingTenant"
        :rules="tenantRules"
        ref="tenantFormRef"
        label-width="100px"
      >
        <el-form-item :label="t('tenantManagement.name')" prop="name">
          <el-input v-model="editingTenant.name" :disabled="isEditingExisting" :placeholder="t('tenantManagement.namePlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('tenantManagement.coding')" prop="code">
          <el-input v-model="editingTenant.code" :disabled="isEditingExisting" :placeholder="t('tenantManagement.codePlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('tenantManagement.description')" prop="description">
          <el-input
            v-model="editingTenant.description"
            type="textarea"
            :rows="3"
            :placeholder="t('tenantManagement.descriptionPlaceholder')"
          />
        </el-form-item>
      </el-form>

      <template #footer>
        <span class="dialog-footer">
          <el-button @click="closeDetailDialog">{{ t('common.cancel') }}</el-button>
          <el-button type="primary" @click="saveTenant">{{ t('common.confirm') }}</el-button>
        </span>
      </template>
    </el-dialog>

    <!-- Access Management Dialog -->
    <el-dialog
      v-model="accessDialogVisible"
      :title="t('tenantManagement.manageAccessTitle')"
      width="70%"
    >
      <div v-if="selectedTenant">
        <el-tabs v-model="accessTab">
          <el-tab-pane :label="t('tenantManagement.users')" name="users">
            <el-button type="primary" @click="addUserToTenant" style="margin-bottom: 15px;">
              <el-icon><Plus /></el-icon>
              {{ t('tenantManagement.addUser') }}
            </el-button>

            <el-table :data="selectedTenant.users" style="width: 100%">
              <el-table-column prop="username" :label="t('common.username')" width="150" />
              <el-table-column prop="email" :label="t('common.email')" width="200" />
              <el-table-column prop="role" :label="t('common.role')" width="120">
                <template #default="{ row }">
                  <el-tag :type="getRoleType(row.role)">{{ getRoleLabel(row.role) }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column :label="t('common.actions')" width="150">
                <template #default="{ row }">
                  <el-button size="small" type="primary" @click="editUserRole(row)">{{ t('common.edit') }}</el-button>
                  <el-popconfirm
                    :title="t('tenantManagement.deleteUserConfirm')"
                    @confirm="removeUserFromTenant(row.id)"
                  >
                    <template #reference>
                      <el-button size="small" type="danger">{{ t('common.delete') }}</el-button>
                    </template>
                  </el-popconfirm>
                </template>
              </el-table-column>
            </el-table>
          </el-tab-pane>

        </el-tabs>
      </div>

      <template #footer>
        <span class="dialog-footer">
          <el-button @click="accessDialogVisible = false">{{ t('common.close') }}</el-button>
        </span>
      </template>
    </el-dialog>

    <el-dialog
      v-model="roleDialogVisible"
      :title="t('tenantManagement.editUserRole')"
      width="420px"
    >
      <el-form :model="roleForm" label-width="80px">
        <el-form-item :label="t('common.user')">
          <el-input :value="editingUser?.username || ''" disabled />
        </el-form-item>
        <el-form-item :label="t('common.role')">
          <el-select v-model="roleForm.role" style="width: 100%">
            <el-option
              v-for="role in roleOptions"
              :key="role.value"
              :label="role.label"
              :value="role.value"
            />
          </el-select>
        </el-form-item>
      </el-form>

      <template #footer>
        <span class="dialog-footer">
          <el-button @click="roleDialogVisible = false">{{ t('common.cancel') }}</el-button>
          <el-button type="primary" :loading="roleSaving" @click="saveUserRole">{{ t('common.update') }}</el-button>
        </span>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, reactive, onMounted } from 'vue'
import { Search, Refresh, Plus } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '@/composables/useApi'
import { API_ENDPOINTS } from '@/config/api'
import { t } from '@/i18n'

const tenants = ref([])
const loading = ref(false)

onMounted(() => {
  loadTenants()
})

const loadTenants = async () => {
  loading.value = true
  try {
    const response = await api.get(API_ENDPOINTS.TENANT.LIST)
    const tenantList = response.data?.data || response.data || []
    tenants.value = tenantList.map(t => ({
      ...t,
      status: t.status || 'active',
      nodeCount: 0,
      tokenCount: 0,
      createdAt: t.created_at || new Date().toISOString(),
      description: t.description || '',
      users: []
    }))
  } catch (error) {
    console.error('Failed to load tenants:', error)
    ElMessage.error(t('tenantManagement.loadFailed'))
  } finally {
    loading.value = false
  }
}

const searchQuery = ref('')
const currentPage = ref(1)
const pageSize = ref(10)
const detailDialogVisible = ref(false)
const accessDialogVisible = ref(false)
const editingTenant = ref(null)
const selectedTenant = ref(null)
const accessTab = ref('users')
const isEditingExisting = ref(false)
const tenantFormRef = ref(null)
const roleDialogVisible = ref(false)
const roleSaving = ref(false)
const editingUser = ref(null)
const roleForm = reactive({
  role: 'operator'
})

const roleOptions = computed(() => [
  { value: 'admin', label: `${t('tenantManagement.roleAdmin')} (admin)` },
  { value: 'operator', label: `${t('tenantManagement.roleOperator')} (operator)` },
  { value: 'viewer', label: `${t('tenantManagement.roleViewer')} (viewer)` }
])

const safeLower = (value) => String(value ?? '').toLowerCase()

const filteredTenants = computed(() => {
  if (!searchQuery.value) {
    return tenants.value
  }

  const query = safeLower(searchQuery.value)
  return tenants.value.filter(tenant =>
    safeLower(tenant.name).includes(query) ||
    safeLower(tenant.code).includes(query) ||
    safeLower(tenant.description).includes(query)
  )
})

const paginatedTenants = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  return filteredTenants.value.slice(start, start + pageSize.value)
})

const dialogTitle = computed(() => {
  return editingTenant.value?.id ? t('tenantManagement.edit') : t('tenantManagement.createNew')
})

const getToggleConfirm = (tenant) => {
  const action = tenant.status === 'active' ? t('tenantManagement.suspend') : t('tenantManagement.activate')
  return t('tenantManagement.toggleConfirm')
    .replace('{action}', action)
    .replace('{name}', tenant.name)
}

const getStatusType = (status) => {
  switch(status) {
    case 'active': return 'success'
    case 'suspended': return 'warning'
    case 'inactive': return 'info'
    default: return 'info'
  }
}

const getStatusText = (status) => {
  switch(status) {
    case 'active': return t('tenantManagement.statusActive')
    case 'suspended': return t('tenantManagement.statusSuspended')
    case 'inactive': return t('tenantManagement.statusInactive')
    default: return status
  }
}

const getRoleType = (role) => {
  switch(role) {
    case 'admin': return 'primary'
    case 'operator':
    case 'member': return 'success'
    case 'viewer': return 'info'
    default: return 'default'
  }
}

const getRoleLabel = (role) => {
  switch (role) {
    case 'admin': return t('tenantManagement.roleAdmin')
    case 'operator': return t('tenantManagement.roleOperator')
    case 'member': return t('tenantManagement.roleMember')
    case 'viewer': return t('tenantManagement.roleViewer')
    default: return role || '-'
  }
}

const tenantRules = computed(() => ({
  name: [
    { required: true, message: t('tenantManagement.nameRequired'), trigger: 'blur' },
    { min: 2, max: 50, message: t('tenantManagement.nameLength'), trigger: 'blur' }
  ],
  code: [
    { required: true, message: t('tenantManagement.codeRequired'), trigger: 'blur' },
    { min: 2, max: 20, message: t('tenantManagement.codeLength'), trigger: 'blur' },
    { pattern: /^[a-z0-9][a-z0-9\-]*[a-z0-9]$/, message: t('tenantManagement.codePattern'), trigger: 'blur' }
  ]
}))

const refreshTenants = () => {
  loadTenants()
}

const createTenant = () => {
  editingTenant.value = {
    name: '',
    code: '',
    status: 'active',
    description: '',
    limits: { maxNodes: 10, maxTokens: 5, maxBandwidth: 100 },
    users: []
  }
  isEditingExisting.value = false
  detailDialogVisible.value = true
}

const viewTenantDetails = (tenant) => {
  editingTenant.value = { ...tenant }
  isEditingExisting.value = true
  detailDialogVisible.value = true
}

const editTenant = (tenant) => {
  editingTenant.value = { ...tenant }
  isEditingExisting.value = true
  detailDialogVisible.value = true
}

const toggleTenantStatus = async (tenant) => {
  const previousStatus = tenant.status
  const newStatus = tenant.status === 'active' ? 'suspended' : 'active'
  try {
    await api.put(API_ENDPOINTS.TENANT.DETAIL(tenant.id), {
      status: newStatus
    })
    tenant.status = newStatus

    const index = tenants.value.findIndex(t => t.id === tenant.id)
    if (index !== -1) {
      tenants.value[index] = { ...tenants.value[index], status: newStatus }
    }

    const action = newStatus === 'active' ? t('tenantManagement.activated') : t('tenantManagement.statusSuspendedVerb')
    ElMessage.success(t('tenantManagement.statusUpdated').replace('{name}', tenant.name).replace('{action}', action))
  } catch (error) {
    tenant.status = previousStatus
    console.error('Failed to update tenant status:', error)
    ElMessage.error(t('tenantManagement.updateStatusFailed'))
  }
}

const deleteTenant = async (id) => {
  try {
    await api.delete(API_ENDPOINTS.TENANT.DETAIL(id))
    tenants.value = tenants.value.filter(tenant => tenant.id !== id)
    ElMessage.success(t('tenantManagement.deleted'))
  } catch (error) {
    console.error('Failed to delete tenant:', error)
    ElMessage.error(t('tenantManagement.deleteFailed'))
  }
}

const saveTenant = async () => {
  if (!tenantFormRef.value) return

  try {
    await tenantFormRef.value.validate()

    const tenantData = {
      name: editingTenant.value.name,
      code: editingTenant.value.code || '',
      description: editingTenant.value.description || ''
    }

    if (isEditingExisting.value) {
      await api.put(API_ENDPOINTS.TENANT.DETAIL(editingTenant.value.id), tenantData)
      const index = tenants.value.findIndex(t => t.id === editingTenant.value.id)
      if (index !== -1) {
        tenants.value[index] = { ...tenants.value[index], ...tenantData }
      }
      ElMessage.success(t('tenantManagement.updated'))
    } else {
      const response = await api.post(API_ENDPOINTS.TENANT.LIST, tenantData)
      const newTenant = {
        ...response.data?.data || tenantData,
        status: response.data?.data?.status || 'active',
        nodeCount: 0,
        tokenCount: 0,
        createdAt: new Date().toISOString(),
        users: []
      }
      tenants.value.push(newTenant)
      ElMessage.success(t('tenantManagement.created'))
    }

    detailDialogVisible.value = false
  } catch (error) {
    console.error('Failed to save tenant:', error)
    ElMessage.error(t('tenantManagement.saveFailed'))
  }
}

const manageAccess = async (tenant) => {
  selectedTenant.value = { ...tenant, users: [] }
  accessDialogVisible.value = true
  
  try {
    const response = await api.get(API_ENDPOINTS.TENANT.USERS(tenant.id))
    selectedTenant.value.users = response.data?.data || response.data || []
  } catch (error) {
    console.error('Failed to load users:', error)
    ElMessage.error(t('tenantManagement.loadUsersFailed'))
  }
}

const addUserToTenant = () => {
  ElMessageBox.prompt(t('tenantManagement.promptUsername'), t('tenantManagement.addUserTitle'), {
    confirmButtonText: t('tenantManagement.add'),
    cancelButtonText: t('common.cancel'),
    inputPattern: /\S+/,
    inputErrorMessage: t('tenantManagement.promptUsername')
  }).then(async ({ value }) => {
    const username = value
    ElMessageBox.prompt(t('tenantManagement.promptPassword'), t('tenantManagement.setPasswordTitle'), {
      confirmButtonText: t('common.confirm'),
      cancelButtonText: t('common.cancel'),
      inputPattern: /.{6,}/,
      inputErrorMessage: t('tenantManagement.passwordMin')
    }).then(async ({ value: password }) => {
      try {
        await api.post(API_ENDPOINTS.TENANT.USERS(selectedTenant.value.id), {
          username,
          password,
          role: 'operator',
          email: ''
        })
        ElMessage.success(t('tenantManagement.userAdded'))
        manageAccess(selectedTenant.value)
      } catch (error) {
        console.error('Failed to add user:', error)
        ElMessage.error(t('tenantManagement.addUserFailed'))
      }
    }).catch(() => {})
  }).catch(() => {})
}

const editUserRole = (user) => {
  editingUser.value = user
  roleForm.role = ['admin', 'operator', 'viewer'].includes(user.role) ? user.role : 'operator'
  roleDialogVisible.value = true
}

const saveUserRole = async () => {
  if (!selectedTenant.value?.id || !editingUser.value?.id) {
    ElMessage.error(t('tenantManagement.missingTenantOrUser'))
    return
  }

  roleSaving.value = true
  try {
    await api.put(API_ENDPOINTS.TENANT.USER_DETAIL(selectedTenant.value.id, editingUser.value.id), {
      role: roleForm.role
    })
    ElMessage.success(t('tenantManagement.roleUpdated'))
    roleDialogVisible.value = false
    await manageAccess(selectedTenant.value)
  } catch (error) {
    console.error('Failed to update user:', error)
    ElMessage.error(t('tenantManagement.updateUserFailed'))
  } finally {
    roleSaving.value = false
  }
}

const removeUserFromTenant = async (userId) => {
  try {
    await api.delete(API_ENDPOINTS.TENANT.USER_DETAIL(selectedTenant.value.id, userId))
    ElMessage.success(t('tenantManagement.userDeleted'))
    manageAccess(selectedTenant.value)
  } catch (error) {
    console.error('Failed to delete user:', error)
    ElMessage.error(t('tenantManagement.deleteUserFailed'))
  }
}

const closeDetailDialog = () => {
  detailDialogVisible.value = false
  editingTenant.value = null
}

const handleSizeChange = (size) => {
  pageSize.value = size
  currentPage.value = 1
}

const handleCurrentChange = (page) => {
  currentPage.value = page
}
</script>

<style scoped>
.tenant-management {
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
}

.pagination-container {
  margin-top: 20px;
  display: flex;
  justify-content: flex-end;
}

.tenant-name-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.tenant-icon {
  width: 16px;
  height: 16px;
}
</style>
