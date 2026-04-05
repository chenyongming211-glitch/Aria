<!-- src/views/BandwidthControl.vue -->
<template>
  <div class="bandwidth-control">
    <el-card>
      <template #header>
        <div class="card-header">
          <h3>带宽控制</h3>
          <div class="header-actions">
            <el-button @click="applyAllRules" type="success">
              <el-icon><VideoPlay /></el-icon>
              应用规则
            </el-button>
            <el-button @click="clearAllRules" type="danger">
              <el-icon><Delete /></el-icon>
              清空规则
            </el-button>
            <el-button type="primary" @click="showAddDialog">
              <el-icon><Plus /></el-icon>
              添加
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
        title="QoS优先级说明"
        type="info"
        description="Aria 采用基于 eBPF 的高性能三层流量整形架构。系统按照 '从微观到宏观' 的粒度，通过特例豁免（Override）机制，实现对业务流量的精细化治理。系统遵循 '精确匹配优先（Most Specific Match First）' 原则。"
        :closable="false"
        style="margin-bottom: 20px;"
      />

      <el-tabs v-model="activeTab" @tab-change="onTabChange">
        <el-tab-pane label="规则" name="rules">
          <el-table
            :data="currentRules"
            stripe
            style="width: 100%"
            v-loading="loading"
          >
            <el-table-column prop="nodeName" label="节点" width="160" />
            <el-table-column prop="id" label="ID" width="100" />
            <el-table-column prop="type" label="类型" width="150">
              <template #default="{ row }">
                <el-tag :type="getRuleTypeColor(row.type)">
                  {{ getRuleTypeName(row.type) }}
                  <el-tooltip effect="dark" :content="getRuleTypeDescription(row.type)" placement="top">
                    <el-icon style="margin-left: 5px;"><QuestionFilled /></el-icon>
                  </el-tooltip>
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="name" label="名称" width="200" />
            <el-table-column label="详情" min-width="300">
              <template #default="{ row }">
                <div v-if="row.type === 'app'">
                  <strong>{{ row.srcIp || 'Any' }}:{{ row.srcPort || 'any' }}</strong>
                  →
                  <strong>{{ row.dstIp || 'Any' }}:{{ row.dstPort || 'any' }}</strong>
                  ({{ row.protocol.toUpperCase() || 'Any' }})
                  <el-tag size="small" type="warning">第一优先级</el-tag>
                </div>
                <div v-else-if="row.type === 'peer'">
                  <strong>{{ row.srcIp }}</strong> ↔ <strong>{{ row.dstIp }}</strong>
                  <el-tag size="small" type="primary">第二优先级</el-tag>
                </div>
                <div v-else-if="row.type === 'global'">
                  IP <strong>{{ row.targetIp }}</strong>
                  <el-tag size="small" type="info">第三优先级</el-tag>
                </div>
              </template>
            </el-table-column>
            <el-table-column prop="bandwidth" label="带宽 (Mbps)" width="150">
              <template #default="{ row }">
                <el-tag type="danger">{{ row.bandwidth }} Mbps</el-tag>
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
                <el-tag size="small" :type="getPolicyTagType(row.policyStatus)">
                  {{ formatPolicyStatus(row.policyStatus) }}
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
                <el-button size="small" @click="editRule(row)">编辑</el-button>
                <el-button size="small" type="primary" @click="viewRule(row)">查看</el-button>
                <el-popconfirm
                  :title="`确定要删除此${getRuleTypeName(row.type)}规则吗？`"
                  @confirm="deleteRule(row.id)"
                >
                  <template #reference>
                    <el-button size="small" type="danger">删除</el-button>
                  </template>
                </el-popconfirm>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>

        <el-tab-pane label="场景" name="scenarios">
          <div class="scenarios-container">
            <el-row :gutter="20">
              <el-col :span="8">
                <el-card class="scenario-card" shadow="hover">
                  <div class="scenario-header">
                    <el-tag type="warning" size="large">应用级</el-tag>
                    <div class="priority-badge high">第一优先级</div>
                  </div>
                  <div class="scenario-content">
                    <h4>应用级限速 (Application QoS)</h4>
                    <p>针对特定服务端口或特定连接的精细化控制</p>
                    <ul>
                      <li>服务熔断与保护：限制全网对特定端口的访问，防止服务过载</li>
                      <li>关键业务保障：保障核心数据库的主从同步流量，不受其他流量干扰</li>
                      <li>特定协议限速：精确控制HTTP、MySQL等协议流量</li>
                      <li>五元组精确匹配：源IP、目标IP、协议、源端口、目标端口</li>
                    </ul>
                  </div>
                </el-card>
              </el-col>

              <el-col :span="8">
                <el-card class="scenario-card" shadow="hover">
                  <div class="scenario-header">
                    <el-tag type="primary" size="large">对等体级</el-tag>
                    <div class="priority-badge medium">第二优先级</div>
                  </div>
                  <div class="scenario-content">
                    <h4>对等体限速 (Peer QoS)</h4>
                    <p>针对两个节点（IP对）之间所有通信流量的总和控制</p>
                    <ul>
                      <li>跨地域流量控制：限制数据中心间的总传输速率</li>
                      <li>租户间隔离：控制不同租户之间的通信带宽</li>
                      <li>备份限速：控制两台机器间的数据同步速度</li>
                      <li>点对点通信管理：管控节点间的所有协议流量</li>
                    </ul>
                  </div>
                </el-card>
              </el-col>

              <el-col :span="8">
                <el-card class="scenario-card" shadow="hover">
                  <div class="scenario-header">
                    <el-tag type="info" size="large">全局IP级</el-tag>
                    <div class="priority-badge lowest">第三优先级</div>
                  </div>
                  <div class="scenario-content">
                    <h4>全局IP限速 (Global IP QoS)</h4>
                    <p>针对单个节点（IP）的总吞吐量限制，作为最后的兜底规则</p>
                    <ul>
                      <li>嘈杂邻居隔离：防止某个租户抢占物理网卡资源</li>
                      <li>出口带宽限制：限制单机总出口带宽</li>
                      <li>安全防护：防止恶意攻击占满带宽</li>
                      <li>资源配额管理：为虚拟机分配带宽资源</li>
                    </ul>
                  </div>
                </el-card>
              </el-col>
            </el-row>

            <el-card style="margin-top: 20px;">
              <template #header>
                <div class="card-header">
                  <span>匹配逻辑说明</span>
                </div>
              </template>
              <div class="matching-logic">
                <p><strong>系统遵循 "精确匹配优先（Most Specific Match First）" 原则。</strong></p>

                <p><strong>优先级顺序（从高到低）：</strong></p>
                <ol>
                  <li><strong>应用级 (Application/Flow)</strong> → 针对特定业务端口或五元组</li>
                  <li><strong>对等体级 (Peer/Host)</strong> → 针对两点间的点对点通信</li>
                  <li><strong>全局级 (Global IP)</strong> → 针对单机的总出口带宽</li>
                </ol>

                <p><strong>关键规则（豁免机制）：</strong></p>
                <ul>
                  <li>一旦数据包匹配了高优先级的规则（例如应用级），该流量将单独限速，并且不再占用低优先级（如IP级）的带宽配额</li>
                  <li>这允许管理员为关键业务开辟"独立车道"</li>
                  <li>高优先级规则优先执行</li>
                </ul>
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
                    <div class="stat-value">{{ stats.totalRules }}</div>
                    <div class="stat-label">规则</div>
                  </div>
                </el-card>
              </el-col>
              <el-col :span="6">
                <el-card class="stat-card">
                  <div class="stat-content">
                    <div class="stat-value">{{ stats.appRules }}</div>
                    <div class="stat-label">应用级规则</div>
                  </div>
                </el-card>
              </el-col>
              <el-col :span="6">
                <el-card class="stat-card">
                  <div class="stat-content">
                    <div class="stat-value">{{ stats.peerRules }}</div>
                    <div class="stat-label">对等体级规则</div>
                  </div>
                </el-card>
              </el-col>
              <el-col :span="6">
                <el-card class="stat-card">
                  <div class="stat-content">
                    <div class="stat-value">{{ stats.globalRules }}</div>
                    <div class="stat-label">全局IP级规则</div>
                  </div>
                </el-card>
              </el-col>
            </el-row>

            <el-row :gutter="20" style="margin-top: 20px;">
              <el-col :span="12">
                <el-card class="chart-card">
                  <template #header>
                    <div class="card-header">
                      <span>规则分布</span>
                    </div>
                  </template>
                  <div id="rule-chart" style="height: 300px;"></div>
                </el-card>
              </el-col>
              <el-col :span="12">
                <el-card class="chart-card">
                  <template #header>
                    <div class="card-header">
                      <span>带宽分配</span>
                    </div>
                  </template>
                  <div id="bandwidth-chart" style="height: 300px;"></div>
                </el-card>
              </el-col>
            </el-row>
          </div>
        </el-tab-pane>
      </el-tabs>
    </el-card>

    <!-- Rule Editor Dialog -->
    <el-dialog
      v-model="ruleDialogVisible"
      :title="editingRule.id ? '编辑 ' + getRuleTypeName(editingRule.type) + ' 规则' : '添加 ' + getRuleTypeName(editingRule.type) + ' 规则'"
      width="60%"
    >
      <el-form
        v-if="editingRule"
        :model="editingRule"
        label-width="150px"
      >
        <el-form-item label="类型">
          <el-radio-group v-model="editingRule.type" @change="onRuleTypeChange">
            <el-radio label="app">
              应用级 (App-Level)
              <el-tooltip effect="dark" content="针对特定服务端口或特定连接的精细化控制（第一优先级）" placement="top">
                <el-icon><QuestionFilled style="margin-left: 5px;" /></el-icon>
              </el-tooltip>
            </el-radio>
            <el-radio label="peer">
              对等体级 (Peer-Level)
              <el-tooltip effect="dark" content="针对两个节点（IP对）之间所有通信流量的总和控制（第二优先级）" placement="top">
                <el-icon><QuestionFilled style="margin-left: 5px;" /></el-icon>
              </el-tooltip>
            </el-radio>
            <el-radio label="global">
              全局IP级 (Global-Level)
              <el-tooltip effect="dark" content="针对单个节点（IP）的总吞吐量限制，作为最后的兜底规则（第三优先级）" placement="top">
                <el-icon><QuestionFilled style="margin-left: 5px;" /></el-icon>
              </el-tooltip>
            </el-radio>
          </el-radio-group>
        </el-form-item>

        <el-form-item label="名称">
          <el-input v-model="editingRule.name" />
        </el-form-item>

        <el-form-item label="目标节点">
          <el-select v-model="editingRule.nodeId" placeholder="选择节点" style="width: 100%">
            <el-option
              v-for="node in tenantNodes"
              :key="node.id"
              :label="node.hostname || node.public_key || node.id"
              :value="node.id"
            />
          </el-select>
        </el-form-item>

        <!-- Application-level specific fields -->
        <template v-if="editingRule.type === 'app'">
          <el-form-item label="源 IP (Src IP)">
            <el-input v-model="editingRule.srcIp" placeholder="e.g. 192.168.1.100 or 192.168.1.0/24" />
          </el-form-item>
          <el-form-item label="目标 IP (Dst IP)">
            <el-input v-model="editingRule.dstIp" placeholder="e.g. 10.0.0.100 or 10.0.0.0/8" />
          </el-form-item>
          <el-form-item label="源端口 (Src Port)">
            <el-input v-model="editingRule.srcPort" placeholder="e.g. 1024 or 1024-2048" />
          </el-form-item>
          <el-form-item label="目标端口 (Dst Port)">
            <el-input v-model="editingRule.dstPort" placeholder="e.g. 80 or 80,443,8080" />
          </el-form-item>
          <el-form-item label="协议 (Protocol)">
            <el-select v-model="editingRule.protocol" placeholder="选择协议">
              <el-option label="TCP" value="tcp" />
              <el-option label="UDP" value="udp" />
              <el-option label="ICMP" value="icmp" />
              <el-option label="Any" value="any" />
            </el-select>
          </el-form-item>
        </template>

        <!-- Peer-level specific fields -->
        <template v-if="editingRule.type === 'peer'">
          <el-form-item label="源 IP (Src IP)">
            <el-input v-model="editingRule.srcIp" placeholder="e.g. 192.168.1.100" />
          </el-form-item>
          <el-form-item label="目标 IP (Dst IP)">
            <el-input v-model="editingRule.dstIp" placeholder="e.g. 10.0.0.100" />
          </el-form-item>
        </template>

        <!-- Global-level specific fields -->
        <template v-if="editingRule.type === 'global'">
          <el-form-item label="目标 IP (Target IP)">
            <el-input v-model="editingRule.targetIp" placeholder="e.g. 192.168.1.100 or 192.168.1.0/24" />
          </el-form-item>
        </template>

        <el-form-item label="最大带宽 (Mbps)">
          <el-input-number
            v-model="editingRule.bandwidth"
            :min="1"
            :max="10000"
            :step="1"
            placeholder="输入最大带宽，单位 Mbps"
          />
        </el-form-item>

        <el-form-item label="优先级">
          <el-input-number
            v-model="editingRule.priority"
            :min="1"
            :max="100"
            placeholder="优先级数值（1-100，数值越小优先级越高）"
          />
        </el-form-item>

        <el-form-item label="状态">
          <el-select v-model="editingRule.status" placeholder="选择状态">
            <el-option label="启用" value="active" />
            <el-option label="禁用" value="inactive" />
          </el-select>
        </el-form-item>

        <!-- Priority explanation -->
        <el-form-item>
          <el-alert
            :title="getPriorityExplanation(editingRule.type)"
            type="info"
            :closable="false"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="ruleDialogVisible = false">取消</el-button>
          <el-button type="primary" @click="saveRule">保存</el-button>
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
import { useTenantApi } from '@/composables/useTenantApi'

// Rule types mapping
const ruleTypes = {
  app: '应用级',
  peer: '对等体级',
  global: '全局IP级'
}

// Rule type descriptions
const ruleTypeDescriptions = {
  app: '针对特定服务端口或特定连接的精细化控制（第一优先级）',
  peer: '针对两个节点（IP对）之间所有通信流量的总和控制（第二优先级）',
  global: '针对单个节点（IP）的总吞吐量限制，作为最后的兜底规则（第三优先级）'
}

// All rules combined
const allRules = ref([])
const tenantNodes = ref([])

// Stats data
const stats = ref({
  totalRules: 0,
  appRules: 0,
  peerRules: 0,
  globalRules: 0,
  totalBandwidth: 0,
  utilization: 0
})

// Reactive variables
const loading = ref(false)
const activeTab = ref('rules')
const ruleDialogVisible = ref(false)
const editingRule = ref({
  nodeId: '',
  type: 'app',
  name: '',
  srcIp: '',
  dstIp: '',
  srcPort: '',
  dstPort: '',
  protocol: 'any',
  targetIp: '',
  targetPort: null,
  bandwidth: 100,
  priority: 50,
  status: 'active'
})

// Current rules based on active tab
const currentRules = ref([])

// Chart instances (will be initialized later)
let ruleChart = null
let bandwidthChart = null

// Get rule type color
const getRuleTypeColor = (type) => {
  switch(type) {
    case 'app': return 'warning' // yellow for first priority
    case 'peer': return 'primary'    // blue for second priority
    case 'global': return 'info'         // gray for third priority
    default: return 'info'
  }
}

// Get rule type name
const getRuleTypeName = (type) => {
  return ruleTypes[type] || type
}

// Get rule type description
const getRuleTypeDescription = (type) => {
  return ruleTypeDescriptions[type] || ''
}

// Get priority explanation
const getPriorityExplanation = (type) => {
  const explanations = {
    app: '应用级规则优先级最高，针对特定服务端口或特定连接进行精细化控制',
    peer: '对等体级规则优先级居中，针对两个节点（IP对）之间所有通信流量的总和控制',
    global: '全局IP级规则优先级最低，作为最后的兜底规则'
  }
  return explanations[type] || '规则优先级说明'
}

// Initialize charts
const initCharts = async () => {
  await nextTick() // Wait for DOM to be ready

  // Only import echarts when needed to reduce bundle size
  const echarts = await import('echarts')

  // Rule distribution chart
  const ruleChartDom = document.getElementById('rule-chart')
  if (ruleChartDom) {
    if (ruleChart) {
      ruleChart.dispose() // Dispose existing chart if any
    }
    ruleChart = echarts.init(ruleChartDom)
    const ruleOption = {
      title: {
        text: '规则分布',
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
          name: '规则类型',
          type: 'pie',
          radius: '50%',
          data: [
            { value: stats.value.appRules, name: '应用级' },
            { value: stats.value.peerRules, name: '对等体级' },
            { value: stats.value.globalRules, name: '全局IP级' }
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
    ruleChart.setOption(ruleOption)
  }

  // Bandwidth allocation chart
  const bandwidthChartDom = document.getElementById('bandwidth-chart')
  if (bandwidthChartDom) {
    if (bandwidthChart) {
      bandwidthChart.dispose() // Dispose existing chart if any
    }
    bandwidthChart = echarts.init(bandwidthChartDom)
    const bandwidthOption = {
      title: {
        text: '带宽分配',
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
        data: ['应用级', '对等体级', '全局IP级']
      },
      yAxis: {
        type: 'value',
        name: 'Mbps'
      },
      series: [
        {
          name: '分配带宽',
          type: 'bar',
          data: [
            stats.value.appRules * 50, // 示例数据计算
            stats.value.peerRules * 100,
            stats.value.globalRules * 200
          ]
        }
      ]
    }
    bandwidthChart.setOption(bandwidthOption)
  }
}

// Update chart data
const updateCharts = () => {
  // Update stats based on current rules
  stats.value.appRules = allRules.value.filter(rule => rule.type === 'app').length
  stats.value.peerRules = allRules.value.filter(rule => rule.type === 'peer').length
  stats.value.globalRules = allRules.value.filter(rule => rule.type === 'global').length
  stats.value.totalRules = allRules.value.length

  // Calculate total bandwidth
  stats.value.totalBandwidth = allRules.value.reduce((sum, rule) => sum + rule.bandwidth, 0)

  // Update charts if they exist
  if (ruleChart) {
    ruleChart.setOption({
      series: [{
        data: [
          { value: stats.value.appRules, name: '应用级' },
          { value: stats.value.peerRules, name: '对等体级' },
          { value: stats.value.globalRules, name: '全局IP级' }
        ]
      }]
    })
  }

  if (bandwidthChart) {
    bandwidthChart.setOption({
      series: [{
        data: [
          stats.value.appRules * 50,
          stats.value.peerRules * 100,
          stats.value.globalRules * 200
        ]
      }]
    })
  }
}

// Get status type for tag coloring
const getStatusType = (status) => {
  switch(status) {
    case 'active': return 'success'
    case 'inactive': return 'info'
    default: return 'info'
  }
}

const formatPolicyStatus = (status) => {
  const map = {
    applied: '已应用',
    pending: '待下发',
    in_progress: '下发中',
    error: '失败',
    idle: '空闲'
  }
  return map[status] || status || '未知'
}

const getPolicyTagType = (status) => {
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

// Fetch all rules from API
const loadNodes = async () => {
  try {
    tenantNodes.value = await useTenantApi.getTenantNodes()
    if (!editingRule.value.nodeId && tenantNodes.value.length > 0) {
      editingRule.value.nodeId = tenantNodes.value[0].id
    }
  } catch (error) {
    console.error('加载节点失败:', error)
  }
}

const fetchRules = async () => {
  try {
    loading.value = true
    // Get all rules from the API
    const response = await useQosApi.getAllRules()
    allRules.value = response

    updateCurrentRules()
    updateCharts()
  } catch (error) {
    console.error('Failed to fetch rules:', error)
    ElMessage.error(`获取规则失败: ${error.message}`)

    // For demo purposes, initialize with some sample data
    allRules.value = [
      {
        id: 'app-1',
        type: 'app',
        name: '数据库连接限速',
        srcIp: '192.168.1.100',
        dstIp: '192.168.1.200',
        srcPort: '0',
        dstPort: '3306',
        protocol: 'tcp',
        bandwidth: 50,
        priority: 1,
        status: 'active'
      },
      {
        id: 'peer-1',
        type: 'peer',
        name: '数据中心同步',
        srcIp: '10.0.1.10',
        dstIp: '10.0.2.10',
        bandwidth: 500,
        priority: 25,
        status: 'active'
      },
      {
        id: 'global-1',
        type: 'global',
        name: '用户带宽限制',
        targetIp: '192.168.1.100',
        bandwidth: 50,
        priority: 75,
        status: 'active'
      },
      {
        id: 'app-2',
        type: 'app',
        name: '视频流媒体限制',
        srcIp: '10.0.0.0/8',
        dstIp: '0.0.0.0/0',
        srcPort: 'any',
        dstPort: 'any',
        protocol: 'tcp',
        bandwidth: 50,
        priority: 10,
        status: 'active'
      }
    ]

    updateCurrentRules()
    updateCharts()
  } finally {
    loading.value = false
  }
}

// Refresh data
const refreshData = async () => {
  await fetchRules()
}

// Show add dialog
const showAddDialog = () => {
  editingRule.value = {
    nodeId: tenantNodes.value[0]?.id || '',
    type: 'app',
    name: '',
    srcIp: '',
    dstIp: '',
    srcPort: '',
    dstPort: '',
    protocol: 'any',
    targetIp: '',
    targetPort: null,
    bandwidth: 100,
    priority: 50,
    status: 'active'
  }
  ruleDialogVisible.value = true
}

// On rule type change
const onRuleTypeChange = (type) => {
  // Reset fields specific to other types when changing
  if (type !== 'app') {
    editingRule.value.srcIp = ''
    editingRule.value.dstIp = ''
    editingRule.value.srcPort = ''
    editingRule.value.dstPort = ''
    editingRule.value.protocol = 'any'
  }
  if (type !== 'peer') {
    editingRule.value.srcIp = type === 'app' ? editingRule.value.srcIp : ''
    editingRule.value.dstIp = type === 'app' ? editingRule.value.dstIp : ''
  }
  if (type !== 'global') {
    editingRule.value.targetIp = ''
  }
}

// Edit rule
const editRule = (rule) => {
  editingRule.value = { ...rule }
  ruleDialogVisible.value = true
}

// View rule
const viewRule = (rule) => {
  editingRule.value = { ...rule }
  ruleDialogVisible.value = true
  // Could disable editing in view mode
}

// Validate rule before saving
const validateRule = (rule) => {
  if (!rule.nodeId) {
    ElMessage.error('请选择目标节点')
    return false
  }

  if (!rule.name) {
    ElMessage.error('规则名称不能为空')
    return false
  }

  if (rule.bandwidth <= 0) {
    ElMessage.error('带宽必须大于0')
    return false
  }

  switch(rule.type) {
    case 'app':
      if (!rule.srcIp || !rule.dstIp) {
        ElMessage.error('应用级规则必须填写源IP和目标IP')
        return false
      }
      break
    case 'peer':
      if (!rule.srcIp || !rule.dstIp) {
        ElMessage.error('对等体级规则必须填写源IP和目标IP')
        return false
      }
      break
    case 'global':
      if (!rule.targetIp) {
        ElMessage.error('全局IP级规则必须填写目标IP')
        return false
      }
      break
  }

  return true
}

// Save rule
const saveRule = async () => {
  if (!validateRule(editingRule.value)) {
    return
  }

  try {
    if (editingRule.value.id) {
      // Update existing rule
      let updateResult;

      switch(editingRule.value.type) {
        case 'app':
          updateResult = await useQosApi.updateServiceRule(editingRule.value.id, editingRule.value)
          break
        case 'peer':
          updateResult = await useQosApi.updatePeerRule(editingRule.value.id, editingRule.value)
          break
        case 'global':
          updateResult = await useQosApi.updateIpRule(editingRule.value.id, editingRule.value)
          break
      }

      const index = allRules.value.findIndex(r => r.id === editingRule.value.id)
      if (index !== -1) {
        const selectedNode = tenantNodes.value.find((node) => node.id === editingRule.value.nodeId)
        allRules.value[index] = {
          ...editingRule.value,
          id: updateResult?.id || editingRule.value.id,
          nodeName: selectedNode?.hostname || editingRule.value.nodeName || editingRule.value.nodeId
        }
      }
      ElMessage.success(`${ruleTypes[editingRule.value.type]}规则已更新`)
    } else {
      // Create new rule
      let createResult;

      switch(editingRule.value.type) {
        case 'app':
          createResult = await useQosApi.createServiceRule(editingRule.value)
          break
        case 'peer':
          createResult = await useQosApi.createPeerRule(editingRule.value)
          break
        case 'global':
          createResult = await useQosApi.createIpRule(editingRule.value)
          break
      }

      const newId = createResult?.id || `${editingRule.value.type}-${Date.now()}`
      const selectedNode = tenantNodes.value.find((node) => node.id === editingRule.value.nodeId)
      allRules.value.push({
        ...editingRule.value,
        id: newId,
        nodeName: selectedNode?.hostname || editingRule.value.nodeName || editingRule.value.nodeId
      })
      ElMessage.success(`${ruleTypes[editingRule.value.type]}规则已创建`)
    }

    // Update current view
    updateCurrentRules()
    updateCharts()
    ruleDialogVisible.value = false
  } catch (error) {
    console.error('Error saving rule:', error)
    ElMessage.error(`保存规则失败: ${error.message || error}`)
  }
}

// Apply all rules (activate the eBPF programs)
const applyAllRules = async () => {
  try {
    loading.value = true
    // Call API to activate all rules
    const response = await useQosApi.applyAllRules()

    if (response.success) {
      ElMessage.success('所有规则已成功应用到系统')
    } else {
      ElMessage.warning('部分规则可能存在冲突，请检查配置')
    }
  } catch (error) {
    console.error('Error applying rules:', error)
    ElMessage.error(`应用规则失败: ${error.message || error}`)
  } finally {
    loading.value = false
  }
}

// Clear all rules
const clearAllRules = async () => {
  try {
    await ElMessageBox.confirm(
      '确定要清空所有规则吗？此操作不可撤销。',
      '警告',
      {
        confirmButtonText: '确定清空',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )

    // Call API to clear all rules
    await useQosApi.clearAllRules()

    // Clear local data
    allRules.value = []
    updateCurrentRules()
    updateCharts()
    ElMessage.success('所有规则已清空')
  } catch (error) {
    if (error !== 'cancel') {
      console.error('Error clearing rules:', error)
      ElMessage.error(`清空规则失败: ${error.message || error}`)
    }
  }
}

// Delete rule
const deleteRule = async (id) => {
  try {
    await ElMessageBox.confirm(
      '确定要删除此规则吗？此操作不可撤销。',
      '警告',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )

    // Find the rule to get its type for success message
    const ruleToDelete = allRules.value.find(rule => rule.id === id)
    const ruleType = ruleToDelete ? ruleTypes[ruleToDelete.type] : '未知'

    // Remove from API
    switch(ruleToDelete.type) {
      case 'app':
        await useQosApi.deleteServiceRule(id)
        break
      case 'peer':
        await useQosApi.deletePeerRule(id)
        break
      case 'global':
        await useQosApi.deleteIpRule(id)
        break
    }

    // Remove from local data
    allRules.value = allRules.value.filter(rule => rule.id !== id)
    updateCurrentRules()
    updateCharts()
    ElMessage.success(`${ruleType}规则已删除`)
  } catch (error) {
    if (error !== 'cancel') {
      console.error('Error deleting rule:', error)
      ElMessage.error(`删除规则失败: ${error.message || error}`)
    }
  }
}

// Update current rules based on filters
const updateCurrentRules = () => {
  // For now, just copy all rules - in a real implementation you might filter
  currentRules.value = [...allRules.value]
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
  loadNodes()
  fetchRules()
})

// Clean up charts when component is unmounted
const destroyCharts = () => {
  if (ruleChart) {
    ruleChart.dispose()
    ruleChart = null
  }
  if (bandwidthChart) {
    bandwidthChart.dispose()
    bandwidthChart = null
  }
}

onUnmounted(() => {
  destroyCharts()
})
</script>

<style scoped>
.bandwidth-control {
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

.priority-badge.high {
  background-color: #e6a23c; /* warning color */
}

.priority-badge.medium {
  background-color: #409eff; /* primary color */
}

.priority-badge.low {
  background-color: #67c23a; /* success color */
}

.priority-badge.lowest {
  background-color: #909399; /* info color */
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

.matching-logic {
  line-height: 1.6;
}

.matching-logic p {
  margin-bottom: 10px;
}

.matching-logic ol, .matching-logic ul {
  margin: 10px 0;
  padding-left: 25px;
}

.matching-logic li {
  margin-bottom: 5px;
}
</style>
