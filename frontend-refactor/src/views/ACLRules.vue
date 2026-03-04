<template>
  <div class="acl-rules-container">
    <div class="page-header">
      <h2>ACL 规则管理</h2>
      <el-button type="primary" @click="handleCreate">
        <el-icon><Plus /></el-icon>
        新建规则
      </el-button>
    </div>

    <div class="filter-section">
      <el-input
        v-model="filters.name"
        placeholder="搜索规则名称"
        style="width: 200px"
        @input="debouncedSearch"
      >
        <template #prefix>
          <el-icon><Search /></el-icon>
        </template>
      </el-input>
      
      <el-select v-model="filters.action" placeholder="动作" clearable @change="loadRules">
        <el-option label="允许" value="allow" />
        <el-option label="拒绝" value="deny" />
        <el-option label="通过" value="pass" />
        <el-option label="丢弃" value="drop" />
      </el-select>
      
      <el-select v-model="filters.enabled" placeholder="状态" clearable @change="loadRules">
        <el-option label="启用" :value="true" />
        <el-option label="禁用" :value="false" />
      </el-select>
      
      <el-button @click="loadRules">
        <el-icon><Search /></el-icon>
        搜索
      </el-button>
    </div>

    <el-table :data="rules" v-loading="loading" style="width: 100%">
      <el-table-column prop="priority" label="优先级" width="80" sortable />
      <el-table-column prop="name" label="名称" width="150" />
      <el-table-column prop="src_net" label="源网络" width="150" />
      <el-table-column prop="dst_net" label="目标网络" width="150" />
      <el-table-column label="端口范围" width="120">
        <template #default="{ row }">
          {{ row.min_port || 0 }} - {{ row.max_port || 65535 }}
        </template>
      </el-table-column>
      <el-table-column prop="protocol" label="协议" width="80">
        <template #default="{ row }">
          {{ getProtocolName(row.protocol) }}
        </template>
      </el-table-column>
      <el-table-column prop="action" label="动作" width="80">
        <template #default="{ row }">
          <el-tag :type="getActionType(row.action)">
            {{ getActionName(row.action) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="enabled" label="状态" width="80">
        <template #default="{ row }">
          <el-switch v-model="row.enabled" @change="handleToggleEnabled(row)" />
        </template>
      </el-table-column>
      <el-table-column label="操作" width="150" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="handleEdit(row)">编辑</el-button>
          <el-button link type="danger" @click="handleDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <div class="pagination-section">
      <el-pagination
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.pageSize"
        :page-sizes="[10, 20, 50, 100]"
        :total="pagination.total"
        layout="total, sizes, prev, pager, next, jumper"
        @size-change="loadRules"
        @current-change="loadRules"
      />
    </div>

    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="600px" @closed="resetForm">
      <el-form ref="formRef" :model="form" :rules="formRules" label-width="120px">
        <el-form-item label="规则名称" prop="name">
          <el-input v-model="form.name" placeholder="请输入规则名称" />
        </el-form-item>
        
        <el-form-item label="源网络" prop="src_net">
          <el-input v-model="form.src_net" placeholder="例如: 192.168.1.0/24" />
        </el-form-item>
        
        <el-form-item label="目标网络" prop="dst_net">
          <el-input v-model="form.dst_net" placeholder="例如: 10.0.0.0/24" />
        </el-form-item>
        
        <el-form-item label="协议" prop="protocol">
          <el-select v-model="form.protocol">
            <el-option label="TCP" :value="6" />
            <el-option label="UDP" :value="17" />
            <el-option label="ICMP" :value="1" />
          </el-select>
        </el-form-item>
        
        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="最小端口">
              <el-input-number v-model="form.min_port" :min="0" :max="65535" style="width: 100%" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="最大端口">
              <el-input-number v-model="form.max_port" :min="0" :max="65535" style="width: 100%" />
            </el-form-item>
          </el-col>
        </el-row>
        
        <el-form-item label="动作" prop="action">
          <el-select v-model="form.action">
            <el-option label="允许 (allow)" value="allow" />
            <el-option label="拒绝 (deny)" value="deny" />
            <el-option label="通过 (pass)" value="pass" />
            <el-option label="丢弃 (drop)" value="drop" />
          </el-select>
        </el-form-item>
        
        <el-form-item label="优先级" prop="priority">
          <el-input-number v-model="form.priority" :min="1" :max="10000" style="width: 100%" />
          <div class="form-help">数字越小优先级越高</div>
        </el-form-item>
        
        <el-form-item label="是否启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
        
        <el-form-item label="描述">
          <el-input v-model="form.description" type="textarea" :rows="3" placeholder="请输入规则描述（可选）" />
        </el-form-item>
      </el-form>
      
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="handleSubmit">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import { useAclApi } from '@/composables/useAclApi'

const loading = ref(false)
const rules = ref([])
const dialogVisible = ref(false)
const submitting = ref(false)
const formRef = ref(null)

const filters = reactive({
  name: '',
  action: '',
  enabled: undefined
})

const pagination = reactive({
  page: 1,
  pageSize: 50,
  total: 0
})

const form = reactive({
  id: null,
  name: '',
  src_net: '',
  dst_net: '',
  protocol: 6,
  min_port: null,
  max_port: null,
  action: 'allow',
  enabled: true,
  priority: 100,
  description: ''
})

const formRules = {
  name: [
    { required: true, message: '请输入规则名称', trigger: 'blur' },
    { min: 1, max: 50, message: '长度在 1 到 50 个字符', trigger: 'blur' }
  ],
  src_net: [
    { required: true, message: '请输入源网络', trigger: 'blur' },
    { pattern: /^(\d{1,3}\.){3}\d{1,3}\/\d{1,2}$/, message: '请输入有效的 CIDR 格式', trigger: 'blur' }
  ],
  dst_net: [
    { required: true, message: '请输入目标网络', trigger: 'blur' },
    { pattern: /^(\d{1,3}\.){3}\d{1,3}\/\d{1,2}$/, message: '请输入有效的 CIDR 格式', trigger: 'blur' }
  ],
  protocol: [
    { required: true, message: '请选择协议', trigger: 'change' }
  ],
  action: [
    { required: true, message: '请选择动作', trigger: 'change' }
  ],
  priority: [
    { required: true, message: '请输入优先级', trigger: 'blur' }
  ]
}

const dialogTitle = computed(() => form.id ? '编辑规则' : '新建规则')

const loadRules = async () => {
  loading.value = true
  try {
    const params = { ...filters, page: pagination.page, page_size: pagination.pageSize }
    Object.keys(params).forEach(key => {
      if (params[key] === '' || params[key] === undefined) delete params[key]
    })
    
    const response = await useAclApi.getACLRules(params)
    
    if (Array.isArray(response)) {
      rules.value = response
      pagination.total = response.length
    } else if (response.data) {
      rules.value = response.data
      pagination.total = response.meta?.total || response.data.length
    }
  } catch (error) {
    ElMessage.error('加载规则失败: ' + (error.message || '未知错误'))
  } finally {
    loading.value = false
  }
}

let searchTimer = null
const debouncedSearch = () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    pagination.page = 1
    loadRules()
  }, 500)
}

const handleCreate = () => {
  resetForm()
  dialogVisible.value = true
}

const handleEdit = (row) => {
  Object.assign(form, row)
  dialogVisible.value = true
}

const handleDelete = async (row) => {
  try {
    await ElMessageBox.confirm(`确定要删除规则 "${row.name}" 吗？`, '删除确认', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    })
    
    await useAclApi.deleteACLRule(row.id)
    ElMessage.success('删除成功')
    loadRules()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('删除失败: ' + (error.message || '未知错误'))
    }
  }
}

const handleToggleEnabled = async (row) => {
  try {
    await useAclApi.updateACLRule(row.id, { enabled: row.enabled })
    ElMessage.success(row.enabled ? '已启用' : '已禁用')
  } catch (error) {
    row.enabled = !row.enabled
    ElMessage.error('操作失败: ' + (error.message || '未知错误'))
  }
}

const handleSubmit = async () => {
  if (!formRef.value) return
  
  try {
    await formRef.value.validate()
    submitting.value = true
    
    const data = { ...form }
    
    if (data.min_port !== null && data.max_port !== null) {
      if (data.min_port > data.max_port) {
        ElMessage.error('最小端口不能大于最大端口')
        return
      }
    }
    
    if (form.id) {
      await useAclApi.updateACLRule(form.id, data)
      ElMessage.success('更新成功')
    } else {
      await useAclApi.createACLRule(data)
      ElMessage.success('创建成功')
    }
    
    dialogVisible.value = false
    loadRules()
  } catch (error) {
    if (error !== false) {
      ElMessage.error('操作失败: ' + (error.message || '未知错误'))
    }
  } finally {
    submitting.value = false
  }
}

const resetForm = () => {
  Object.assign(form, {
    id: null, name: '', src_net: '', dst_net: '',
    protocol: 6, min_port: null, max_port: null,
    action: 'allow', enabled: true, priority: 100, description: ''
  })
  if (formRef.value) formRef.value.resetFields()
}

const getProtocolName = (protocol) => {
  const map = { 6: 'TCP', 17: 'UDP', 1: 'ICMP' }
  return map[protocol] || 'Unknown'
}

const getActionName = (action) => {
  const map = { allow: '允许', deny: '拒绝', pass: '通过', drop: '丢弃' }
  return map[action] || action
}

const getActionType = (action) => {
  const map = { allow: 'success', deny: 'danger', pass: 'info', drop: 'warning' }
  return map[action] || ''
}

onMounted(() => {
  loadRules()
})
</script>

<style scoped>
.acl-rules-container { padding: 20px; }
.page-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; }
.page-header h2 { margin: 0; font-size: 20px; }
.filter-section { display: flex; gap: 10px; margin-bottom: 20px; flex-wrap: wrap; }
.pagination-section { margin-top: 20px; display: flex; justify-content: flex-end; }
.form-help { font-size: 12px; color: #909399; margin-top: 5px; }
</style>
