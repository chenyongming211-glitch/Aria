<template>
  <div class="acl-rules-container">
    <div class="page-header">
      <h2>ACL 规则管理</h2>
      <el-button v-if="hasPermission('acls:write')" type="primary" @click="handleCreate">
        <el-icon><Plus /></el-icon>
        新建规则
      </el-button>
    </div>

    <div class="filter-section">
      <el-select v-model="filters.node_id" placeholder="选择节点 (必选)" style="width: 220px" @change="handleNodeChange">
        <el-option
          v-for="node in tenantNodes"
          :key="node.id"
          :label="node.hostname || node.public_key || node.id"
          :value="node.id"
        />
      </el-select>

      <el-divider direction="vertical" />

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

    <el-table :data="paginatedRules" v-loading="loading" style="width: 100%">
      <el-table-column prop="node_name" label="节点" width="160" />
      <el-table-column prop="priority" label="优先级" width="80" sortable />
      <el-table-column prop="name" label="名称" width="150" />
      <el-table-column label="源 Group/CIDR" min-width="170">
        <template #default="{ row }">
          <code>{{ formatGroupRef(row.src_group_id, row.runtime_src_group || row.src_cidr) }}</code>
        </template>
      </el-table-column>
      <el-table-column label="目标 Group/CIDR" min-width="170">
        <template #default="{ row }">
          <code>{{ formatGroupRef(row.dst_group_id, row.runtime_dst_group || row.dst_cidr) }}</code>
        </template>
      </el-table-column>
      <el-table-column label="方向/端口规则" min-width="160">
        <template #default="{ row }">
          <div class="acl-runtime-cell">
            <el-tag size="small" effect="plain">{{ formatDirection(row.direction) }}</el-tag>
            <span>{{ row.runtime_ports || row.ports || 'all' }}</span>
          </div>
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
          <el-switch
            v-model="row.enabled"
            :disabled="!hasPermission('acls:write')"
            @change="handleToggleEnabled(row)"
          />
        </template>
      </el-table-column>
      <el-table-column label="下发状态" width="120">
        <template #default="{ row }">
          <el-tag size="small" :type="getPolicyTagType(row.policy_status)">
            {{ formatPolicyStatus(row.policy_status) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="pending_cmds" label="待执行" width="90" />
      <el-table-column label="Stats" width="150">
        <template #default="{ row }">
          <div class="acl-runtime-cell">
            <span>{{ formatNumber(row.stats?.packets) }} pkts / {{ formatBytes(row.stats?.bytes) }}</span>
            <small>drop {{ formatNumber(row.stats?.dropped_packets) }} / {{ formatBytes(row.stats?.dropped_bytes) }}</small>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="最近命令" width="150">
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
      <el-table-column prop="last_command_error" label="失败原因" min-width="180" show-overflow-tooltip />
      <el-table-column label="操作" width="150" fixed="right">
        <template #default="{ row }">
          <el-button v-if="hasPermission('acls:write')" link type="primary" @click="handleEdit(row)">编辑</el-button>
          <el-button v-if="hasPermission('acls:write')" link type="danger" @click="handleDelete(row)">删除</el-button>
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
        <el-form-item label="目标节点" prop="node_id">
          <el-select v-model="form.node_id" placeholder="请选择节点" style="width: 100%">
            <el-option
              v-for="node in tenantNodes"
              :key="node.id"
              :label="node.hostname || node.public_key || node.id"
              :value="node.id"
            />
          </el-select>
        </el-form-item>

        <el-form-item label="规则名称" prop="name">
          <el-input v-model="form.name" placeholder="请输入规则名称" />
        </el-form-item>
        
        <el-form-item label="源 IP Group">
          <el-select v-model="form.src_group_id" clearable filterable placeholder="any / 选择源 Group" style="width: 100%">
            <el-option
              v-for="group in selectableGroups"
              :key="group.id"
              :label="formatGroupOption(group)"
              :value="group.id"
            />
          </el-select>
        </el-form-item>

        <el-form-item label="源 CIDR" prop="src_cidr">
          <el-input v-model="form.src_cidr" :disabled="Boolean(form.src_group_id)" placeholder="留空为 any；例如 192.168.1.0/24" />
          <div class="form-help">未选择 Group 时，CIDR 会保存为 inline IP Group。</div>
        </el-form-item>

        <el-form-item label="目标 IP Group">
          <el-select v-model="form.dst_group_id" clearable filterable placeholder="any / 选择目标 Group" style="width: 100%">
            <el-option
              v-for="group in selectableGroups"
              :key="group.id"
              :label="formatGroupOption(group)"
              :value="group.id"
            />
          </el-select>
        </el-form-item>

        <el-form-item label="目标 CIDR" prop="dst_cidr">
          <el-input v-model="form.dst_cidr" :disabled="Boolean(form.dst_group_id)" placeholder="留空为 any；例如 10.0.0.0/24" />
          <div class="form-help">未选择 Group 时，CIDR 会保存为 inline IP Group。</div>
        </el-form-item>
        
        <el-form-item label="协议" prop="protocol">
          <el-select v-model="form.protocol">
            <el-option label="Any" :value="0" />
            <el-option label="TCP" :value="6" />
            <el-option label="UDP" :value="17" />
            <el-option label="ICMP" :value="1" />
          </el-select>
        </el-form-item>
        
        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="目标端口" prop="dst_port">
              <el-input-number v-model="form.dst_port" :min="0" :max="65535" style="width: 100%" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="方向" prop="direction">
              <el-select v-model="form.direction" style="width: 100%">
                <el-option label="入站 ingress" value="ingress" />
                <el-option label="出站 egress" value="egress" />
                <el-option label="双向 both" value="both" />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>

        <el-form-item label="端口规则">
          <el-input v-model="form.ports" placeholder="例如: 80-82,443；留空则使用目标端口" />
          <div class="form-help">Any 协议带端口时，Controller 会下发为 TCP/UDP 两条运行时规则。</div>
        </el-form-item>
        
        <el-form-item label="动作" prop="action">
          <el-select v-model="form.action">
            <el-option label="允许 (allow)" value="allow" />
            <el-option label="拒绝 (deny)" value="deny" />
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
        <el-button
          v-if="hasPermission('acls:write')"
          type="primary"
          :loading="submitting"
          @click="handleSubmit"
        >
          确定
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import { useAclApi } from '@/composables/useAclApi'
import { useIpGroupApi } from '@/composables/useIpGroupApi'
import { useTenantApi } from '@/composables/useTenantApi'
import { usePermission } from '@/composables/usePermission'
import { useTenantChangeReload } from '@/composables/useTenantChangeReload'

const { hasPermission } = usePermission()

const loading = ref(false)
const rules = ref([])
const tenantNodes = ref([])
const ipGroups = ref([])
const dialogVisible = ref(false)
const submitting = ref(false)
const formRef = ref(null)

const filters = reactive({
  node_id: '',
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
  node_id: '',
  node_name: '',
  id: null,
  name: '',
  src_group_id: '',
  dst_group_id: '',
  src_cidr: '',
  dst_cidr: '',
  protocol: 6,
  dst_port: 0,
  direction: 'ingress',
  ports: '',
  action: 'allow',
  enabled: true,
  priority: 100,
  description: ''
})

const formRules = {
  node_id: [
    { required: true, message: '请选择目标节点', trigger: 'change' }
  ],
  name: [
    { required: true, message: '请输入规则名称', trigger: 'blur' },
    { min: 1, max: 50, message: '长度在 1 到 50 个字符', trigger: 'blur' }
  ],
  src_cidr: [],
  dst_cidr: [],
  dst_port: [
    { required: true, message: '请输入目标端口', trigger: 'blur' }
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

const selectableGroups = computed(() => ipGroups.value.filter((group) => group.kind !== 'inline'))

const groupById = computed(() => {
  const result = new Map()
  ipGroups.value.forEach((group) => result.set(group.id, group))
  return result
})

const paginatedRules = computed(() => {
  const start = (pagination.page - 1) * pagination.pageSize
  return rules.value.slice(start, start + pagination.pageSize)
})

const loadNodes = async () => {
  try {
    tenantNodes.value = await useTenantApi.getTenantNodes()
    if (!filters.node_id && tenantNodes.value.length > 0) {
      filters.node_id = tenantNodes.value[0].id
      await loadRules()
    } else if (tenantNodes.value.length === 0) {
      filters.node_id = ''
      rules.value = []
      pagination.total = 0
    }
  } catch (error) {
    console.error('加载节点失败:', error)
  }
}

const loadIPGroups = async () => {
  try {
    ipGroups.value = await useIpGroupApi.listIPGroups()
  } catch (error) {
    console.error('加载 IP Group 失败:', error)
    ipGroups.value = []
  }
}

const loadRules = async () => {
  if (!filters.node_id) {
    rules.value = []
    pagination.total = 0
    return
  }

  loading.value = true
  try {
    const f = { ...filters }
    const nodeId = f.node_id
    delete f.node_id
    
    const response = await useAclApi.getACLRulesByNode(nodeId, f)
    rules.value = response
    pagination.total = rules.value.length
  } catch (error) {
    ElMessage.error('加载规则失败: ' + (error.message || '未知错误'))
  } finally {
    loading.value = false
  }
}

const handleNodeChange = () => {
  pagination.page = 1
  loadRules()
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
  Object.assign(form, {
    ...row,
    node_id: row.node_id,
    src_group_id: row.src_group_id || '',
    dst_group_id: row.dst_group_id || '',
    src_cidr: row.src_cidr || row.src_net || '',
    dst_cidr: row.dst_cidr || row.dst_net || '',
    dst_port: row.dst_port ?? row.max_port ?? 0,
    direction: row.direction || 'ingress',
    ports: row.ports || ''
  })
  dialogVisible.value = true
}

const handleDelete = async (row) => {
  try {
    await ElMessageBox.confirm(`确定要删除规则 "${row.name}" 吗？`, '删除确认', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    })
    
    await useAclApi.deleteACLRule(row.id, row.node_id)
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
    await useAclApi.updateACLRule(row.id, { ...row, enabled: row.enabled, node_id: row.node_id })
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
    
    if (data.dst_port < 0 || data.dst_port > 65535) {
      ElMessage.error('目标端口必须在 0 到 65535 之间')
      return
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
    node_id: tenantNodes.value[0]?.id || '',
    node_name: '',
    id: null, name: '', src_group_id: '', dst_group_id: '', src_cidr: '', dst_cidr: '',
    protocol: 6, dst_port: 0, direction: 'ingress', ports: '',
    action: 'allow', enabled: true, priority: 100, description: ''
  })
  if (formRef.value) formRef.value.resetFields()
}

const getProtocolName = (protocol) => {
  const map = { 0: 'Any', 6: 'TCP', 17: 'UDP', 1: 'ICMP' }
  return map[protocol] || 'Unknown'
}

const getActionName = (action) => {
  const map = { allow: '允许', deny: '拒绝' }
  return map[action] || action
}

const getActionType = (action) => {
  const map = { allow: 'success', deny: 'danger' }
  return map[action] || ''
}

const formatDirection = (direction) => {
  const map = { ingress: '入站', egress: '出站', both: '双向' }
  return map[direction] || direction || '入站'
}

const formatPolicyStatus = (status) => {
  const map = {
    applied: '已应用',
    pending: '待下发',
    queued: '排队中',
    sent: '已发送',
    in_progress: '下发中',
    failed: '失败',
    error: '失败',
    idle: '空闲'
  }
  return map[status] || status || '未知'
}

const getPolicyTagType = (status) => {
  const map = {
    applied: 'success',
    pending: 'warning',
    queued: 'warning',
    sent: 'warning',
    in_progress: 'warning',
    failed: 'danger',
    error: 'danger',
    idle: 'info'
  }
  return map[status] || 'info'
}

const formatGroupOption = (group) => {
  const cidrs = Array.isArray(group.members) ? group.members.map((member) => member.cidr).join(', ') : ''
  return cidrs ? `${group.name} (${cidrs})` : group.name
}

const formatGroupRef = (groupId, fallback) => {
  if (groupId) {
    const group = groupById.value.get(groupId)
    if (group) return group.name
    if (fallback && fallback !== groupId) return fallback
    return '未知 IP Group'
  }
  return fallback || 'any'
}

const shortCommandId = (commandId) => {
  if (!commandId) {
    return '-'
  }
  return commandId.slice(0, 8)
}

const formatNumber = (value) => {
  const number = Number(value || 0)
  if (number >= 1000000) return `${(number / 1000000).toFixed(1)}M`
  if (number >= 1000) return `${(number / 1000).toFixed(1)}K`
  return String(number)
}

const formatBytes = (value) => {
  const bytes = Number(value || 0)
  if (bytes >= 1073741824) return `${(bytes / 1073741824).toFixed(1)} GB`
  if (bytes >= 1048576) return `${(bytes / 1048576).toFixed(1)} MB`
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${bytes} B`
}

const reloadTenantScopedData = async () => {
  rules.value = []
  tenantNodes.value = []
  ipGroups.value = []
  filters.node_id = ''
  pagination.page = 1
  pagination.total = 0
  await Promise.all([loadIPGroups(), loadNodes()])
}

onMounted(() => {
  reloadTenantScopedData()
})

useTenantChangeReload(reloadTenantScopedData)
</script>

<style scoped>
.acl-rules-container { padding: 20px; }
.page-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; }
.page-header h2 { margin: 0; font-size: 20px; }
.filter-section { display: flex; gap: 10px; margin-bottom: 20px; flex-wrap: wrap; }
.pagination-section { margin-top: 20px; display: flex; justify-content: flex-end; }
.form-help { font-size: 12px; color: #909399; margin-top: 5px; }
.acl-runtime-cell { display: flex; flex-direction: column; gap: 4px; line-height: 1.25; }
.acl-runtime-cell small { color: var(--aria-text-muted, #8a93a6); }
code { background: #f4f4f5; padding: 2px 4px; border-radius: 4px; color: #cf9236; font-family: monospace; }
</style>
