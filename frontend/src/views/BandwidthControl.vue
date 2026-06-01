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

      <!-- QoS Priority Info Banner -->
      <el-alert
        title="QoS 优先级架构"
        type="warning"
        description="Aria 采用三层 eBPF 限速：Service (五元组) > Peers (节点对) > IP (单节点)。高优先级规则匹配后，该流量将豁免于低优先级限速。"
        :closable="false"
        show-icon
        style="margin-bottom: 20px;"
      />

      <el-tabs v-model="activeCategory" @tab-change="onCategoryChange">
        <el-tab-pane label="服务级 (Service)" name="service">
          <template #label>
            <span>服务级 <el-badge :value="rules.service.length" class="item" :hidden="!rules.service.length" /></span>
          </template>
          <div class="tab-content">
            <p class="tab-desc">最高优先级：基于五元组（IP+端口+协议）进行精细化限速。</p>
            <el-table :data="rules.service" stripe v-loading="loading">
              <el-table-column prop="description" label="描述" min-width="150" />
              <el-table-column label="匹配条件" min-width="250">
                <template #default="{ row }">
                  <code>{{ row.src_cidr || '*' }}</code>:{{ row.src_port || '*' }} → 
                  <code>{{ row.dst_cidr || '*' }}</code>:{{ row.dst_port }} ({{ getProtocolName(row.protocol) }})
                </template>
              </el-table-column>
              <el-table-column label="带宽限制" width="150">
                <template #default="{ row }">
                  <el-tag type="danger" effect="plain">{{ row.bandwidth_mbps }} Mbps</el-tag>
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
          </div>
        </el-tab-pane>

        <el-tab-pane label="节点对 (Peers)" name="peers">
          <template #label>
            <span>节点对 <el-badge :value="rules.peers.length" class="item" :hidden="!rules.peers.length" /></span>
          </template>
          <div class="tab-content">
            <p class="tab-desc">中优先级：控制两个特定网络节点之间的总通信带宽。</p>
            <el-table :data="rules.peers" stripe v-loading="loading">
              <el-table-column prop="description" label="描述" min-width="150" />
              <el-table-column label="匹配条件" min-width="250">
                <template #default="{ row }">
                  <code>{{ row.src_cidr }}</code> ↔ <code>{{ row.dst_cidr }}</code>
                </template>
              </el-table-column>
              <el-table-column label="带宽限制" width="150">
                <template #default="{ row }">
                  <el-tag type="danger" effect="plain">{{ row.bandwidth_mbps }} Mbps</el-tag>
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
          </div>
        </el-tab-pane>

        <el-tab-pane label="单节点 (IP)" name="ip">
          <template #label>
            <span>单节点 <el-badge :value="rules.ip.length" class="item" :hidden="!rules.ip.length" /></span>
          </template>
          <div class="tab-content">
            <p class="tab-desc">低优先级：为单个节点设置总出口带宽上限（兜底规则）。</p>
            <el-table :data="rules.ip" stripe v-loading="loading">
              <el-table-column prop="description" label="描述" min-width="150" />
              <el-table-column label="目标 IP/网段" min-width="200">
                <template #default="{ row }">
                  <code>{{ row.src_cidr }}</code>
                </template>
              </el-table-column>
              <el-table-column label="带宽限制" width="150">
                <template #default="{ row }">
                  <el-tag type="danger" effect="plain">{{ row.bandwidth_mbps }} Mbps</el-tag>
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
          </div>
        </el-tab-pane>
      </el-tabs>
    </el-card>

    <!-- QoS 规则编辑器 -->
    <el-dialog
      v-model="dialogVisible"
      :title="`添加 ${getCategoryLabel(activeCategory)} 规则`"
      width="550px"
      @closed="resetForm"
    >
      <el-form :model="form" label-width="120px" ref="formRef" :rules="formRules">
        <el-form-item label="描述" prop="description">
          <el-input v-model="form.description" placeholder="例如: 限制数据库备份流量" />
        </el-form-item>

        <el-form-item label="源 CIDR" prop="src_cidr">
          <el-input v-model="form.src_cidr" placeholder="例如: 10.0.0.1/32" />
        </el-form-item>

        <template v-if="activeCategory !== 'ip'">
          <el-form-item label="目标 CIDR" prop="dst_cidr">
            <el-input v-model="form.dst_cidr" placeholder="例如: 10.0.0.2/32" />
          </el-form-item>
        </template>

        <template v-if="activeCategory === 'service'">
          <el-form-item label="目标端口" prop="dst_port">
            <el-input-number v-model="form.dst_port" :min="1" :max="65535" style="width: 100%" />
          </el-form-item>
          <el-form-item label="协议">
            <el-select v-model="form.protocol" style="width: 100%">
              <el-option label="TCP" :value="6" />
              <el-option label="UDP" :value="17" />
              <el-option label="Any" :value="0" />
            </el-select>
          </el-form-item>
        </template>

        <el-form-item label="带宽限制" prop="bandwidth_mbps">
          <el-input-number v-model="form.bandwidth_mbps" :min="1" :max="10000" style="width: 100%" />
          <span class="unit-text">Mbps</span>
        </el-form-item>
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

const { hasPermission } = usePermission()

// 状态变量
const loading = ref(false)
const submitting = ref(false)
const dialogVisible = ref(false)
const selectedNodeId = ref('')
const activeCategory = ref('service')
const tenantNodes = ref([])
const formRef = ref(null)

const rules = reactive({
  service: [],
  peers: [],
  ip: []
})

const form = reactive({
  description: '',
  src_cidr: '',
  dst_cidr: '',
  dst_port: 80,
  protocol: 6,
  bandwidth_mbps: 100
})

const formRules = {
  description: [{ required: true, message: '请输入描述', trigger: 'blur' }],
  src_cidr: [{ required: true, message: '请输入源网段', trigger: 'blur' }],
  dst_cidr: [{ required: true, message: '请输入目标网段', trigger: 'blur' }],
  bandwidth_mbps: [{ required: true, message: '请设置带宽限制', trigger: 'blur' }]
}

// 获取节点列表
const loadNodes = async () => {
  try {
    tenantNodes.value = await useTenantApi.getTenantNodes()
    if (tenantNodes.value.length > 0) {
      selectedNodeId.value = tenantNodes.value[0].id
      refreshData()
    }
  } catch (error) {
    console.error('加载节点失败:', error)
  }
}

// 刷新当前分类的数据
const refreshData = async () => {
  if (!selectedNodeId.value) return
  
  loading.value = true
  try {
    const data = await useQosApi.getQoSRulesByNode(selectedNodeId.value, activeCategory.value)
    rules[activeCategory.value] = data
  } catch (error) {
    ElMessage.error(`获取 ${activeCategory.value} 规则失败`)
  } finally {
    loading.value = false
  }
}

// 切换节点
const onNodeChange = () => {
  refreshData()
}

// 切换分类
const onCategoryChange = () => {
  if (rules[activeCategory.value].length === 0) {
    refreshData()
  }
}

const getCategoryLabel = (cat) => {
  const map = { service: '服务级', peers: '节点对', ip: '单节点' }
  return map[cat] || cat
}

const getProtocolName = (p) => useQosApi.getProtocolName(p)

const getPolicyTagType = (status) => {
  const map = { applied: 'success', pending: 'warning', error: 'danger' }
  return map[status] || 'info'
}

const formatPolicyStatus = (status) => {
  const map = { applied: '已收敛', pending: '同步中', error: '异常' }
  return map[status] || '待同步'
}

const showAddDialog = () => {
  dialogVisible.value = true
}

const resetForm = () => {
  Object.assign(form, {
    description: '',
    src_cidr: '',
    dst_cidr: '',
    dst_port: 80,
    protocol: 6,
    bandwidth_mbps: 100
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
    
    await useQosApi.createQoSRule(selectedNodeId.value, activeCategory.value, form)
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
    
    await useQosApi.deleteQoSRule(selectedNodeId.value, activeCategory.value, row.id)
    ElMessage.success('删除指令已下发')
    refreshData()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('删除失败')
    }
  }
}

onMounted(() => {
  loadNodes()
})
</script>

<style scoped>
.bandwidth-control { padding: 20px; }
.card-header { display: flex; justify-content: space-between; align-items: center; }
.header-left { display: flex; align-items: center; }
.header-actions { display: flex; gap: 10px; }
.tab-content { padding: 10px 0; }
.tab-desc { color: #909399; font-size: 13px; margin-bottom: 20px; }
.unit-text { margin-left: 10px; color: #606266; }
code { background: #f4f4f5; padding: 2px 4px; border-radius: 4px; color: #cf9236; font-family: monospace; }
</style>
