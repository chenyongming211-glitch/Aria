<!-- src/views/PolicyManagement.vue -->
<template>
  <div class="policy-management">
    <el-card>
      <template #header>
        <div class="card-header">
          <h3>策略管理</h3>
          <div class="header-actions">
            <el-button type="primary" @click="showAddDialog">
              <el-icon><Plus /></el-icon>
              添加策略
            </el-button>
            <el-button @click="refreshData">
              <el-icon><Refresh /></el-icon>
              刷新
            </el-button>
            <el-button type="success" @click="applyAllPolicies">
              <el-icon><VideoPlay /></el-icon>
              应用策略
            </el-button>
            <el-button type="danger" @click="clearAllPolicies">
              <el-icon><Delete /></el-icon>
              清空策略
            </el-button>
          </div>
        </div>
      </template>

      <!-- Security Policy Architecture Banner -->
      <el-alert
        title="安全策略架构说明"
        type="info"
        description="Aria 采用“三维立体防护”架构，将网络安全策略细分为 入站防御、入站允许 与 出站管控 三个核心场景，构建从网络边缘到业务内部的全链路安全闭环。"
        :closable="false"
        style="margin-bottom: 20px;"
      />

      <el-tabs v-model="activeTab" @tab-change="onTabChange">
        <el-tab-pane label="策略列表" name="policies">
          <el-table
            :data="currentPolicies"
            stripe
            style="width: 100%"
            v-loading="loading"
          >
            <el-table-column prop="id" label="ID" width="100" />
            <el-table-column prop="type" label="类型" width="150">
              <template #default="{ row }">
                <el-tag :type="getPolicyTypeColor(row.type)">
                  {{ getPolicyTypeName(row.type) }}
                  <el-tooltip effect="dark" :content="getPolicyTypeDescription(row.type)" placement="top">
                    <el-icon style="margin-left: 5px;"><QuestionFilled /></el-icon>
                  </el-tooltip>
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="name" label="名称" width="200" />
            <el-table-column label="详情" min-width="300">
              <template #default="{ row }">
                <div v-if="row.type === 'inbound-def'">
                  <strong>拦截:</strong> {{ row.sourceIp }} → {{ row.targetPort || 'Any Port' }}
                  <el-tag size="small" type="danger">拒绝</el-tag>
                </div>
                <div v-else-if="row.type === 'inbound-allow'">
                  <strong>放行:</strong> {{ row.sourceIp }} → {{ row.targetPort }}
                  <el-tag size="small" type="success">允许</el-tag>
                </div>
                <div v-else-if="row.type === 'outbound'">
                  <strong>管控:</strong> {{ row.sourceIp || 'Any' }} → {{ row.destinationIp }}
                  <el-tag size="small" type="warning">出站控制</el-tag>
                </div>
              </template>
            </el-table-column>
            <el-table-column prop="action" label="动作" width="100">
              <template #default="{ row }">
                <el-tag :type="getActionType(row.action)">
                  {{ getActionName(row.action) }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="priority" label="优先级" width="100" />
            <el-table-column prop="status" label="状态" width="100">
              <template #default="{ row }">
                <el-tag :type="getStatusType(row.status)">
                  {{ row.status === 'active' ? '启用' : '禁用' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="下发状态" width="120">
              <template #default="{ row }">
                <el-tag size="small" :type="getPolicyDeliveryTagType(row.policyStatus)">
                  {{ formatPolicyDeliveryStatus(row.policyStatus) }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="pendingCmds" label="待执行" width="90" />
            <el-table-column label="最近命令" width="150">
              <template #default="{ row }">
                <el-tooltip
                  v-if="row.lastDeliveryCommandId"
                  :content="row.lastDeliveryCommandId"
                  placement="top"
                >
                  <span>{{ shortCommandId(row.lastDeliveryCommandId) }}</span>
                </el-tooltip>
                <span v-else>-</span>
              </template>
            </el-table-column>
            <el-table-column prop="lastCommandError" label="失败原因" min-width="180" show-overflow-tooltip />
            <el-table-column label="操作" width="250">
              <template #default="{ row }">
                <el-button size="small" @click="editPolicy(row)">编辑</el-button>
                <el-button size="small" type="primary" @click="viewPolicy(row)">查看</el-button>
                <el-popconfirm
                  :title="`确定要删除此${getPolicyTypeName(row.type)}策略吗？`"
                  @confirm="deletePolicy(row.id)"
                >
                  <template #reference>
                    <el-button size="small" type="danger">删除</el-button>
                  </template>
                </el-popconfirm>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>

        <el-tab-pane label="场景说明" name="scenarios">
          <div class="scenarios-container">
            <el-row :gutter="20">
              <el-col :span="8">
                <el-card class="scenario-card" shadow="hover">
                  <div class="scenario-header">
                    <el-tag type="danger" size="large">入站防御</el-tag>
                    <div class="priority-badge danger">第一道防线</div>
                  </div>
                  <div class="scenario-content">
                    <h4>入站防御 (Inbound Defense)</h4>
                    <p>"将恶意流量拒之门外"</p>
                    <ul>
                      <li>抗DDoS攻击：配置防御策略直接丢弃攻击源IP的流量，保障节点CPU不被打满</li>
                      <li>高危端口封禁：屏蔽TCP 445(SMB)、135(RPC)、UDP 11211(Memcached)等高危端口</li>
                      <li>恶意扫描阻断：自动联动威胁情报，将扫描全网的恶意IP地址加入防御列表</li>
                    </ul>
                  </div>
                </el-card>
              </el-col>

              <el-col :span="8">
                <el-card class="scenario-card" shadow="hover">
                  <div class="scenario-header">
                    <el-tag type="success" size="large">入站允许</el-tag>
                    <div class="priority-badge success">第二道防线</div>
                  </div>
                  <div class="scenario-content">
                    <h4>入站允许 (Inbound Allow)</h4>
                    <p>"仅放行合法的业务流量"</p>
                    <ul>
                      <li>业务端口开放：仅允许公网访问Web服务端口(如80/443)，其余端口保持关闭</li>
                      <li>管理通道加固：限制SSH(22)或RDP(3389)端口，仅允许特定IP连接</li>
                      <li>微隔离：在多租户环境下，仅允许指定IP间的特定端口访问</li>
                    </ul>
                  </div>
                </el-card>
              </el-col>

              <el-col :span="8">
                <el-card class="scenario-card" shadow="hover">
                  <div class="scenario-header">
                    <el-tag type="warning" size="large">出站管控</el-tag>
                    <div class="priority-badge warning">第三道防线</div>
                  </div>
                  <div class="scenario-content">
                    <h4>出站管控 (Outbound Control)</h4>
                    <p>"防止数据外泄与非法连接"</p>
                    <ul>
                      <li>防止反弹Shell：阻断被入侵服务器主动连接黑客控制端(C&C Server)</li>
                      <li>阻断挖矿木马：阻止挖矿病毒连接外部矿池下载脚本或上传算力</li>
                      <li>数据防泄漏：禁止数据库服务器主动连接公网，确保数据内网流转</li>
                    </ul>
                  </div>
                </el-card>
              </el-col>
            </el-row>

            <el-card style="margin-top: 20px;">
              <template #header>
                <div class="card-header">
                  <span>流量处理流水线</span>
                </div>
              </template>
              <div class="pipeline-description">
                <p><strong>当一个数据包试图访问您的服务器时，它将经历以下旅程：</strong></p>
                <ol>
                  <li><strong>第一关：入站防御</strong> → 是黑名单吗？ → 是，直接丢弃 (DROP)</li>
                  <li><strong>第二关：入站允许</strong> → 是白名单吗？ → 是，允许进入 (ACCEPT)
                    <br /><span class="sub-text">（注：如果不属于黑名单，也不属于白名单，则会被默认策略拒绝）</span>
                  </li>
                  <li><strong>第三关：业务处理</strong> → 应用程序处理请求并准备回应</li>
                  <li><strong>第四关：出站管控</strong> → 试图发起的连接合法吗？ → 是，允许发送；否，拦截</li>
                </ol>
              </div>
            </el-card>
          </div>
        </el-tab-pane>

        <el-tab-pane label="统计" name="stats">
          <div class="stats-container">
            <el-row :gutter="20">
              <el-col :span="6">
                <el-card class="stat-card">
                  <div class="stat-content">
                    <div class="stat-value">{{ stats.totalPolicies }}</div>
                    <div class="stat-label">策略</div>
                  </div>
                </el-card>
              </el-col>
              <el-col :span="6">
                <el-card class="stat-card">
                  <div class="stat-content">
                    <div class="stat-value">{{ stats.inboundDefPolicies }}</div>
                    <div class="stat-label">入站防御策略</div>
                  </div>
                </el-card>
              </el-col>
              <el-col :span="6">
                <el-card class="stat-card">
                  <div class="stat-content">
                    <div class="stat-value">{{ stats.inboundAllowPolicies }}</div>
                    <div class="stat-label">入站允许策略</div>
                  </div>
                </el-card>
              </el-col>
              <el-col :span="6">
                <el-card class="stat-card">
                  <div class="stat-content">
                    <div class="stat-value">{{ stats.outboundPolicies }}</div>
                    <div class="stat-label">出站管控策略</div>
                  </div>
                </el-card>
              </el-col>
            </el-row>

            <el-row :gutter="20" style="margin-top: 20px;">
              <el-col :span="12">
                <el-card class="chart-card">
                  <template #header>
                    <div class="card-header">
                      <span>策略分布</span>
                    </div>
                  </template>
                  <div id="policy-chart" style="height: 300px;"></div>
                </el-card>
              </el-col>
              <el-col :span="12">
                <el-card class="chart-card">
                  <template #header>
                    <div class="card-header">
                      <span>策略统计</span>
                    </div>
                  </template>
                  <div id="policy-stats-chart" style="height: 300px;"></div>
                </el-card>
              </el-col>
            </el-row>
          </div>
        </el-tab-pane>
      </el-tabs>
    </el-card>

    <!-- Policy Editor Dialog -->
    <el-dialog
      v-model="policyDialogVisible"
      :title="editingPolicy.id ? '编辑 ' + getPolicyTypeName(editingPolicy.type) + ' 策略' : '添加 ' + getPolicyTypeName(editingPolicy.type) + ' 策略'"
      width="60%"
    >
      <el-form
        v-if="editingPolicy"
        :model="editingPolicy"
        label-width="150px"
      >
        <el-form-item label="类型">
          <el-radio-group v-model="editingPolicy.type" @change="onPolicyTypeChange">
            <el-radio label="inbound-def">
              入站防御 (Inbound Defense)
              <el-tooltip effect="dark" content="针对已知的恶意IP、高频攻击源或非业务端口的流量，在网卡驱动层直接进行清洗和拦截" placement="top">
                <el-icon><QuestionFilled style="margin-left: 5px;" /></el-icon>
              </el-tooltip>
            </el-radio>
            <el-radio label="inbound-allow">
              入站允许 (Inbound Allow)
              <el-tooltip effect="dark" content="基于零信任原则，仅对策略中明确定义的白名单流量予以放行" placement="top">
                <el-icon><QuestionFilled style="margin-left: 5px;" /></el-icon>
              </el-tooltip>
            </el-radio>
            <el-radio label="outbound">
              出站管控 (Outbound Control)
              <el-tooltip effect="dark" content="针对服务器内部程序向外部网络发起的主动连接进行约束" placement="top">
                <el-icon><QuestionFilled style="margin-left: 5px;" /></el-icon>
              </el-tooltip>
            </el-radio>
          </el-radio-group>
        </el-form-item>

        <el-form-item label="名称">
          <el-input v-model="editingPolicy.name" />
        </el-form-item>

        <!-- Inbound Defense specific fields -->
        <template v-if="editingPolicy.type === 'inbound-def'">
          <el-form-item label="源 IP">
            <el-input v-model="editingPolicy.sourceIp" placeholder="e.g. 192.168.1.100 or 192.168.1.0/24" />
          </el-form-item>
          <el-form-item label="目标端口">
            <el-input v-model="editingPolicy.targetPort" placeholder="e.g. 80, 443, 22 or Any" />
          </el-form-item>
          <el-form-item label="协议">
            <el-select v-model="editingPolicy.protocol" placeholder="选择协议">
              <el-option label="TCP" value="tcp" />
              <el-option label="UDP" value="udp" />
              <el-option label="ICMP" value="icmp" />
              <el-option label="Any" value="any" />
            </el-select>
          </el-form-item>
        </template>

        <!-- Inbound Allow specific fields -->
        <template v-if="editingPolicy.type === 'inbound-allow'">
          <el-form-item label="源 IP">
            <el-input v-model="editingPolicy.sourceIp" placeholder="e.g. 10.0.0.0/8 or 192.168.1.100" />
          </el-form-item>
          <el-form-item label="目标端口">
            <el-input v-model="editingPolicy.targetPort" placeholder="e.g. 80, 443, 22" required />
          </el-form-item>
          <el-form-item label="协议">
            <el-select v-model="editingPolicy.protocol" placeholder="选择协议">
              <el-option label="TCP" value="tcp" />
              <el-option label="UDP" value="udp" />
              <el-option label="ICMP" value="icmp" />
              <el-option label="Any" value="any" />
            </el-select>
          </el-form-item>
        </template>

        <!-- Outbound specific fields -->
        <template v-if="editingPolicy.type === 'outbound'">
          <el-form-item label="源 IP">
            <el-input v-model="editingPolicy.sourceIp" placeholder="e.g. 192.168.1.100 or Any (leave empty)" />
          </el-form-item>
          <el-form-item label="目标 IP">
            <el-input v-model="editingPolicy.destinationIp" placeholder="e.g. 8.8.8.8 or 1.1.1.0/24" required />
          </el-form-item>
          <el-form-item label="目标端口">
            <el-input v-model="editingPolicy.targetPort" placeholder="e.g. 443, 80, 53" />
          </el-form-item>
          <el-form-item label="协议">
            <el-select v-model="editingPolicy.protocol" placeholder="选择协议">
              <el-option label="TCP" value="tcp" />
              <el-option label="UDP" value="udp" />
              <el-option label="ICMP" value="icmp" />
              <el-option label="Any" value="any" />
            </el-select>
          </el-form-item>
        </template>

        <el-form-item label="动作">
          <el-select v-model="editingPolicy.action" placeholder="选择动作">
            <el-option
              v-if="editingPolicy.type === 'inbound-allow'"
              label="允许"
              value="allow"
            />
            <el-option
              v-if="editingPolicy.type === 'inbound-def'"
              label="拒绝"
              value="deny"
            />
            <el-option
              v-if="editingPolicy.type === 'inbound-def'"
              label="丢弃"
              value="drop"
            />
            <el-option
              v-if="editingPolicy.type === 'outbound'"
              label="允许"
              value="allow"
            />
            <el-option
              v-if="editingPolicy.type === 'outbound'"
              label="拒绝"
              value="deny"
            />
          </el-select>
        </el-form-item>

        <el-form-item label="优先级">
          <el-input-number
            v-model="editingPolicy.priority"
            :min="1"
            :max="100"
            placeholder="优先级数值（1-100，数值越小优先级越高）"
          />
        </el-form-item>

        <el-form-item label="最大带宽 (Mbps)">
          <el-input-number
            v-model="editingPolicy.bandwidth"
            :min="1"
            :max="10000"
            :step="1"
            placeholder="输入最大带宽，单位 Mbps"
          />
        </el-form-item>

        <el-form-item label="状态">
          <el-select v-model="editingPolicy.status" placeholder="选择状态">
            <el-option label="启用" value="active" />
            <el-option label="禁用" value="inactive" />
          </el-select>
        </el-form-item>

        <!-- Policy explanation -->
        <el-form-item>
          <el-alert
            :title="getPolicyExplanation(editingPolicy.type)"
            type="info"
            :closable="false"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="policyDialogVisible = false">取消</el-button>
          <el-button type="primary" @click="savePolicy">保存</el-button>
        </span>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted, nextTick, onUnmounted } from 'vue'
import { Plus, Refresh, QuestionFilled, VideoPlay, Delete } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useQosApi } from '@/composables/useQosApi'
import { API_ENDPOINTS } from '@/config/api'

// Policy types mapping
const policyTypes = {
  'inbound-def': '入站防御',
  'inbound-allow': '入站允许',
  outbound: '出站管控'
}

// Policy type descriptions
const policyTypeDescriptions = {
  'inbound-def': '针对已知的恶意IP、高频攻击源或非业务端口的流量，在网卡驱动层(XDP)直接进行清洗和拦截',
  'inbound-allow': '基于零信任原则，仅对策略中明确定义的白名单流量予以放行',
  outbound: '针对服务器内部程序向外部网络发起的主动连接进行约束'
}

// All policies combined
const allPolicies = ref([])

// Stats data
const stats = ref({
  totalPolicies: 0,
  inboundDefPolicies: 0,
  inboundAllowPolicies: 0,
  outboundPolicies: 0,
  totalBandwidth: 0,
  utilization: 0
})

// Reactive variables
const loading = ref(false)
const activeTab = ref('policies')
const policyDialogVisible = ref(false)
const editingPolicy = ref({
  type: 'inbound-def',
  name: '',
  sourceIp: '',
  destinationIp: '',
  targetPort: '',
  protocol: 'tcp',
  action: 'deny',
  priority: 50,
  bandwidth: 100,
  status: 'active'
})

// Current policies based on active tab
const currentPolicies = ref([])

// Chart instances (will be initialized later)
let policyChart = null
let policyStatsChart = null

// Get policy type color
const getPolicyTypeColor = (type) => {
  switch(type) {
    case 'inbound-def': return 'danger' // red for defense
    case 'inbound-allow': return 'success' // green for allow
    case 'outbound': return 'warning' // yellow for outbound control
    default: return 'info'
  }
}

// Get policy type name
const getPolicyTypeName = (type) => {
  return policyTypes[type] || type
}

// Get policy type description
const getPolicyTypeDescription = (type) => {
  return policyTypeDescriptions[type] || ''
}

// Get action type for tag coloring
const getActionType = (action) => {
  switch(action) {
    case 'allow': return 'success'
    case 'deny': return 'danger'
    case 'drop': return 'info'
    default: return 'info'
  }
}

// Get action name
const getActionName = (action) => {
  const names = {
    allow: '允许',
    deny: '拒绝',
    drop: '丢弃'
  }
  return names[action] || action
}

// Get status type for tag coloring
const getStatusType = (status) => {
  switch(status) {
    case 'active': return 'success'
    case 'inactive': return 'info'
    default: return 'info'
  }
}

const formatPolicyDeliveryStatus = (status) => {
  const names = {
    applied: '已应用',
    pending: '待下发',
    in_progress: '下发中',
    error: '失败',
    idle: '空闲'
  }
  return names[status] || status || '未知'
}

const getPolicyDeliveryTagType = (status) => {
  switch (status) {
    case 'applied': return 'success'
    case 'pending':
    case 'in_progress': return 'warning'
    case 'error': return 'danger'
    default: return 'info'
  }
}

const shortCommandId = (commandId) => {
  if (!commandId) {
    return '-'
  }
  return commandId.slice(0, 8)
}

// Get policy explanation
const getPolicyExplanation = (type) => {
  const explanations = {
    'inbound-def': '入站防御策略在网卡驱动层(XDP)直接处理流量，是最高效的第一道防线',
    'inbound-allow': '入站允许策略基于零信任原则，默认拒绝所有访问，仅放行白名单流量',
    outbound: '出站管控策略防止服务器主动连接外部非法地址，有效阻止数据泄露和恶意连接'
  }
  return explanations[type] || '策略说明'
}

// Initialize charts
const initCharts = async () => {
  await nextTick() // Wait for DOM to be ready

  // Only import echarts when needed to reduce bundle size
  const echarts = await import('echarts')

  // Policy distribution chart
  const policyChartDom = document.getElementById('policy-chart')
  if (policyChartDom) {
    if (policyChart) {
      policyChart.dispose() // Dispose existing chart if any
    }
    policyChart = echarts.init(policyChartDom)
    const policyOption = {
      title: {
        text: '策略分布',
        left: 'center'
      },
      tooltip: {
        trigger: 'item'
      },
      legend: {
        orient: 'vertical',
        left: 'left'
      },
      series: [
        {
          name: '策略类型',
          type: 'pie',
          radius: '50%',
          data: [
            { value: stats.value.inboundDefPolicies, name: '入站防御' },
            { value: stats.value.inboundAllowPolicies, name: '入站允许' },
            { value: stats.value.outboundPolicies, name: '出站管控' }
          ],
          emphasis: {
            itemStyle: {
              shadowBlur: 10,
              shadowOffsetX: 0,
              shadowColor: 'rgba(0, 0, 0, 0.5)'
            }
          }
        }
      ]
    }
    policyChart.setOption(policyOption)
  }

  // Policy statistics chart
  const policyStatsChartDom = document.getElementById('policy-stats-chart')
  if (policyStatsChartDom) {
    if (policyStatsChart) {
      policyStatsChart.dispose() // Dispose existing chart if any
    }
    policyStatsChart = echarts.init(policyStatsChartDom)
    const policyStatsOption = {
      title: {
        text: '策略统计',
        left: 'center'
      },
      tooltip: {
        trigger: 'axis',
        axisPointer: {
          type: 'shadow'
        }
      },
      xAxis: {
        type: 'category',
        data: ['入站防御', '入站允许', '出站管控']
      },
      yAxis: {
        type: 'value',
        name: '策略数量'
      },
      series: [
        {
          name: '策略数量',
          type: 'bar',
          data: [
            stats.value.inboundDefPolicies,
            stats.value.inboundAllowPolicies,
            stats.value.outboundPolicies
          ]
        }
      ]
    }
    policyStatsChart.setOption(policyStatsOption)
  }
}

// Update chart data
const updateCharts = () => {
  // Update stats based on current policies
  stats.value.inboundDefPolicies = allPolicies.value.filter(policy => policy.type === 'inbound-def').length
  stats.value.inboundAllowPolicies = allPolicies.value.filter(policy => policy.type === 'inbound-allow').length
  stats.value.outboundPolicies = allPolicies.value.filter(policy => policy.type === 'outbound').length
  stats.value.totalPolicies = allPolicies.value.length

  // Calculate total bandwidth
  stats.value.totalBandwidth = allPolicies.value.reduce((sum, policy) => sum + policy.bandwidth, 0)

  // Update charts if they exist
  if (policyChart) {
    policyChart.setOption({
      series: [{
        data: [
          { value: stats.value.inboundDefPolicies, name: '入站防御' },
          { value: stats.value.inboundAllowPolicies, name: '入站允许' },
          { value: stats.value.outboundPolicies, name: '出站管控' }
        ]
      }]
    })
  }

  if (policyStatsChart) {
    policyStatsChart.setOption({
      series: [{
        data: [
          stats.value.inboundDefPolicies,
          stats.value.inboundAllowPolicies,
          stats.value.outboundPolicies
        ]
      }]
    })
  }
}

// Fetch all policies from API
const fetchPolicies = async () => {
  try {
    loading.value = true
    // Get all policies from the API (mock for now)
    // const response = await useQosApi.getAllPolicies()
    // allPolicies.value = response

    // For demo purposes, initialize with some sample data
    allPolicies.value = [
      {
        id: 'inbound-def-1',
        type: 'inbound-def',
        name: 'DDoS攻击防御',
        sourceIp: '104.28.29.0/24',
        targetPort: 'Any',
        protocol: 'tcp',
        action: 'drop',
        priority: 10,
        bandwidth: 100,
        status: 'active'
      },
      {
        id: 'inbound-allow-1',
        type: 'inbound-allow',
        name: 'Web服务开放',
        sourceIp: '0.0.0.0/0',
        targetPort: '80,443',
        protocol: 'tcp',
        action: 'allow',
        priority: 20,
        bandwidth: 1000,
        status: 'active'
      },
      {
        id: 'outbound-1',
        type: 'outbound',
        name: '防数据外泄',
        sourceIp: '192.168.1.100',
        destinationIp: '8.8.8.8',
        targetPort: '53',
        protocol: 'udp',
        action: 'allow',
        priority: 30,
        bandwidth: 10,
        status: 'active'
      },
      {
        id: 'inbound-def-2',
        type: 'inbound-def',
        name: '高危端口封禁',
        sourceIp: 'Any',
        targetPort: '445,135,139',
        protocol: 'tcp',
        action: 'deny',
        priority: 5,
        bandwidth: 100,
        status: 'active'
      },
      {
        id: 'inbound-allow-2',
        type: 'inbound-allow',
        name: 'SSH管理通道',
        sourceIp: '10.0.0.0/8',
        targetPort: '22',
        protocol: 'tcp',
        action: 'allow',
        priority: 15,
        bandwidth: 10,
        status: 'active'
      }
    ]

    updateCurrentPolicies()
    updateCharts()
    ElMessage.success('策略数据加载成功')
  } catch (error) {
    console.error('Failed to fetch policies:', error)
    ElMessage.error(`获取策略失败: ${error.message}`)

    // For demo purposes, initialize with some sample data
    allPolicies.value = [
      {
        id: 'inbound-def-1',
        type: 'inbound-def',
        name: 'DDoS攻击防御',
        sourceIp: '104.28.29.0/24',
        targetPort: 'Any',
        protocol: 'tcp',
        action: 'drop',
        priority: 10,
        bandwidth: 100,
        status: 'active'
      },
      {
        id: 'inbound-allow-1',
        type: 'inbound-allow',
        name: 'Web服务开放',
        sourceIp: '0.0.0.0/0',
        targetPort: '80,443',
        protocol: 'tcp',
        action: 'allow',
        priority: 20,
        bandwidth: 1000,
        status: 'active'
      },
      {
        id: 'outbound-1',
        type: 'outbound',
        name: '防数据外泄',
        sourceIp: '192.168.1.100',
        destinationIp: '8.8.8.8',
        targetPort: '53',
        protocol: 'udp',
        action: 'allow',
        priority: 30,
        bandwidth: 10,
        status: 'active'
      }
    ]

    updateCurrentPolicies()
    updateCharts()
  } finally {
    loading.value = false
  }
}

// Refresh data
const refreshData = async () => {
  await fetchPolicies()
}

// Show add dialog
const showAddDialog = () => {
  editingPolicy.value = {
    type: 'inbound-def',
    name: '',
    sourceIp: '',
    destinationIp: '',
    targetPort: '',
    protocol: 'tcp',
    action: 'deny',
    priority: 50,
    bandwidth: 100,
    status: 'active'
  }
  policyDialogVisible.value = true
}

// On policy type change
const onPolicyTypeChange = (type) => {
  // Reset fields specific to other types when changing
  if (type !== 'inbound-def') {
    editingPolicy.value.sourceIp = ''
    editingPolicy.value.targetPort = ''
    editingPolicy.value.protocol = 'tcp'
    editingPolicy.value.action = 'deny'
  }
  if (type !== 'inbound-allow') {
    editingPolicy.value.sourceIp = type === 'inbound-def' ? editingPolicy.value.sourceIp : ''
    editingPolicy.value.targetPort = ''
    editingPolicy.value.protocol = 'tcp'
    editingPolicy.value.action = 'allow'
  }
  if (type !== 'outbound') {
    editingPolicy.value.destinationIp = ''
    editingPolicy.value.sourceIp = type === 'inbound-def' || type === 'inbound-allow' ? editingPolicy.value.sourceIp : ''
  }
}

// Edit policy
const editPolicy = (policy) => {
  editingPolicy.value = { ...policy }
  policyDialogVisible.value = true
}

// View policy
const viewPolicy = (policy) => {
  editingPolicy.value = { ...policy }
  policyDialogVisible.value = true
  // Could disable editing in view mode
}

// Validate policy before saving
const validatePolicy = (policy) => {
  if (!policy.name) {
    ElMessage.error('策略名称不能为空')
    return false
  }

  switch(policy.type) {
    case 'inbound-def':
      if (!policy.sourceIp) {
        ElMessage.error('入站防御策略必须填写源IP')
        return false
      }
      break
    case 'inbound-allow':
      if (!policy.sourceIp) {
        ElMessage.error('入站允许策略必须填写源IP')
        return false
      }
      if (!policy.targetPort) {
        ElMessage.error('入站允许策略必须填写目标端口')
        return false
      }
      break
    case 'outbound':
      if (!policy.destinationIp) {
        ElMessage.error('出站管控策略必须填写目标IP')
        return false
      }
      break
  }

  return true
}

// Save policy
const savePolicy = async () => {
  if (!validatePolicy(editingPolicy.value)) {
    return
  }

  try {
    if (editingPolicy.value.id) {
      // Update existing policy
      let updateResult;

      switch(editingPolicy.value.type) {
        case 'inbound-def':
        case 'inbound-allow':
          // Both inbound types use the same endpoint (we're grouping them as firewall rules)
          updateResult = await useQosApi.updateServiceRule(editingPolicy.value.id, editingPolicy.value)
          break
        case 'outbound':
          updateResult = await useQosApi.updateIpRule(editingPolicy.value.id, editingPolicy.value)
          break
      }

      const index = allPolicies.value.findIndex(r => r.id === editingPolicy.value.id)
      if (index !== -1) {
        allPolicies.value[index] = { ...editingPolicy.value }
      }
      ElMessage.success(`${policyTypes[editingPolicy.value.type]}策略已更新`)
    } else {
      // Create new policy
      let createResult;

      switch(editingPolicy.value.type) {
        case 'inbound-def':
        case 'inbound-allow':
          // Both inbound types use the same endpoint
          createResult = await useQosApi.createServiceRule(editingPolicy.value)
          break
        case 'outbound':
          createResult = await useQosApi.createIpRule(editingPolicy.value)
          break
      }

      const newId = createResult?.id || `${editingPolicy.value.type}-${Date.now()}`
      allPolicies.value.push({
        ...editingPolicy.value,
        id: newId
      })
      ElMessage.success(`${policyTypes[editingPolicy.value.type]}策略已创建`)
    }

    // Update current view
    updateCurrentPolicies()
    updateCharts()
    policyDialogVisible.value = false
  } catch (error) {
    console.error('Error saving policy:', error)
    ElMessage.error(`保存策略失败: ${error.message || error}`)
  }
}

// Apply all policies
const applyAllPolicies = async () => {
  try {
    loading.value = true
    // Call API to activate all policies
    const response = await useQosApi.applyAllRules()

    if (response.success) {
      ElMessage.success('所有策略已成功应用到系统')
    } else {
      ElMessage.warning('部分策略可能存在冲突，请检查配置')
    }
  } catch (error) {
    console.error('Error applying policies:', error)
    ElMessage.error(`应用策略失败: ${error.message || error}`)
  } finally {
    loading.value = false
  }
}

// Clear all policies
const clearAllPolicies = async () => {
  try {
    await ElMessageBox.confirm(
      '确定要清空所有策略吗？此操作不可撤销。',
      '警告',
      {
        confirmButtonText: '确定清空',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )

    // Call API to clear all policies
    await useQosApi.clearAllRules()

    // Clear local data
    allPolicies.value = []
    updateCurrentPolicies()
    updateCharts()
    ElMessage.success('所有策略已清空')
  } catch (error) {
    if (error !== 'cancel') {
      console.error('Error clearing policies:', error)
      ElMessage.error(`清空策略失败: ${error.message || error}`)
    }
  }
}

// Delete policy
const deletePolicy = async (id) => {
  try {
    await ElMessageBox.confirm(
      '确定要删除此策略吗？此操作不可撤销。',
      '警告',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )

    // Find the policy to get its type for success message
    const policyToDelete = allPolicies.value.find(policy => policy.id === id)
    const policyType = policyToDelete ? policyTypes[policyToDelete.type] : '未知'

    // Remove from API (using appropriate endpoint based on type)
    switch(policyToDelete.type) {
      case 'inbound-def':
      case 'inbound-allow':
        await useQosApi.deleteServiceRule(id)
        break
      case 'outbound':
        await useQosApi.deleteIpRule(id)
        break
    }

    // Remove from local data
    allPolicies.value = allPolicies.value.filter(policy => policy.id !== id)
    updateCurrentPolicies()
    updateCharts()
    ElMessage.success(`${policyType}策略已删除`)
  } catch (error) {
    if (error !== 'cancel') {
      console.error('Error deleting policy:', error)
      ElMessage.error(`删除策略失败: ${error.message || error}`)
    }
  }
}

// Update current policies based on filters
const updateCurrentPolicies = () => {
  // For now, just copy all policies - in a real implementation you might filter
  currentPolicies.value = [...allPolicies.value]
}

// Tab change handler
const onTabChange = (tabName) => {
  if (tabName === 'stats') {
    // Initialize charts when stats tab is selected
    setTimeout(initCharts, 100)
  }
}

// On component mounted
onMounted(() => {
  fetchPolicies()
})

// Clean up charts when component is unmounted
const destroyCharts = () => {
  if (policyChart) {
    policyChart.dispose()
    policyChart = null
  }
  if (policyStatsChart) {
    policyStatsChart.dispose()
    policyStatsChart = null
  }
}

onUnmounted(() => {
  destroyCharts()
})
</script>

<style scoped>
.policy-management {
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

.stats-container {
  padding: 20px 0;
}

.stat-card {
  height: 100px;
  overflow: hidden;
}

.stat-content {
  padding: 10px;
}

.stat-value {
  font-size: 20px;
  font-weight: bold;
  color: #303133;
  margin-bottom: 4px;
}

.stat-label {
  font-size: 14px;
  color: #909399;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}

.chart-card {
  height: 400px;
}

.scenario-card {
  height: 350px;
  display: flex;
  flex-direction: column;
}

.scenario-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
}

.priority-badge {
  padding: 2px 8px;
  border-radius: 12px;
  font-size: 12px;
  color: white;
}

.priority-badge.danger {
  background-color: #f56c6c; /* danger color */
}

.priority-badge.success {
  background-color: #67c23a; /* success color */
}

.priority-badge.warning {
  background-color: #e6a23c; /* warning color */
}

.scenario-content h4 {
  margin: 10px 0 5px 0;
}

.scenario-content ul {
  padding-left: 20px;
  font-size: 13px;
  color: #606266;
}

.scenario-content li {
  margin-bottom: 3px;
}

.pipeline-description {
  line-height: 1.6;
}

.pipeline-description p {
  margin-bottom: 10px;
}

.pipeline-description ol {
  margin: 10px 0;
  padding-left: 25px;
}

.pipeline-description li {
  margin-bottom: 10px;
}

.sub-text {
  color: #909399;
  font-size: 13px;
  font-style: italic;
}
</style>
