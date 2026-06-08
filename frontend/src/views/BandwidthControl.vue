<!-- src/views/BandwidthControl.vue -->
<template>
  <div class="bandwidth-control">
    <el-card>
      <template #header>
        <div class="card-header">
          <div class="header-left">
            <h3>带宽控制 (QoS)</h3>
            <el-select
              v-model="selectedNodeId"
              placeholder="选择节点 (必选)"
              style="width: 240px; margin-left: 20px;"
              @change="onNodeChange"
            >
              <el-option
                v-for="node in tenantNodes"
                :key="node.id"
                :label="node.hostname || node.id"
                :value="node.id"
              />
            </el-select>
          </div>
          <div class="header-actions">
            <el-button
              v-if="hasPermission('qos:write')"
              type="primary"
              @click="showAddDialog"
              :disabled="!selectedNodeId"
            >
              <el-icon><Plus /></el-icon>
              添加规则
            </el-button>
            <el-button @click="refreshData">
              <el-icon><Refresh /></el-icon>
              刷新
            </el-button>
          </div>
        </div>
      </template>

      <el-alert
        title="Agent QoS 运行模型"
        type="warning"
        description="每条 QoS 规则直接下发为 group + direction + rate_bps + burst_bytes + mode。Group 可以是 CIDR，也可以是 any；不再按旧分类建模。"
        :closable="false"
        show-icon
        style="margin-bottom: 20px;"
      />

      <el-table :data="rules" stripe v-loading="loading">
        <el-table-column prop="description" label="描述" min-width="160" />
        <el-table-column label="Group" min-width="180">
          <template #default="{ row }">
            <code>{{ row.runtime_group || row.group_cidr || 'any' }}</code>
          </template>
        </el-table-column>
        <el-table-column label="带宽限制" width="150">
          <template #default="{ row }">
            <el-tag type="danger" effect="plain">{{ row.bandwidth_mbps }} Mbps</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="方向/模式" width="150">
          <template #default="{ row }">
            <div class="qos-runtime-cell">
              <el-tag size="small" effect="plain">{{ formatDirection(row.direction) }}</el-tag>
              <span>{{ formatMode(row.mode) }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="Runtime" width="170">
          <template #default="{ row }">
            <div class="qos-runtime-cell">
              <span>{{ formatRate(row.rate_bps) }}</span>
              <small>burst {{ formatBytes(row.burst_bytes) }}</small>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="priority" label="优先级" width="90" />
        <el-table-column label="Stats" width="180">
          <template #default="{ row }">
            <div class="qos-runtime-cell">
              <span>pass {{ formatBytes(row.stats?.passed_bytes) }}</span>
              <small>drop {{ formatBytes(row.stats?.dropped_bytes) }} / shape {{ formatBytes(row.stats?.shaped_bytes) }}</small>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="同步状态" width="120">
          <template #default="{ row }">
            <el-tag size="small" :type="getPolicyTagType(row.policyStatus)">{{ formatPolicyStatus(row.policyStatus) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column v-if="hasPermission('qos:write')" label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <el-button link type="danger" @click="handleDelete(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog
      v-model="dialogVisible"
      title="添加 QoS 规则"
      width="520px"
      @closed="resetForm"
    >
      <el-form :model="form" label-width="120px" ref="formRef" :rules="formRules">
        <el-form-item label="描述" prop="description">
          <el-input v-model="form.description" placeholder="例如: 限制某个网段出站带宽" />
        </el-form-item>

        <el-form-item label="Group" prop="group_cidr">
          <el-input v-model="form.group_cidr" placeholder="例如: 10.0.0.0/24，或 any" />
        </el-form-item>

        <el-form-item label="带宽限制" prop="bandwidth_mbps">
          <el-input-number v-model="form.bandwidth_mbps" :min="1" :max="10000" style="width: 100%" />
          <span class="unit-text">Mbps</span>
        </el-form-item>

        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="方向" prop="direction">
              <el-select v-model="form.direction" style="width: 100%">
                <el-option label="入站 ingress" value="ingress" />
                <el-option label="出站 egress" value="egress" />
                <el-option label="双向 both" value="both" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="模式" prop="mode">
              <el-select v-model="form.mode" style="width: 100%">
                <el-option label="Policing" value="policing" />
                <el-option label="Shaping" value="shaping" />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>

        <el-form-item label="优先级" prop="priority">
          <el-input-number v-model="form.priority" :min="0" :max="255" style="width: 100%" />
        </el-form-item>

        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="Rate bps">
              <el-input-number v-model="form.rate_bps" :min="0" :step="1000000" style="width: 100%" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="Burst bytes">
              <el-input-number v-model="form.burst_bytes" :min="0" :step="1500" style="width: 100%" />
            </el-form-item>
          </el-col>
        </el-row>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button v-if="hasPermission('qos:write')" type="primary" @click="handleSave" :loading="submitting">保存并应用</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { Plus, Refresh } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useQosApi } from '@/composables/useQosApi'
import { useTenantApi } from '@/composables/useTenantApi'
import { usePermission } from '@/composables/usePermission'
import { useTenantChangeReload } from '@/composables/useTenantChangeReload'

const { hasPermission } = usePermission()

const loading = ref(false)
const submitting = ref(false)
const dialogVisible = ref(false)
const selectedNodeId = ref('')
const tenantNodes = ref([])
const rules = ref([])
const formRef = ref(null)

const form = reactive({
  description: '',
  group_cidr: '',
  bandwidth_mbps: 100,
  direction: 'egress',
  rate_bps: 0,
  burst_bytes: 0,
  priority: 100,
  mode: 'policing'
})

const formRules = {
  description: [{ required: true, message: '请输入描述', trigger: 'blur' }],
  group_cidr: [{ required: true, message: '请输入 Group', trigger: 'blur' }],
  bandwidth_mbps: [{ required: true, message: '请设置带宽限制', trigger: 'blur' }]
}

const loadNodes = async () => {
  try {
    tenantNodes.value = await useTenantApi.getTenantNodes()
    if (tenantNodes.value.length > 0) {
      selectedNodeId.value = tenantNodes.value[0].id
      refreshData()
    } else {
      selectedNodeId.value = ''
      rules.value = []
    }
  } catch (error) {
    console.error('加载节点失败:', error)
  }
}

const refreshData = async () => {
  if (!selectedNodeId.value) return

  loading.value = true
  try {
    rules.value = await useQosApi.getQoSRulesByNode(selectedNodeId.value)
  } catch (error) {
    ElMessage.error('获取 QoS 规则失败')
  } finally {
    loading.value = false
  }
}

const onNodeChange = () => {
  refreshData()
}

const getPolicyTagType = (status) => {
  const map = { applied: 'success', pending: 'warning', error: 'danger' }
  return map[status] || 'info'
}

const formatPolicyStatus = (status) => {
  const map = { applied: '已收敛', pending: '同步中', error: '异常' }
  return map[status] || '待同步'
}

const formatDirection = (direction) => {
  const map = { ingress: '入站', egress: '出站', both: '双向' }
  return map[direction] || direction || '出站'
}

const formatMode = (mode) => {
  const map = { policing: 'Policing', shaping: 'Shaping' }
  return map[mode] || mode || 'Policing'
}

const formatRate = (rateBps) => {
  const value = Number(rateBps || 0)
  if (value >= 1000000000) return `${(value / 1000000000).toFixed(2)} Gbps`
  if (value >= 1000000) return `${(value / 1000000).toFixed(1)} Mbps`
  if (value >= 1000) return `${(value / 1000).toFixed(1)} Kbps`
  return `${value} bps`
}

const formatBytes = (bytes) => {
  const value = Number(bytes || 0)
  if (value >= 1048576) return `${(value / 1048576).toFixed(1)} MB`
  if (value >= 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${value} B`
}

const showAddDialog = () => {
  dialogVisible.value = true
}

const resetForm = () => {
  Object.assign(form, {
    description: '',
    group_cidr: '',
    bandwidth_mbps: 100,
    direction: 'egress',
    rate_bps: 0,
    burst_bytes: 0,
    priority: 100,
    mode: 'policing'
  })
  if (formRef.value) formRef.value.resetFields()
}

const handleSave = async () => {
  if (!hasPermission('qos:write')) {
    ElMessage.error('缺少 QoS 管理权限')
    return
  }
  if (!formRef.value) return

  try {
    await formRef.value.validate()
    submitting.value = true

    await useQosApi.createQoSRule(selectedNodeId.value, form)
    ElMessage.success('规则已创建并排队下发')
    dialogVisible.value = false
    refreshData()
  } catch (error) {
    if (error !== false) {
      ElMessage.error('创建失败: ' + (error.message || '未知错误'))
    }
  } finally {
    submitting.value = false
  }
}

const handleDelete = async (row) => {
  if (!hasPermission('qos:write')) {
    ElMessage.error('缺少 QoS 管理权限')
    return
  }

  try {
    await ElMessageBox.confirm('确定要删除此带宽限速规则吗？', '确认删除', {
      type: 'warning',
      confirmButtonText: '确定删除',
      cancelButtonText: '取消'
    })

    await useQosApi.deleteQoSRule(selectedNodeId.value, row.id)
    ElMessage.success('删除指令已下发')
    refreshData()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('删除失败')
    }
  }
}

const reloadTenantScopedData = async () => {
  selectedNodeId.value = ''
  tenantNodes.value = []
  rules.value = []
  await loadNodes()
}

onMounted(() => {
  reloadTenantScopedData()
})

useTenantChangeReload(reloadTenantScopedData)
</script>

<style scoped>
.bandwidth-control { padding: 20px; }
.card-header { display: flex; justify-content: space-between; align-items: center; }
.header-left { display: flex; align-items: center; }
.header-actions { display: flex; gap: 10px; }
.unit-text { margin-left: 10px; color: #606266; }
.qos-runtime-cell { display: flex; flex-direction: column; gap: 4px; line-height: 1.25; }
.qos-runtime-cell small { color: var(--aria-text-muted, #8a93a6); }
code { background: #f4f4f5; padding: 2px 4px; border-radius: 4px; color: #cf9236; font-family: monospace; }
</style>
