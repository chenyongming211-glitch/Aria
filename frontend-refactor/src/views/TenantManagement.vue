<!-- src/views/TenantManagement.vue -->
<template>
  <div class="tenant-management">
    <el-card>
      <template #header>
        <div class="card-header">
          <h3>Tenant Management</h3>
          <div class="header-actions">
            <el-input
              v-model="searchQuery"
              placeholder="Search tenants..."
              style="width: 200px; margin-right: 10px;"
              clearable
            >
              <template #prefix>
                <el-icon><Search /></el-icon>
              </template>
            </el-input>
            <el-button type="primary" @click="refreshTenants">
              <el-icon><Refresh /></el-icon>
              Refresh
            </el-button>
            <el-button type="primary" @click="createTenant">
              <el-icon><Plus /></el-icon>
              Create Tenant
            </el-button>
          </div>
        </div>
      </template>

      <el-table
        :data="filteredTenants"
        stripe
        style="width: 100%"
        v-loading="loading"
        row-key="id"
        :tree-props="{children: 'children', hasChildren: 'hasChildren'}"
      >
        <el-table-column prop="name" label="Name" width="200">
          <template #default="{ row }">
            <div class="tenant-name-cell">
              <el-icon v-if="row.icon" :class="row.icon" class="tenant-icon" />
              <span>{{ row.name }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="code" label="Code" width="150" />
        <el-table-column prop="status" label="Status" width="120">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)">
              {{ row.status }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="nodeCount" label="Nodes" width="100" />
        <el-table-column prop="tokenCount" label="Tokens" width="100" />
        <el-table-column prop="createdAt" label="Created At" width="180" />
        <el-table-column prop="description" label="Description" show-overflow-tooltip />
        <el-table-column label="Actions" width="250">
          <template #default="{ row }">
            <el-button size="small" @click="viewTenantDetails(row)">View</el-button>
            <el-button size="small" type="primary" @click="editTenant(row)">Edit</el-button>
            <el-button size="small" type="info" @click="manageAccess(row)">Manage Access</el-button>
            <el-popconfirm
              :title="`Are you sure to ${row.status === 'active' ? 'suspend' : 'activate'} tenant ${row.name}?`"
              @confirm="toggleTenantStatus(row)"
            >
              <template #reference>
                <el-button size="small" :type="row.status === 'active' ? 'warning' : 'success'">
                  {{ row.status === 'active' ? 'Suspend' : 'Activate' }}
                </el-button>
              </template>
            </el-popconfirm>
            <el-popconfirm
              title="Are you sure to delete this tenant? This action cannot be undone."
              @confirm="deleteTenant(row.id)"
            >
              <template #reference>
                <el-button size="small" type="danger">Delete</el-button>
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
      width="60%"
      :before-close="closeDetailDialog"
    >
      <el-form
        v-if="editingTenant"
        :model="editingTenant"
        :rules="tenantRules"
        ref="tenantFormRef"
        label-width="120px"
      >
        <el-form-item label="Name" prop="name">
          <el-input v-model="editingTenant.name" :disabled="isEditingExisting" />
        </el-form-item>
        <el-form-item label="Code" prop="code">
          <el-input v-model="editingTenant.code" :disabled="isEditingExisting" />
        </el-form-item>
        <el-form-item label="Status" prop="status">
          <el-select v-model="editingTenant.status" placeholder="Select status">
            <el-option label="Active" value="active" />
            <el-option label="Inactive" value="inactive" />
            <el-option label="Suspended" value="suspended" />
          </el-select>
        </el-form-item>
        <el-form-item label="Description" prop="description">
          <el-input
            v-model="editingTenant.description"
            type="textarea"
            :rows="4"
            placeholder="Enter tenant description"
          />
        </el-form-item>
        <el-form-item label="Resource Limits">
          <el-row :gutter="10">
            <el-col :span="8">
              <el-form-item label="Max Nodes">
                <el-input-number v-model="editingTenant.limits.maxNodes" :min="0" :max="10000" />
              </el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item label="Max Tokens">
                <el-input-number v-model="editingTenant.limits.maxTokens" :min="0" :max="1000" />
              </el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item label="Max Bandwidth (Mbps)">
                <el-input-number v-model="editingTenant.limits.maxBandwidth" :min="0" :max="10000" />
              </el-form-item>
            </el-col>
          </el-row>
        </el-form-item>
      </el-form>

      <template #footer>
        <span class="dialog-footer">
          <el-button @click="closeDetailDialog">Cancel</el-button>
          <el-button type="primary" @click="saveTenant">Confirm</el-button>
        </span>
      </template>
    </el-dialog>

    <!-- Access Management Dialog -->
    <el-dialog
      v-model="accessDialogVisible"
      title="Manage Tenant Access"
      width="70%"
    >
      <div v-if="selectedTenant">
        <el-tabs v-model="accessTab">
          <el-tab-pane label="Users" name="users">
            <el-button type="primary" @click="addUserToTenant" style="margin-bottom: 15px;">
              <el-icon><Plus /></el-icon>
              Add User
            </el-button>

            <el-table :data="selectedTenant.users" style="width: 100%">
              <el-table-column prop="name" label="User Name" width="150" />
              <el-table-column prop="email" label="Email" width="200" />
              <el-table-column prop="role" label="Role" width="120">
                <template #default="{ row }">
                  <el-tag :type="getRoleType(row.role)">{{ row.role }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="joinedAt" label="Joined At" width="180" />
              <el-table-column label="Actions" width="150">
                <template #default="{ row }">
                  <el-button size="small" type="primary" @click="editUserRole(row)">Edit Role</el-button>
                  <el-popconfirm
                    title="Are you sure to remove this user from tenant?"
                    @confirm="removeUserFromTenant(row.id)"
                  >
                    <template #reference>
                      <el-button size="small" type="danger">Remove</el-button>
                    </template>
                  </el-popconfirm>
                </template>
              </el-table-column>
            </el-table>
          </el-tab-pane>

          <el-tab-pane label="Permissions" name="permissions">
            <el-card>
              <h4>Resource Permissions</h4>
              <el-checkbox-group v-model="selectedTenant.permissions">
                <el-checkbox label="view_nodes">View Nodes</el-checkbox>
                <el-checkbox label="manage_nodes">Manage Nodes</el-checkbox>
                <el-checkbox label="view_topology">View Topology</el-checkbox>
                <el-checkbox label="manage_tokens">Manage Tokens</el-checkbox>
                <el-checkbox label="view_monitoring">View Monitoring</el-checkbox>
                <el-checkbox label="receive_alerts">Receive Alerts</el-checkbox>
              </el-checkbox-group>
            </el-card>
          </el-tab-pane>
        </el-tabs>
      </div>

      <template #footer>
        <span class="dialog-footer">
          <el-button @click="accessDialogVisible = false">Cancel</el-button>
          <el-button type="primary" @click="saveAccessChanges">Save Changes</el-button>
        </span>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, reactive } from 'vue'
import { Search, Refresh, Plus } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'

// Mock data
const tenants = ref([
  {
    id: 'tenant-1',
    name: 'Acme Corporation',
    code: 'acme',
    status: 'active',
    nodeCount: 12,
    tokenCount: 5,
    createdAt: '2024-01-15 10:30:25',
    description: 'Main corporate tenant',
    limits: { maxNodes: 50, maxTokens: 20, maxBandwidth: 1000 },
    users: [
      { id: 'user-1', name: 'John Doe', email: 'john@acme.com', role: 'admin', joinedAt: '2024-01-15' },
      { id: 'user-2', name: 'Jane Smith', email: 'jane@acme.com', role: 'member', joinedAt: '2024-01-16' }
    ],
    permissions: ['view_nodes', 'manage_nodes', 'view_topology', 'manage_tokens', 'view_monitoring']
  },
  {
    id: 'tenant-2',
    name: 'Beta Startup',
    code: 'beta',
    status: 'active',
    nodeCount: 5,
    tokenCount: 2,
    createdAt: '2024-02-20 14:22:10',
    description: 'Development tenant',
    limits: { maxNodes: 20, maxTokens: 10, maxBandwidth: 500 },
    users: [
      { id: 'user-3', name: 'Bob Johnson', email: 'bob@beta.com', role: 'admin', joinedAt: '2024-02-20' }
    ],
    permissions: ['view_nodes', 'view_topology', 'view_monitoring']
  },
  {
    id: 'tenant-3',
    name: 'Gamma LLC',
    code: 'gamma',
    status: 'suspended',
    nodeCount: 8,
    tokenCount: 3,
    createdAt: '2024-03-10 09:15:42',
    description: 'Partner tenant',
    limits: { maxNodes: 30, maxTokens: 15, maxBandwidth: 800 },
    users: [
      { id: 'user-4', name: 'Alice Williams', email: 'alice@gamma.com', role: 'admin', joinedAt: '2024-03-10' },
      { id: 'user-5', name: 'Charlie Brown', email: 'charlie@gamma.com', role: 'member', joinedAt: '2024-03-11' }
    ],
    permissions: ['view_nodes', 'view_monitoring']
  },
  {
    id: 'tenant-4',
    name: 'Delta Operations',
    code: 'delta',
    status: 'inactive',
    nodeCount: 0,
    tokenCount: 0,
    createdAt: '2024-04-05 16:45:18',
    description: 'Legacy tenant',
    limits: { maxNodes: 10, maxTokens: 5, maxBandwidth: 200 },
    users: [],
    permissions: []
  }
])

const loading = ref(false)
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

const filteredTenants = computed(() => {
  if (!searchQuery.value) {
    return tenants.value
  }

  return tenants.value.filter(tenant =>
    tenant.name.toLowerCase().includes(searchQuery.value.toLowerCase()) ||
    tenant.code.toLowerCase().includes(searchQuery.value.toLowerCase()) ||
    tenant.description.toLowerCase().includes(searchQuery.value.toLowerCase())
  )
})

const dialogTitle = computed(() => {
  return editingTenant.value?.id ? 'Edit Tenant' : 'Create New Tenant'
})

const getStatusType = (status) => {
  switch(status) {
    case 'active': return 'success'
    case 'suspended': return 'warning'
    case 'inactive': return 'info'
    default: return 'info'
  }
}

const getRoleType = (role) => {
  switch(role) {
    case 'admin': return 'primary'
    case 'member': return 'success'
    case 'viewer': return 'info'
    default: return 'default'
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
  loading.value = true
  // Simulate API call
  setTimeout(() => {
    loading.value = false
    ElMessage.success('Tenants refreshed')
  }, 1000)
}

const createTenant = () => {
  editingTenant.value = {
    name: '',
    code: '',
    status: 'active',
    description: '',
    limits: { maxNodes: 10, maxTokens: 5, maxBandwidth: 100 },
    users: [],
    permissions: []
  }
  isEditingExisting = false
  detailDialogVisible.value = true
}

const viewTenantDetails = (tenant) => {
  editingTenant.value = { ...tenant }
  isEditingExisting = true
  detailDialogVisible.value = true
}

const editTenant = (tenant) => {
  editingTenant.value = { ...tenant }
  isEditingExisting = true
  detailDialogVisible.value = true
}

const toggleTenantStatus = (tenant) => {
  const newStatus = tenant.status === 'active' ? 'suspended' : 'active'
  tenant.status = newStatus

  const action = newStatus === 'active' ? 'activated' : 'suspended'
  ElMessage.success(`Tenant ${tenant.name} has been ${action}`)
}

const deleteTenant = (id) => {
  tenants.value = tenants.value.filter(tenant => tenant.id !== id)
  ElMessage.success('Tenant deleted')
}

const saveTenant = async () => {
  if (!tenantFormRef.value) return

  try {
    await tenantFormRef.value.validate()

    if (isEditingExisting.value) {
      // Update existing tenant
      const index = tenants.value.findIndex(t => t.id === editingTenant.value.id)
      if (index !== -1) {
        tenants.value[index] = { ...editingTenant.value }
      }
      ElMessage.success('Tenant updated successfully')
    } else {
      // Create new tenant
      editingTenant.value.id = `tenant-${Date.now()}`
      editingTenant.value.nodeCount = 0
      editingTenant.value.tokenCount = 0
      editingTenant.value.createdAt = new Date().toLocaleString()
      tenants.value.push({ ...editingTenant.value })
      ElMessage.success('Tenant created successfully')
    }

    detailDialogVisible.value = false
  } catch (error) {
    ElMessage.error('Please fill in all required fields correctly')
  }
}

const manageAccess = (tenant) => {
  selectedTenant.value = { ...tenant }
  accessDialogVisible.value = true
}

const addUserToTenant = () => {
  ElMessageBox.prompt('Please enter user email', 'Add User', {
    confirmButtonText: 'Add',
    cancelButtonText: 'Cancel',
    inputPattern: /\S+@\S+\.\S+/,
    inputErrorMessage: 'Invalid email format'
  }).then(({ value }) => {
    // Add user to tenant's user list
    const newUser = {
      id: `user-${Date.now()}`,
      name: value.split('@')[0],
      email: value,
      role: 'member',
      joinedAt: new Date().toISOString().split('T')[0]
    }
    selectedTenant.value.users.push(newUser)
    ElMessage.success(`User ${value} added to tenant`)
  }).catch(() => {
    // Cancelled
  })
}

const editUserRole = (user) => {
  ElMessageBox({
    title: 'Change User Role',
    message: `Select new role for ${user.name}`,
    showCancelButton: true,
    confirmButtonText: 'Update',
    inputType: 'select',
    inputOptions: [
      { value: 'viewer', label: 'Viewer' },
      { value: 'member', label: 'Member' },
      { value: 'admin', label: 'Administrator' }
    ],
    inputValue: user.role,
    inputValidator: (value) => {
      if (!value) return 'Please select a role'
      return true
    }
  }).then(({ value }) => {
    user.role = value
    ElMessage.success(`Role updated for ${user.name}`)
  }).catch(() => {
    // Cancelled
  })
}

const removeUserFromTenant = (userId) => {
  selectedTenant.value.users = selectedTenant.value.users.filter(u => u.id !== userId)
  ElMessage.success('User removed from tenant')
}

const saveAccessChanges = () => {
  // Find and update the tenant in the main list
  const index = tenants.value.findIndex(t => t.id === selectedTenant.value.id)
  if (index !== -1) {
    tenants.value[index] = { ...selectedTenant.value }
  }

  accessDialogVisible.value = false
  ElMessage.success('Access settings saved')
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