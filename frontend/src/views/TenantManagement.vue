<!-- src/views/TenantManagement.vue -->
<template>
  <div class="tenant-management">
    <el-card>
      <template #header>
        <div class="card-header">
          <h3>租户管理</h3>
          <div class="header-actions">
            <el-input
              v-model="searchQuery"
              placeholder="搜索租户..."
              style="width: 200px; margin-right: 10px;"
              clearable
            >
              <template #prefix>
                <el-icon><Search /></el-icon>
              </template>
            </el-input>
            <el-button type="primary" @click="refreshTenants">
              <el-icon><Refresh /></el-icon>
              刷新
            </el-button>
            <el-button type="primary" @click="createTenant">
              <el-icon><Plus /></el-icon>
              创建租户
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
        <el-table-column prop="name" label="名称" width="200">
          <template #default="{ row }">
            <div class="tenant-name-cell">
              <el-icon v-if="row.icon" :class="row.icon" class="tenant-icon" />
              <span>{{ row.name }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="code" label="编码" width="150" />
        <el-table-column prop="status" label="状态" width="120">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)">
              {{ getStatusText(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="nodeCount" label="节点数" width="100" />
        <el-table-column prop="tokenCount" label="Token数" width="100" />
        <el-table-column prop="createdAt" label="创建时间" width="180" />
        <el-table-column prop="description" label="描述" show-overflow-tooltip />
        <el-table-column label="操作" width="280">
          <template #default="{ row }">
            <el-button size="small" @click="viewTenantDetails(row)">查看</el-button>
            <el-button size="small" type="primary" @click="editTenant(row)">编辑</el-button>
            <el-button size="small" type="info" @click="manageAccess(row)">管理访问</el-button>
            <el-popconfirm
              :title="`确定要${row.status === 'active' ? '暂停' : '激活'}租户 ${row.name} 吗?`"
              @confirm="toggleTenantStatus(row)"
            >
              <template #reference>
                <el-button size="small" :type="row.status === 'active' ? 'warning' : 'success'">
                  {{ row.status === 'active' ? '暂停' : '激活' }}
                </el-button>
              </template>
            </el-popconfirm>
            <el-popconfirm
              title="确定要删除此租户吗？此操作无法撤销。"
              @confirm="deleteTenant(row.id)"
            >
              <template #reference>
                <el-button size="small" type="danger">删除</el-button>
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
        <el-form-item label="名称" prop="name">
          <el-input v-model="editingTenant.name" :disabled="isEditingExisting" placeholder="请输入租户名称" />
        </el-form-item>
        <el-form-item label="编码" prop="code">
          <el-input v-model="editingTenant.code" :disabled="isEditingExisting" placeholder="请输入租户编码" />
        </el-form-item>
        <el-form-item label="描述" prop="description">
          <el-input
            v-model="editingTenant.description"
            type="textarea"
            :rows="3"
            placeholder="请输入租户描述"
          />
        </el-form-item>
      </el-form>

      <template #footer>
        <span class="dialog-footer">
          <el-button @click="closeDetailDialog">取消</el-button>
          <el-button type="primary" @click="saveTenant">确认</el-button>
        </span>
      </template>
    </el-dialog>

    <!-- Access Management Dialog -->
    <el-dialog
      v-model="accessDialogVisible"
      title="管理访问"
      width="70%"
    >
      <div v-if="selectedTenant">
        <el-tabs v-model="accessTab">
          <el-tab-pane label="用户" name="users">
            <el-button type="primary" @click="addUserToTenant" style="margin-bottom: 15px;">
              <el-icon><Plus /></el-icon>
              添加用户
            </el-button>

            <el-table :data="selectedTenant.users" style="width: 100%">
              <el-table-column prop="username" label="用户名" width="150" />
              <el-table-column prop="email" label="邮箱" width="200" />
              <el-table-column prop="role" label="角色" width="120">
                <template #default="{ row }">
                  <el-tag :type="getRoleType(row.role)">{{ getRoleLabel(row.role) }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="操作" width="150">
                <template #default="{ row }">
                  <el-button size="small" type="primary" @click="editUserRole(row)">编辑</el-button>
                  <el-popconfirm
                    title="确定要删除此用户吗？"
                    @confirm="removeUserFromTenant(row.id)"
                  >
                    <template #reference>
                      <el-button size="small" type="danger">删除</el-button>
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
          <el-button @click="accessDialogVisible = false">关闭</el-button>
        </span>
      </template>
    </el-dialog>

    <el-dialog
      v-model="roleDialogVisible"
      title="修改用户角色"
      width="420px"
    >
      <el-form :model="roleForm" label-width="80px">
        <el-form-item label="用户">
          <el-input :value="editingUser?.username || ''" disabled />
        </el-form-item>
        <el-form-item label="角色">
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
          <el-button @click="roleDialogVisible = false">取消</el-button>
          <el-button type="primary" :loading="roleSaving" @click="saveUserRole">更新</el-button>
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
    ElMessage.error('Failed to load tenants')
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

const roleOptions = [
  { value: 'admin', label: '管理员 (admin)' },
  { value: 'operator', label: '操作员 (operator)' },
  { value: 'viewer', label: '只读 (viewer)' }
]

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
  return editingTenant.value?.id ? '编辑租户' : '创建新租户'
})

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
    case 'active': return '激活'
    case 'suspended': return '已暂停'
    case 'inactive': return '未激活'
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
    case 'admin': return '管理员'
    case 'operator': return '操作员'
    case 'member': return '成员'
    case 'viewer': return '只读'
    default: return role || '-'
  }
}

const tenantRules = reactive({
  name: [
    { required: true, message: 'Please enter tenant name', trigger: 'blur' },
    { min: 2, max: 50, message: 'Tenant name must be between 2 and 50 characters', trigger: 'blur' }
  ],
  code: [
    { required: true, message: 'Please enter tenant code', trigger: 'blur' },
    { min: 2, max: 20, message: 'Tenant code must be between 2 and 20 characters', trigger: 'blur' },
    { pattern: /^[a-z0-9][a-z0-9\-]*[a-z0-9]$/, message: 'Code must start and end with alphanumeric, may contain hyphens', trigger: 'blur' }
  ]
})

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

    const action = newStatus === 'active' ? 'activated' : 'suspended'
    ElMessage.success(`Tenant ${tenant.name} has been ${action}`)
  } catch (error) {
    tenant.status = previousStatus
    console.error('Failed to update tenant status:', error)
    ElMessage.error('更新租户状态失败')
  }
}

const deleteTenant = async (id) => {
  try {
    await api.delete(API_ENDPOINTS.TENANT.DETAIL(id))
    tenants.value = tenants.value.filter(tenant => tenant.id !== id)
    ElMessage.success('Tenant deleted')
  } catch (error) {
    console.error('Failed to delete tenant:', error)
    ElMessage.error('Failed to delete tenant')
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
      ElMessage.success('Tenant updated successfully')
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
      ElMessage.success('Tenant created successfully')
    }

    detailDialogVisible.value = false
  } catch (error) {
    console.error('Failed to save tenant:', error)
    ElMessage.error('Failed to save tenant')
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
    ElMessage.error('加载用户列表失败')
  }
}

const addUserToTenant = () => {
  ElMessageBox.prompt('请输入用户名', '添加用户', {
    confirmButtonText: '添加',
    cancelButtonText: '取消',
    inputPattern: /\S+/,
    inputErrorMessage: '请输入用户名'
  }).then(async ({ value }) => {
    const username = value
    ElMessageBox.prompt('请输入密码', '设置密码', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      inputPattern: /.{6,}/,
      inputErrorMessage: '密码至少6位'
    }).then(async ({ value: password }) => {
      try {
        await api.post(API_ENDPOINTS.TENANT.USERS(selectedTenant.value.id), {
          username,
          password,
          role: 'operator',
          email: ''
        })
        ElMessage.success('用户添加成功')
        manageAccess(selectedTenant.value)
      } catch (error) {
        console.error('Failed to add user:', error)
        ElMessage.error('添加用户失败')
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
    ElMessage.error('缺少租户或用户信息')
    return
  }

  roleSaving.value = true
  try {
    await api.put(API_ENDPOINTS.TENANT.USER_DETAIL(selectedTenant.value.id, editingUser.value.id), {
      role: roleForm.role
    })
    ElMessage.success('角色更新成功')
    roleDialogVisible.value = false
    await manageAccess(selectedTenant.value)
  } catch (error) {
    console.error('Failed to update user:', error)
    ElMessage.error('更新用户失败')
  } finally {
    roleSaving.value = false
  }
}

const removeUserFromTenant = async (userId) => {
  try {
    await api.delete(API_ENDPOINTS.TENANT.USER_DETAIL(selectedTenant.value.id, userId))
    ElMessage.success('用户删除成功')
    manageAccess(selectedTenant.value)
  } catch (error) {
    console.error('Failed to delete user:', error)
    ElMessage.error('删除用户失败')
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
