<!-- src/views/Monitoring.vue -->
<template>
  <div class="monitoring">
    <el-card>
      <template #header>
        <div class="card-header">
          <h3>{{ labels.title }}</h3>
          <div class="header-actions">
            <el-date-picker
              v-model="dateRange"
              type="datetimerange"
              :range-separator="currentLang === 'zh' ? '至' : 'To'"
              :start-placeholder="labels.startDate"
              :end-placeholder="labels.endDate"
              style="width: 300px; margin-right: 10px;"
            />
            <el-button type="primary" @click="refreshData">
              <el-icon><Refresh /></el-icon>
              {{ labels.refresh }}
            </el-button>
          </div>
        </div>
      </template>

      <el-row :gutter="20" class="stats-grid">
        <el-col :span="6">
          <el-card class="stat-card">
            <div class="stat-content">
              <div class="stat-value">{{ stats.totalConnections }}</div>
              <div class="stat-label">{{ labels.activeConnections }}</div>
              <div class="stat-change positive">+5.2% {{ labels.fromLastHour }}</div>
            </div>
          </el-card>
        </el-col>
        <el-col :span="6">
          <el-card class="stat-card">
            <div class="stat-content">
              <div class="stat-value">{{ stats.avgLatency }} ms</div>
              <div class="stat-label">{{ labels.avgLatency }}</div>
              <div class="stat-change negative">+1.3% {{ labels.fromLastHour }}</div>
            </div>
          </el-card>
        </el-col>
        <el-col :span="6">
          <el-card class="stat-card">
            <div class="stat-content">
              <div class="stat-value">{{ stats.packetLoss }}%</div>
              <div class="stat-label">{{ labels.packetLoss }}</div>
              <div class="stat-change negative">+0.1% {{ labels.fromLastHour }}</div>
            </div>
          </el-card>
        </el-col>
        <el-col :span="6">
          <el-card class="stat-card">
            <div class="stat-content">
              <div class="stat-value">{{ stats.throughput }} Gbps</div>
              <div class="stat-label">{{ labels.throughput }}</div>
              <div class="stat-change positive">+3.7% {{ labels.fromLastHour }}</div>
            </div>
          </el-card>
        </el-col>
      </el-row>

      <el-tabs v-model="activeTab" class="monitoring-tabs">
        <el-tab-pane :label="labels.trafficGraph" name="traffic">
          <div ref="trafficChartRef" class="chart-container" />
        </el-tab-pane>
        <el-tab-pane :label="labels.nodeHealth" name="health">
          <el-table
            :data="nodeHealth"
            stripe
            style="width: 100%"
            height="400"
          >
            <el-table-column prop="hostname" :label="labels.node" width="150" />
            <el-table-column prop="cpu" :label="labels.cpu" width="100">
              <template #default="{ row }">
                <el-progress :percentage="row.cpu" :color="getProgressColor(row.cpu)" />
              </template>
            </el-table-column>
            <el-table-column prop="memory" :label="labels.memory" width="100">
              <template #default="{ row }">
                <el-progress :percentage="row.memory" :color="getProgressColor(row.memory)" />
              </template>
            </el-table-column>
            <el-table-column prop="disk" :label="labels.disk" width="100">
              <template #default="{ row }">
                <el-progress :percentage="row.disk" :color="getProgressColor(row.disk)" />
              </template>
            </el-table-column>
            <el-table-column prop="latency" :label="labels.latency" width="100">
              <template #default="{ row }">
                <span :class="getLatencyClass(row.latency)">{{ row.latency }} ms</span>
              </template>
            </el-table-column>
            <el-table-column prop="connections" :label="labels.connections" width="100" />
            <el-table-column prop="status" :label="labels.status" width="100">
              <template #default="{ row }">
                <el-tag :type="getStatusType(row.status)" size="small">{{ getStatusLabel(row.status) }}</el-tag>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>
        <el-tab-pane :label="labels.alerts" name="alerts">
          <el-table
            :data="alerts"
            stripe
            style="width: 100%"
            height="400"
          >
            <el-table-column prop="timestamp" :label="labels.timestamp" width="180" />
            <el-table-column prop="severity" :label="labels.severity" width="100">
              <template #default="{ row }">
                <el-tag :type="getSeverityType(row.severity)" size="small">{{ row.severity }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="message" :label="labels.message" />
            <el-table-column prop="node" :label="labels.node" width="150" />
          </el-table>
        </el-tab-pane>
      </el-tabs>
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted, watch, computed } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import * as echarts from 'echarts'
import { useAppStore } from '@/stores'

const appStore = useAppStore()
const currentLang = computed(() => appStore.lang)

const labels = computed(() => ({
  title: currentLang.value === 'zh' ? '网络监控' : 'Network Monitoring',
  refresh: currentLang.value === 'zh' ? '刷新' : 'Refresh',
  activeConnections: currentLang.value === 'zh' ? '活动连接数' : 'Active Connections',
  avgLatency: currentLang.value === 'zh' ? '平均延迟' : 'Avg. Latency',
  packetLoss: currentLang.value === 'zh' ? '丢包率' : 'Packet Loss',
  throughput: currentLang.value === 'zh' ? '吞吐量' : 'Throughput',
  trafficGraph: currentLang.value === 'zh' ? '流量图表' : 'Traffic Graph',
  nodeHealth: currentLang.value === 'zh' ? '节点健康' : 'Node Health',
  alerts: currentLang.value === 'zh' ? '告警' : 'Alerts',
  node: currentLang.value === 'zh' ? '节点' : 'Node',
  cpu: currentLang.value === 'zh' ? 'CPU' : 'CPU',
  memory: currentLang.value === 'zh' ? '内存' : 'Memory',
  disk: currentLang.value === 'zh' ? '磁盘' : 'Disk',
  latency: currentLang.value === 'zh' ? '延迟' : 'Latency',
  connections: currentLang.value === 'zh' ? '连接数' : 'Connections',
  status: currentLang.value === 'zh' ? '状态' : 'Status',
  healthy: currentLang.value === 'zh' ? '健康' : 'Healthy',
  warning: currentLang.value === 'zh' ? '警告' : 'Warning',
  critical: currentLang.value === 'zh' ? '严重' : 'Critical',
  timestamp: currentLang.value === 'zh' ? '时间' : 'Timestamp',
  severity: currentLang.value === 'zh' ? '级别' : 'Severity',
  message: currentLang.value === 'zh' ? '消息' : 'Message',
  fromLastHour: currentLang.value === 'zh' ? '较前一小时' : 'from last hour',
  startDate: currentLang.value === 'zh' ? '开始日期' : 'Start date',
  endDate: currentLang.value === 'zh' ? '结束日期' : 'End date',
}))

// Mock data
const dateRange = ref([new Date(2024, 0, 15, 0, 0, 0), new Date()])
const activeTab = ref('traffic')
const stats = ref({
  totalConnections: 1242,
  avgLatency: 18.4,
  packetLoss: 0.02,
  throughput: 2.34
})

const nodeHealth = ref([
  { hostname: 'worker-01', cpu: 45, memory: 62, disk: 30, latency: 12, connections: 234, status: 'healthy' },
  { hostname: 'worker-02', cpu: 78, memory: 85, disk: 60, latency: 18, connections: 421, status: 'warning' },
  { hostname: 'backup-server', cpu: 20, memory: 30, disk: 15, latency: 45, connections: 12, status: 'critical' },
  { hostname: 'main-router', cpu: 65, memory: 70, disk: 45, latency: 8, connections: 567, status: 'healthy' },
  { hostname: 'edge-node-01', cpu: 88, memory: 92, disk: 75, latency: 22, connections: 345, status: 'warning' },
  { hostname: 'edge-node-02', cpu: 30, memory: 40, disk: 25, latency: 15, connections: 189, status: 'healthy' },
  { hostname: 'gw-01', cpu: 95, memory: 88, disk: 80, latency: 67, connections: 789, status: 'critical' },
  { hostname: 'gw-02', cpu: 50, memory: 55, disk: 35, latency: 14, connections: 210, status: 'healthy' }
])

const alerts = ref([
  { id: 1, timestamp: '2024-01-15 14:30:25', severity: 'critical', message: 'High CPU usage on gw-01 (>90%)', node: 'gw-01' },
  { id: 2, timestamp: '2024-01-15 14:28:12', severity: 'warning', message: 'Latency above threshold on backup-server', node: 'backup-server' },
  { id: 3, timestamp: '2024-01-15 14:15:08', severity: 'info', message: 'New node connected: edge-node-03', node: 'edge-node-03' },
  { id: 4, timestamp: '2024-01-15 14:10:45', severity: 'critical', message: 'Connection loss to bj-region', node: 'main-router' },
  { id: 5, timestamp: '2024-01-15 14:05:22', severity: 'warning', message: 'Disk usage above 80% on gw-01', node: 'gw-01' }
])

const trafficChartRef = ref(null)
let chartInstance = null

const getProgressColor = (value) => {
  if (value < 70) return '#67c23a'
  if (value < 85) return '#e6a23c'
  return '#f56c6c'
}

const getStatusType = (status) => {
  switch(status) {
    case 'healthy': return 'success'
    case 'warning': return 'warning'
    case 'critical': return 'danger'
    default: return 'info'
  }
}

const getStatusLabel = (status) => {
  const map = {
    healthy: labels.value.healthy,
    warning: labels.value.warning,
    critical: labels.value.critical
  }
  return map[status] || status
}

const getAlertSeverityType = (severity) => {
  switch(severity) {
    case 'critical': return 'danger'
    case 'warning': return 'warning'
    case 'info': return 'info'
    default: return 'primary'
  }
}

const refreshData = () => {
  // Simulate API call
  stats.value = {
    totalConnections: Math.floor(Math.random() * 1000) + 1200,
    avgLatency: parseFloat((Math.random() * 10 + 15).toFixed(1)),
    packetLoss: parseFloat((Math.random() * 0.1).toFixed(2)),
    throughput: parseFloat((Math.random() * 2 + 1.5).toFixed(2))
  }
}

const acknowledgeAlert = (alert) => {
  // In a real app, this would call an API to acknowledge the alert
  console.log('Acknowledging alert:', alert)
}

onMounted(() => {
  initTrafficChart()
})

watch(activeTab, (newTab) => {
  if (newTab === 'traffic' && trafficChartRef.value) {
    // Delay chart initialization to ensure DOM is ready
    setTimeout(initTrafficChart, 100)
  }
})

const initTrafficChart = () => {
  if (trafficChartRef.value) {
    if (chartInstance) {
      chartInstance.dispose()
    }

    chartInstance = echarts.init(trafficChartRef.value)
    const option = {
      tooltip: {
        trigger: 'axis'
      },
      legend: {
        data: ['Upload', 'Download', 'Connections']
      },
      grid: {
        left: '3%',
        right: '4%',
        bottom: '3%',
        containLabel: true
      },
      xAxis: {
        type: 'category',
        boundaryGap: false,
        data: Array.from({ length: 24 }, (_, i) => `${i}:00`)
      },
      yAxis: {
        type: 'value'
      },
      series: [
        {
          name: 'Upload',
          type: 'line',
          smooth: true,
          data: Array.from({ length: 24 }, () => Math.random() * 100 + 50)
        },
        {
          name: 'Download',
          type: 'line',
          smooth: true,
          data: Array.from({ length: 24 }, () => Math.random() * 150 + 80)
        },
        {
          name: 'Connections',
          type: 'line',
          smooth: true,
          data: Array.from({ length: 24 }, () => Math.random() * 500 + 800)
        }
      ]
    }
    chartInstance.setOption(option)

    // Handle resize
    window.addEventListener('resize', () => {
      chartInstance.resize()
    })
  }
}
</script>

<style scoped>
.monitoring {
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

.stats-grid {
  margin-bottom: 20px;
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
  margin-bottom: 4px;
}

.stat-change {
  font-size: 12px;
}

.stat-change.positive {
  color: #67c23a;
}

.stat-change.negative {
  color: #f56c6c;
}

.monitoring-tabs {
  margin-top: 20px;
}

.chart-container {
  width: 100%;
  height: 400px;
}
</style>