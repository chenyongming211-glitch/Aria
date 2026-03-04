<!-- src/views/Dashboard.vue - 浅色主题仪表盘 -->
<template>
  <div class="dashboard">
    <!-- 统计卡片行 -->
    <el-row :gutter="20" class="stats-row">
      <el-col :xs="24" :sm="12" :lg="6" v-for="(stat, index) in stats" :key="index">
        <div class="stat-card light-card" :class="`stat-card-${stat.color}`">
          <div class="stat-header">
            <div class="stat-icon">
              <component :is="stat.icon" />
            </div>
            <div class="stat-trend" :class="stat.trendClass">
              <el-icon>
                <component :is="stat.trendIcon" />
              </el-icon>
              <span>{{ stat.trend }}</span>
            </div>
          </div>
          <div class="stat-content">
            <div class="stat-value">{{ stat.value }}</div>
            <div class="stat-label">{{ stat.label }}</div>
            <div class="stat-description">{{ stat.description }}</div>
          </div>
          <div class="stat-progress" v-if="stat.progress">
            <div class="progress-bar">
              <div class="progress-fill" :style="{ width: stat.progress + '%' }"></div>
            </div>
          </div>
        </div>
      </el-col>
    </el-row>

    <!-- 主内容行 -->
    <el-row :gutter="20" class="content-row">
      <!-- 网络流量图表 -->
      <el-col :xs="24" :lg="16">
        <el-card class="chart-card light-card" shadow="never">
          <template #header>
            <div class="card-header">
              <div class="header-left">
                <el-icon class="header-icon"><TrendCharts /></el-icon>
                <span class="header-title">Network Traffic</span>
              </div>
              <div class="header-actions">
                <el-radio-group v-model="timeRange" size="small">
                  <el-radio-button label="1h">1H</el-radio-button>
                  <el-radio-button label="24h">24H</el-radio-button>
                  <el-radio-button label="7d">7D</el-radio-button>
                  <el-radio-button label="30d">30D</el-radio-button>
                </el-radio-group>
                <el-button link :icon="Download">Export</el-button>
              </div>
            </div>
          </template>
          <div ref="trafficChartRef" class="chart-container" v-loading="loading"></div>
        </el-card>
      </el-col>

      <!-- 最近活动 -->
      <el-col :xs="24" :lg="8">
        <el-card class="activity-card light-card" shadow="never">
          <template #header>
            <div class="card-header">
              <div class="header-left">
                <el-icon class="header-icon"><Clock /></el-icon>
                <span class="header-title">Recent Activity</span>
              </div>
              <el-button link @click="refreshActivities" :icon="Refresh">Refresh</el-button>
            </div>
          </template>
          <div class="activity-list">
            <div
              v-for="(activity, index) in activities"
              :key="index"
              class="activity-item"
            >
              <div class="activity-icon-wrapper" :class="activity.type">
                <el-icon>
                  <component :is="activity.icon" />
                </el-icon>
              </div>
              <div class="activity-content">
                <div class="activity-text">{{ activity.text }}</div>
                <div class="activity-meta">
                  <span class="activity-time">{{ activity.time }}</span>
                  <el-tag v-if="activity.tag" size="small" :type="activity.tagType">
                    {{ activity.tag }}
                  </el-tag>
                </div>
              </div>
            </div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <!-- 第二行内容 -->
    <el-row :gutter="20" class="content-row">
      <!-- 区域分布 -->
      <el-col :xs="24" :lg="8">
        <el-card class="region-card light-card" shadow="never">
          <template #header>
            <div class="card-header">
              <div class="header-left">
                <el-icon class="header-icon"><Location /></el-icon>
                <span class="header-title">Region Distribution</span>
              </div>
            </div>
          </template>
          <div class="region-list">
            <div v-for="region in regions" :key="region.name" class="region-item">
              <div class="region-info">
                <div class="region-icon">{{ region.icon }}</div>
                <div class="region-details">
                  <div class="region-name">{{ region.name }}</div>
                  <div class="region-nodes">{{ region.nodes }} nodes</div>
                </div>
              </div>
              <div class="region-stats">
                <el-progress
                  :percentage="region.percentage"
                  :color="region.color"
                  :stroke-width="6"
                  :show-text="false"
                />
                <div class="region-percentage">{{ region.percentage }}%</div>
              </div>
            </div>
          </div>
        </el-card>
      </el-col>

      <!-- 快速操作 -->
      <el-col :xs="24" :lg="8">
        <el-card class="quick-actions-card light-card" shadow="never">
          <template #header>
            <div class="card-header">
              <div class="header-left">
                <el-icon class="header-icon"><Lightning /></el-icon>
                <span class="header-title">Quick Actions</span>
              </div>
            </div>
          </template>
          <div class="quick-actions-grid">
            <div
              v-for="action in quickActions"
              :key="action.name"
              class="quick-action-item"
              @click="action.handler"
            >
              <div class="action-icon" :style="{ background: action.bgColor }">
                <el-icon :color="action.iconColor">
                  <component :is="action.icon" />
                </el-icon>
              </div>
              <div class="action-name">{{ action.name }}</div>
            </div>
          </div>
        </el-card>
      </el-col>

      <!-- 系统健康 -->
      <el-col :xs="24" :lg="8">
        <el-card class="health-card light-card" shadow="never">
          <template #header>
            <div class="card-header">
              <div class="header-left">
                <el-icon class="header-icon"><CircleCheck /></el-icon>
                <span class="header-title">System Health</span>
              </div>
            </div>
          </template>
          <div class="health-metrics">
            <div v-for="metric in healthMetrics" :key="metric.name" class="health-item">
              <div class="metric-header">
                <span class="metric-name">{{ metric.name }}</span>
                <span class="metric-value" :class="metric.statusClass">
                  {{ metric.value }}
                </span>
              </div>
              <el-progress
                :percentage="metric.percentage"
                :color="metric.color"
                :stroke-width="8"
              />
            </div>
          </div>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import {
  Monitor,
  CircleCheck,
  Position,
  TrendCharts,
  Clock,
  Refresh,
  Location,
  Lightning,
  Setting,
  Plus,
  Download,
  ArrowUp,
  ArrowDown,
  Minus,
  Connection,
  DataAnalysis,
  Lock
} from '@element-plus/icons-vue'
import * as echarts from 'echarts'
import { useMonitorApi } from '@/composables/useMonitorApi'
import { useTenantApi } from '@/composables/useTenantApi'
import { ElMessage } from 'element-plus'

const loading = ref(false)
const timeRange = ref('24h')
const trafficChartRef = ref(null)
let chartInstance = null

// 统计数据
const stats = computed(() => [
  {
    label: 'Total Nodes',
    value: nodesData.value.total,
    description: 'Across all regions',
    color: 'blue',
    icon: Monitor,
    trend: '+12%',
    trendIcon: ArrowUp,
    trendClass: 'positive',
    progress: 85
  },
  {
    label: 'Online Nodes',
    value: nodesData.value.online,
    description: 'Currently active',
    color: 'green',
    icon: CircleCheck,
    trend: '+5%',
    trendIcon: ArrowUp,
    trendClass: 'positive',
    progress: 92
  },
  {
    label: 'Advertised Routes',
    value: nodesData.value.routes,
    description: 'Network routes',
    color: 'orange',
    icon: Position,
    trend: '-2%',
    trendIcon: ArrowDown,
    trendClass: 'negative',
    progress: 68
  },
  {
    label: 'Bandwidth',
    value: nodesData.value.bandwidth + ' Gbps',
    description: 'Peak throughput',
    color: 'purple',
    icon: TrendCharts,
    trend: '+8%',
    trendIcon: ArrowUp,
    trendClass: 'positive',
    progress: 76
  }
])

const nodesData = ref({
  total: 0,
  online: 0,
  routes: 0,
  bandwidth: '0'
})

// 活动数据
const activities = ref([
  {
    type: 'node',
    icon: Monitor,
    text: 'Node "worker-01" came online',
    time: '2 min ago',
    tag: 'Online',
    tagType: 'success'
  },
  {
    type: 'route',
    icon: Position,
    text: 'New route added: 10.0.1.0/24',
    time: '15 min ago',
    tag: 'Route',
    tagType: 'info'
  },
  {
    type: 'config',
    icon: Setting,
    text: 'Configuration updated for "main-router"',
    time: '1 hour ago',
    tag: 'Config',
    tagType: 'warning'
  },
  {
    type: 'node',
    icon: Monitor,
    text: 'Node "backup-server" went offline',
    time: '2 hours ago',
    tag: 'Offline',
    tagType: 'danger'
  },
  {
    type: 'node',
    icon: Monitor,
    text: 'Node "worker-02" joined the network',
    time: '3 hours ago',
    tag: 'New',
    tagType: 'success'
  }
])

// 区域数据
const regions = ref([
  { name: 'Shanghai', icon: '🏙️', nodes: 12, percentage: 45, color: '#3B82F6' },
  { name: 'Beijing', icon: '🏛️', nodes: 8, percentage: 30, color: '#22C55E' },
  { name: 'Guangzhou', icon: '🌆', nodes: 4, percentage: 15, color: '#F59E0B' },
  { name: 'Hong Kong', icon: '🏝️', nodes: 3, percentage: 10, color: '#06B6D4' }
])

// 快速操作
const quickActions = [
  {
    name: 'Add Node',
    icon: Plus,
    bgColor: 'rgba(59, 130, 246, 0.1)',
    iconColor: '#3B82F6',
    handler: () => ElMessage.info('Add node dialog will open')
  },
  {
    name: 'Create Route',
    icon: Position,
    bgColor: 'rgba(34, 197, 94, 0.1)',
    iconColor: '#22C55E',
    handler: () => ElMessage.info('Create route dialog will open')
  },
  {
    name: 'View Logs',
    icon: DataAnalysis,
    bgColor: 'rgba(245, 158, 11, 0.1)',
    iconColor: '#F59E0B',
    handler: () => ElMessage.info('Logs viewer will open')
  },
  {
    name: 'System Config',
    icon: Setting,
    bgColor: 'rgba(139, 92, 246, 0.1)',
    iconColor: '#8B5CF6',
    handler: () => ElMessage.info('System config will open')
  }
]

// 健康指标
const healthMetrics = computed(() => [
  {
    name: 'CPU Usage',
    value: '45%',
    percentage: 45,
    color: '#22C55E',
    statusClass: 'status-good'
  },
  {
    name: 'Memory Usage',
    value: '62%',
    percentage: 62,
    color: '#F59E0B',
    statusClass: 'status-warning'
  },
  {
    name: 'Disk Usage',
    value: '38%',
    percentage: 38,
    color: '#22C55E',
    statusClass: 'status-good'
  },
  {
    name: 'Network Latency',
    value: '12ms',
    percentage: 24,
    color: '#3B82F6',
    statusClass: 'status-good'
  }
])

// 获取仪表盘数据
const fetchDashboardData = async () => {
  try {
    loading.value = true

    const nodes = await useTenantApi.getTenantNodes()

    nodesData.value.total = nodes.length || 0
    nodesData.value.online = nodes.filter(n => n.status === 'online').length || 0

    let routeCount = 0
    nodes.forEach(node => {
      if (node.advertised_routes) {
        const routes = Array.isArray(node.advertised_routes)
          ? node.advertised_routes
          : JSON.parse(node.advertised_routes || '[]')
        routeCount += routes.length
      }
    })
    nodesData.value.routes = routeCount

  } catch (error) {
    console.error('获取仪表盘数据失败:', error)
  } finally {
    loading.value = false
  }
}

// 初始化图表
const initTrafficChart = () => {
  if (trafficChartRef.value) {
    chartInstance = echarts.init(trafficChartRef.value)
    updateChart()

    window.addEventListener('resize', () => {
      chartInstance.resize()
    })
  }
}

// 更新图表数据
const updateChart = () => {
  if (!chartInstance) return

  const data = generateChartData()

  const option = {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'axis',
      backgroundColor: 'rgba(255, 255, 255, 0.95)',
      borderColor: 'rgba(0, 0, 0, 0.1)',
      textStyle: {
        color: '#1E293B'
      },
      axisPointer: {
        lineStyle: {
          color: 'rgba(59, 130, 246, 0.2)'
        }
      }
    },
    legend: {
      data: ['Upload', 'Download'],
      textStyle: {
        color: '#475569'
      },
      top: 0
    },
    grid: {
      left: '3%',
      right: '4%',
      bottom: '3%',
      top: '15%',
      containLabel: true
    },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: data.xAxis,
      axisLine: {
        lineStyle: {
          color: 'rgba(0, 0, 0, 0.08)'
        }
      },
      axisLabel: {
        color: '#64748B'
      }
    },
    yAxis: {
      type: 'value',
      name: 'Gbps',
      nameTextStyle: {
        color: '#64748B'
      },
      axisLine: {
        lineStyle: {
          color: 'rgba(0, 0, 0, 0.08)'
        }
      },
      axisLabel: {
        color: '#64748B'
      },
      splitLine: {
        lineStyle: {
          color: 'rgba(0, 0, 0, 0.05)'
        }
      }
    },
    series: [
      {
        name: 'Upload',
        type: 'line',
        smooth: true,
        symbol: 'none',
        data: data.upload,
        lineStyle: {
          width: 3,
          color: '#3B82F6'
        },
        areaStyle: {
          color: {
            type: 'linear',
            x: 0,
            y: 0,
            x2: 0,
            y2: 1,
            colorStops: [
              { offset: 0, color: 'rgba(59, 130, 246, 0.2)' },
              { offset: 1, color: 'rgba(59, 130, 246, 0.02)' }
            ]
          }
        }
      },
      {
        name: 'Download',
        type: 'line',
        smooth: true,
        symbol: 'none',
        data: data.download,
        lineStyle: {
          width: 3,
          color: '#22C55E'
        },
        areaStyle: {
          color: {
            type: 'linear',
            x: 0,
            y: 0,
            x2: 0,
            y2: 1,
            colorStops: [
              { offset: 0, color: 'rgba(34, 197, 94, 0.2)' },
              { offset: 1, color: 'rgba(34, 197, 94, 0.02)' }
            ]
          }
        }
      }
    ]
  }

  chartInstance.setOption(option, true)
}

// 生成模拟图表数据
const generateChartData = () => {
  const hours = 24
  const xAxis = Array.from({ length: hours }, (_, i) => {
    const time = new Date()
    time.setHours(time.getHours() - hours + i)
    return time.getHours() + ':00'
  })

  const upload = Array.from({ length: hours }, () =>
    (Math.random() * 1.5 + 0.5).toFixed(2)
  )

  const download = Array.from({ length: hours }, () =>
    (Math.random() * 2.5 + 1).toFixed(2)
  )

  return { xAxis, upload, download }
}

// 刷新活动
const refreshActivities = () => {
  ElMessage.success('Activities refreshed')
}

// 监听时间范围变化
watch(timeRange, () => {
  updateChart()
})

onMounted(async () => {
  await fetchDashboardData()
  initTrafficChart()
})
</script>

<style scoped>
/* ============================================
   Dashboard Styles (浅色主题)
   ============================================ */
.dashboard {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* ============================================
   Stats Row
   ============================================ */
.stats-row {
  margin-bottom: 0;
}

/* Stat Cards (浅色) */
.stat-card {
  position: relative;
  padding: 24px;
  border-radius: var(--aria-radius-lg);
  transition: all var(--aria-transition-base);
  cursor: pointer;
}

.stat-card:hover {
  transform: translateY(-4px);
  box-shadow: var(--aria-shadow-lg);
  border-color: var(--aria-border-hover);
}

.stat-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 20px;
}

.stat-icon {
  width: 56px;
  height: 56px;
  border-radius: var(--aria-radius-md);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 28px;
  transition: all var(--aria-transition-base);
}

.stat-card-blue .stat-icon {
  background: rgba(59, 130, 246, 0.1);
  color: #3B82F6;
}

.stat-card-green .stat-icon {
  background: rgba(34, 197, 94, 0.1);
  color: #22C55E;
}

.stat-card-orange .stat-icon {
  background: rgba(245, 158, 11, 0.1);
  color: #F59E0B;
}

.stat-card-purple .stat-icon {
  background: rgba(139, 92, 246, 0.1);
  color: #8B5CF6;
}

.stat-trend {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 13px;
  font-weight: 600;
  padding: 4px 8px;
  border-radius: var(--radius-full);
}

.stat-trend.positive {
  color: var(--aria-success);
  background: rgba(34, 197, 94, 0.1);
}

.stat-trend.negative {
  color: var(--aria-danger);
  background: rgba(239, 68, 68, 0.1);
}

.stat-trend.neutral {
  color: var(--aria-text-muted);
  background: rgba(148, 163, 184, 0.1);
}

.stat-content {
  margin-bottom: 16px;
}

.stat-value {
  font-size: 32px;
  font-weight: 700;
  color: var(--aria-text-primary);
  line-height: 1;
  margin-bottom: 8px;
  letter-spacing: -0.5px;
}

.stat-label {
  font-size: 14px;
  font-weight: 500;
  color: var(--aria-text-secondary);
  margin-bottom: 4px;
}

.stat-description {
  font-size: 12px;
  color: var(--aria-text-muted);
}

.stat-progress {
  height: 4px;
  background: var(--aria-content-bg-tertiary);
  border-radius: 2px;
  overflow: hidden;
}

.progress-bar {
  height: 100%;
  background: var(--aria-content-bg-tertiary);
  border-radius: 2px;
}

.progress-fill {
  height: 100%;
  border-radius: 2px;
  transition: width 0.6s cubic-bezier(0.4, 0, 0.2, 1);
}

.stat-card-blue .progress-fill {
  background: linear-gradient(90deg, #3B82F6 0%, #60A5FA 100%);
}

.stat-card-green .progress-fill {
  background: linear-gradient(90deg, #22C55E 0%, #4ADE80 100%);
}

.stat-card-orange .progress-fill {
  background: linear-gradient(90deg, #F59E0B 0%, #FBBF24 100%);
}

.stat-card-purple .progress-fill {
  background: linear-gradient(90deg, #8B5CF6 0%, #A78BFA 100%);
}

/* ============================================
   Content Row
   ============================================ */
.content-row {
  margin-bottom: 0;
}

/* Card Headers */
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0;
  border: none;
  background: transparent;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 10px;
}

.header-icon {
  font-size: 20px;
  color: var(--aria-primary);
}

.header-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--aria-text-primary);
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

/* Chart Card */
.chart-card {
  min-height: 420px;
}

.chart-container {
  width: 100%;
  height: 340px;
}

/* ============================================
   Activity Card
   ============================================ */
.activity-card {
  min-height: 420px;
}

.activity-list {
  max-height: 340px;
  overflow-y: auto;
  padding: 4px;
}

.activity-item {
  display: flex;
  gap: 12px;
  padding: 16px;
  border-radius: var(--aria-radius);
  transition: all var(--aria-transition-fast);
  cursor: pointer;
  border-bottom: 1px solid var(--aria-border-primary);
}

.activity-item:last-child {
  border-bottom: none;
}

.activity-item:hover {
  background: var(--aria-content-bg-tertiary);
}

.activity-icon-wrapper {
  width: 40px;
  height: 40px;
  border-radius: var(--aria-radius);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  flex-shrink: 0;
}

.activity-icon-wrapper.node {
  background: rgba(59, 130, 246, 0.1);
  color: #3B82F6;
}

.activity-icon-wrapper.route {
  background: rgba(34, 197, 94, 0.1);
  color: #22C55E;
}

.activity-icon-wrapper.config {
  background: rgba(245, 158, 11, 0.1);
  color: #F59E0B;
}

.activity-content {
  flex: 1;
  min-width: 0;
}

.activity-text {
  font-size: 14px;
  color: var(--aria-text-primary);
  margin-bottom: 4px;
  line-height: 1.5;
}

.activity-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.activity-time {
  font-size: 12px;
  color: var(--aria-text-muted);
}

/* ============================================
   Region Card
   ============================================ */
.region-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.region-item {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 12px;
  background: var(--aria-content-bg-tertiary);
  border-radius: var(--aria-radius);
  transition: all var(--aria-transition-fast);
}

.region-item:hover {
  background: var(--aria-content-bg-secondary);
  border: 1px solid var(--aria-border-hover);
}

.region-info {
  display: flex;
  align-items: center;
  gap: 12px;
  flex: 1;
}

.region-icon {
  font-size: 24px;
}

.region-details {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.region-name {
  font-size: 14px;
  font-weight: 500;
  color: var(--aria-text-primary);
}

.region-nodes {
  font-size: 12px;
  color: var(--aria-text-muted);
}

.region-stats {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 6px;
  min-width: 100px;
}

.region-percentage {
  font-size: 12px;
  font-weight: 600;
  color: var(--aria-text-secondary);
}

/* ============================================
   Quick Actions Card
   ============================================ */
.quick-actions-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
}

.quick-action-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 20px;
  background: var(--aria-content-bg-tertiary);
  border-radius: var(--aria-radius);
  cursor: pointer;
  transition: all var(--aria-transition-base);
  border: 1px solid var(--aria-border-primary);
}

.quick-action-item:hover {
  background: var(--aria-content-bg-secondary);
  border-color: var(--aria-border-hover);
  transform: translateY(-2px);
  box-shadow: var(--aria-shadow);
}

.action-icon {
  width: 48px;
  height: 48px;
  border-radius: var(--aria-radius-md);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 24px;
}

.action-name {
  font-size: 13px;
  font-weight: 500;
  color: var(--aria-text-primary);
  text-align: center;
}

/* ============================================
   Health Card
   ============================================ */
.health-metrics {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.health-item {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.metric-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.metric-name {
  font-size: 13px;
  font-weight: 500;
  color: var(--aria-text-secondary);
}

.metric-value {
  font-size: 14px;
  font-weight: 600;
}

.metric-value.status-good {
  color: var(--aria-success);
}

.metric-value.status-warning {
  color: var(--aria-warning);
}

.metric-value.status-danger {
  color: var(--aria-danger);
}

/* ============================================
   Responsive
   ============================================ */
@media (max-width: 768px) {
  .dashboard {
    gap: 16px;
  }

  .stat-value {
    font-size: 28px;
  }

  .chart-card,
  .activity-card {
    min-height: auto;
  }

  .chart-container {
    height: 280px;
  }

  .activity-list {
    max-height: 280px;
  }

  .quick-actions-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}
</style>
