<!-- src/views/Acl.vue -->
<template>
  <div class="acl">
    <el-card>
      <template #header>
        <div class="card-header">
          <h3>{{ t('nav.aclManagement') }}</h3>
          <div class="header-actions">
            <el-input
              v-model="searchQuery"
              :placeholder="t('common.search')"
              style="width: 200px; margin-right: 10px;"
              clearable
            >
              <template #prefix>
                <el-icon><Search /></el-icon>
              </template>
            </el-input>
            <el-button type="primary" @click="createRule">
              <el-icon><Plus /></el-icon>
              {{ t('common.add') }}
            </el-button>
          </div>
        </div>
      </template>

      <el-table
        :data="filteredRules"
        stripe
        style="width: 100%"
        v-loading="loading"
      >
        <el-table-column prop="id" label="ID" width="120" />
        <el-table-column prop="name" :label="t('common.name')" width="200" />
        <el-table-column prop="type" :label="t('common.type')" width="120" />
        <el-table-column prop="source" label="Source" width="150" />
        <el-table-column prop="destination" label="Destination" width="150" />
        <el-table-column prop="action" :label="t('common.action')" width="100" />
        <el-table-column prop="status" :label="t('common.status')" width="120">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)">
              {{ t(`common.${row.status}`) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="priority" :label="t('common.priority')" width="100" />
        <el-table-column :label="t('common.actions')" width="200">
          <template #default="{ row }">
            <el-button size="small" @click="viewRule(row)">{{ t('common.view') }}</el-button>
            <el-button size="small" type="primary" @click="editRule(row)">{{ t('common.edit') }}</el-button>
            <el-popconfirm
              :title="`Are you sure to delete this rule?`"
              @confirm="deleteRule(row.id)"
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
          :total="filteredRules.length"
          layout="sizes, prev, pager, next, jumper, ->, total"
          @size-change="handleSizeChange"
          @current-change="handleCurrentChange"
        />
      </div>
    </el-card>

    <!-- ACL Rule Detail Dialog -->
    <el-dialog
      v-model="detailDialogVisible"
      :title="editingRule ? t('common.edit') : t('common.add')"
      width="60%"
    >
      <el-form
        v-if="editingRule"
        :model="editingRule"
        label-width="120px"
      >
        <el-form-item :label="t('common.name')">
          <el-input v-model="editingRule.name" />
        </el-form-item>
        <el-form-item :label="t('common.type')">
          <el-select v-model="editingRule.type" placeholder="Select rule type">
            <el-option label="IP Rule" value="ip" />
            <el-option label="Port Rule" value="port" />
            <el-option label="Protocol Rule" value="protocol" />
            <el-option label="User Rule" value="user" />
          </el-select>
        </el-form-item>
        <el-form-item label="Source">
          <el-input v-model="editingRule.source" placeholder="IP range, username, etc." />
        </el-form-item>
        <el-form-item label="Destination">
          <el-input v-model="editingRule.destination" placeholder="IP range, service, etc." />
        </el-form-item>
        <el-form-item :label="t('common.action')">
          <el-select v-model="editingRule.action" placeholder="Select action">
            <el-option label="Allow" value="allow" />
            <el-option label="Deny" value="deny" />
            <el-option label="Log" value="log" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('common.status')">
          <el-select v-model="editingRule.status" placeholder="Select status">
            <el-option :label="t('common.active')" value="active" />
            <el-option :label="t('common.inactive')" value="inactive" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('common.priority')">
          <el-input-number v-model="editingRule.priority" :min="1" :max="100" />
        </el-form-item>
        <el-form-item label="Description">
          <el-input
            v-model="editingRule.description"
            type="textarea"
            :rows="4"
            placeholder="Describe the rule"
          />
        </el-form-item>
      </el-form>

      <template #footer>
        <span class="dialog-footer">
          <el-button @click="detailDialogVisible = false">{{ t('common.cancel') }}</el-button>
          <el-button type="primary" @click="saveRule">{{ t('common.save') }}</el-button>
        </span>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { Search, Plus } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { t } from '@/i18n'

// Mock data
const rules = ref([
  {
    id: 'rule-1',
    name: 'Allow Internal Access',
    type: 'ip',
    source: '10.0.0.0/8',
    destination: '192.168.0.0/16',
    action: 'allow',
    status: 'active',
    priority: 10,
    description: 'Allow access from internal networks to private networks'
  },
  {
    id: 'rule-2',
    name: 'Block External SSH',
    type: 'port',
    source: '0.0.0.0/0',
    destination: 'any:22',
    action: 'deny',
    status: 'active',
    priority: 20,
    description: 'Block external SSH access to any host'
  },
  {
    id: 'rule-3',
    name: 'Admin Access Only',
    type: 'user',
    source: 'admin-users',
    destination: 'critical-services',
    action: 'allow',
    status: 'active',
    priority: 5,
    description: 'Allow admin users access to critical services'
  },
  {
    id: 'rule-4',
    name: 'Logging Rule',
    type: 'protocol',
    source: 'any',
    destination: 'any',
    action: 'log',
    status: 'inactive',
    priority: 100,
    description: 'Log all traffic (high priority, usually disabled)'
  }
])

const loading = ref(false)
const searchQuery = ref('')
const currentPage = ref(1)
const pageSize = ref(10)
const detailDialogVisible = ref(false)
const editingRule = ref(null)

const getStatusType = (status) => {
  switch(status) {
    case 'active': return 'success'
    case 'inactive': return 'info'
    default: return 'info'
  }
}

const filteredRules = computed(() => {
  if (!searchQuery.value) {
    return rules.value
  }

  return rules.value.filter(rule =>
    rule.id.toLowerCase().includes(searchQuery.value.toLowerCase()) ||
    rule.name.toLowerCase().includes(searchQuery.value.toLowerCase()) ||
    rule.type.toLowerCase().includes(searchQuery.value.toLowerCase())
  )
})

const createRule = () => {
  editingRule.value = {
    id: '',
    name: '',
    type: 'ip',
    source: '',
    destination: '',
    action: 'allow',
    status: 'active',
    priority: 50,
    description: ''
  }
  detailDialogVisible.value = true
}

const viewRule = (rule) => {
  editingRule.value = { ...rule }
  detailDialogVisible.value = true
}

const editRule = (rule) => {
  editingRule.value = { ...rule }
  detailDialogVisible.value = true
}

const saveRule = () => {
  if (editingRule.value.id) {
    // Update existing rule
    const index = rules.value.findIndex(r => r.id === editingRule.value.id)
    if (index !== -1) {
      rules.value[index] = { ...editingRule.value }
    }
    ElMessage.success('Rule updated successfully')
  } else {
    // Create new rule
    editingRule.value.id = `rule-${Date.now()}`
    rules.value.push({ ...editingRule.value })
    ElMessage.success('Rule created successfully')
  }
  detailDialogVisible.value = false
}

const deleteRule = (id) => {
  rules.value = rules.value.filter(rule => rule.id !== id)
  ElMessage.success('Rule deleted')
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
.acl {
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

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>