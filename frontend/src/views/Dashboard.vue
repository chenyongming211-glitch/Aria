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
          </div>
          <div class="stat-content">
            <div class="stat-value">{{ stat.value }}</div>
            <div class="stat-label">{{ stat.label }}</div>
            <div class="stat-description">{{ stat.description }}</div>
          </div>
          <div class="stat-progress" v-if="stat.progress !== undefined">
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
          <div ref="trafficChartRef" class="chart-container" v-loading="trafficLoading"></div>
          <div v-if="trafficError" class="chart-error">
            <el-alert :title="trafficError" type="warning" :closable="false" show-icon />
          </div>
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
            <el-empty v-if="activities.length === 0" description="No recent activity" :image-size="60" />
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
            <el-empty v-if="regions.length === 0" description="No region data" :image-size="60" />
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
          <div v-if="healthError" class="health-error">
            <el-alert title="数据不可用" type="warning" :closable="false" show-icon />
          </div>
          <div v-else class="health-metrics" v-loading="healthLoading">
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
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
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
  DataAnalysis,
  Warning
} from '@element-plus/icons-vue'
import * as echarts from 'echarts'
import { useRouter } from 'vue-router'
import { useMonitorApi } from '@/composables/useMonitorApi'
import { useTenantApi } from '@/composables/useTenantApi'
import { useTenantChangeReload } from '@/composables/useTenantChangeReload'

const router = useRouter()
const trafficLoading = ref(false)
const healthLoading = ref(false)
const timeRange = ref('24h')
const trafficChartRef = ref(null)
const trafficError = ref('')
const healthError = ref(false)
let chartInstance = null
let healthTimer = null

// 节点统计
const nodesData = ref({
  total: 'N/A',
  online: 'N/A',
  routes: 'N/A',
  bandwidth: 'N/A'
})

// 健康数据
const healthData = ref(null)

// 活动数据
const activities = ref([])

// 区域数据（从节点列表聚合）
const regions = ref([])

// 缓存节点列表用于区域聚合
let cachedNodes = []

// 统计卡片 — 无硬编码趋势
const stats = computed(() => {
  const total = nodesData.value.total
  const online = nodesData.value.online
  const routes = nodesData.value.routes
  const bw = nodesData.value.bandwidth

  return [
    {
      label: 'Total Nodes',
      value: total,
      description: 'Across all regions',
      color: 'blue',
      icon: Monitor,
      progress: typeof total === 'number' ? Math.min(total * 5, 100) : 0
    },
    {
      label: 'Online Nodes',
      value: online,
      description: 'Currently active',
      color: 'green',
      icon: CircleCheck,
      progress: (typeof total === 'number' && typeof online === 'number' && total > 0)
        ? Math.round(online / total * 100)
        : 0
    },
    {
      label: 'Advertised Routes',
      value: routes,
      description: 'Network routes',
      color: 'orange',
      icon: Position,
      progress: typeof routes === 'number' ? Math.min(routes * 3, 100) : 0
    },
    {
      label: 'Bandwidth',
      value: bw === 'N/A' ? 'N/A' : bw + ' Mbps',
      description: 'Peak throughput',
      color: 'purple',
      icon: TrendCharts,
      progress: (typeof bw === 'number') ? Math.min(bw / 10, 100) : 0
    }
  ]
})

// 健康指标
const healthMetrics = computed(() => {
  if (!healthData.value) return []

  const h = healthData.value
  const onlineRate = h.node_online_rate ?? 0
  const syncRate = h.sync_success_rate ?? 0
  const alerts = h.active_alerts_count ?? 0
  const failedCmds = h.failed_commands_count ?? 0

  return [
    {
      name: 'Node Online Rate',
      value: onlineRate.toFixed(1) + '%',
      percentage: Math.min(Math.round(onlineRate), 100),
      color: onlineRate >= 80 ? '#22C55E' : onlineRate >= 50 ? '#F59E0B' : '#EF4444',
      statusClass: onlineRate >= 80 ? 'status-good' : onlineRate >= 50 ? 'status-warning' : 'status-danger'
    },
    {
      name: 'Sync Success Rate',
      value: syncRate.toFixed(1) + '%',
      percentage: Math.min(Math.round(syncRate), 100),
      color: syncRate >= 90 ? '#22C55E' : syncRate >= 70 ? '#F59E0B' : '#EF4444',
      statusClass: syncRate >= 90 ? 'status-good' : syncRate >= 70 ? 'status-warning' : 'status-danger'
    },
    {
      name: 'Active Alerts',
      value: String(alerts),
      percentage: Math.min(alerts * 10, 100),
      color: alerts === 0 ? '#22C55E' : alerts <= 3 ? '#F59E0B' : '#EF4444',
      statusClass: alerts === 0 ? 'status-good' : alerts <= 3 ? 'status-warning' : 'status-danger'
    },
    {
      name: 'Failed Commands',
      value: String(failedCmds),
      percentage: Math.min(failedCmds * 15, 100),
      color: failedCmds === 0 ? '#22C55E' : failedCmds <= 2 ? '#F59E0B' : '#EF4444',
      statusClass: failedCmds === 0 ? 'status-good' : failedCmds <= 2 ? 'status-warning' : 'status-danger'
    }
  ]
})

// 快速操作
const quickActions = [
  {
    name: 'Add Node',
    icon: Plus,
    bgColor: 'rgba(59, 130, 246, 0.1)',
    iconColor: '#3B82F6',
    handler: () => router.push('/nodes')
  },
  {
    name: 'Create Route',
    icon: Position,
    bgColor: 'rgba(34, 197, 94, 0.1)',
    iconColor: '#22C55E',
    handler: () => router.push('/routing')
  },
  {
    name: 'View Logs',
    icon: DataAnalysis,
    bgColor: 'rgba(245, 158, 11, 0.1)',
    iconColor: '#F59E0B',
    handler: () => router.push('/monitoring')
  },
  {
    name: 'System Config',
    icon: Setting,
    bgColor: 'rgba(139, 92, 246, 0.1)',
    iconColor: '#8B5CF6',
    handler: () => router.push('/settings')
  }
]

// 区域颜色映射
const regionColors = ['#3B82F6', '#22C55E', '#F59E0B', '#06B6D4', '#8B5CF6', '#EF4444', '#EC4899']
const regionIcons = { cn: '🇨🇳', us: '🇺🇸', eu: '🇪🇺', jp: '🇯🇵', sg: '🇸🇬', hk: '🇭🇰', default: '🌐' }

// 从节点列表按 region 聚合
const aggregateRegions = (nodes) => {
  if (!nodes || nodes.length === 0) {
    regions.value = []
    return
  }
  const map = {}
  nodes.forEach(n => {
    const r = (n.region || 'unknown').toLowerCase()
    if (!map[r]) map[r] = 0
    map[r]++
  })
  const total = nodes.length
  const entries = Object.entries(map).sort((a, b) => b[1] - a[1])
  regions.value = entries.map(([name, count], i) => ({
    name: name.charAt(0).toUpperCase() + name.slice(1),
    icon: regionIcons[name] || regionIcons.default,
    nodes: count,
    percentage: Math.round(count / total * 100),
    color: regionColors[i % regionColors.length]
  }))
}

// 事件类型映射
const eventTypeMap = {
  alert_fired: { type: 'config', icon: Warning, tag: 'Alert', tagType: 'danger' },
  alert_resolved: { type: 'node', icon: CircleCheck, tag: 'Resolved', tagType: 'success' },
  node_registered: { type: 'node', icon: Monitor, tag: 'New', tagType: 'success' },
  node_online: { type: 'node', icon: Monitor, tag: 'Online', tagType: 'success' },
  node_offline: { type: 'node', icon: Monitor, tag: 'Offline', tagType: 'danger' },
  command_sent: { type: 'config', icon: Setting, tag: 'Command', tagType: 'info' },
  config_updated: { type: 'config', icon: Setting, tag: 'Config', tagType: 'warning' },
  default: { type: 'node', icon: Monitor, tag: 'Event', tagType: 'info' }
}

const formatEventTime = (ts) => {
  if (!ts) return ''
  const date = new Date(ts)
  if (isNaN(date.getTime())) return ts
  const now = Date.now()
  const diff = Math.floor((now - date.getTime()) / 1000)
  if (diff < 60) return 'just now'
  if (diff < 3600) return Math.floor(diff / 60) + ' min ago'
  if (diff < 86400) return Math.floor(diff / 3600) + ' hours ago'
  return Math.floor(diff / 86400) + ' days ago'
}

const normalizeAdvertisedRoutes = (routes) => {
  if (Array.isArray(routes)) return routes
  if (!routes || typeof routes !== 'string') return []
  try {
    const parsed = JSON.parse(routes)
    return Array.isArray(parsed) ? parsed : []
  } catch (error) {
    console.warn('Invalid advertised_routes payload:', error)
    return []
  }
}

// 获取节点数据 + 统计卡片
const fetchNodesData = async () => {
  try {
    const nodes = await useTenantApi.getTenantNodes()
    cachedNodes = Array.isArray(nodes) ? nodes : []

    nodesData.value.total = cachedNodes.length
    nodesData.value.online = cachedNodes.filter(n =>
      (n.availability_status || n.status) === 'online'
    ).length

    let routeCount = 0
    cachedNodes.forEach(node => {
      routeCount += normalizeAdvertisedRoutes(node.advertised_routes).length
    })
    nodesData.value.routes = routeCount

    // 聚合区域
    aggregateRegions(cachedNodes)
  } catch (error) {
    console.error('获取节点数据失败:', error)
    nodesData.value.total = 'N/A'
    nodesData.value.online = 'N/A'
    nodesData.value.routes = 'N/A'
  }
}

// 获取流量数据 + 峰值带宽
const fetchTrafficData = async () => {
  trafficLoading.value = true
  trafficError.value = ''
  try {
    const data = await useMonitorApi.getTraffic(timeRange.value)
    if (data) {
      nodesData.value.bandwidth = data.peak_bandwidth_mbps != null
        ? Number(data.peak_bandwidth_mbps.toFixed(1))
        : 0
      renderTrafficChart(data)
    }
  } catch (error) {
    console.error('获取流量数据失败:', error)
    trafficError.value = 'Failed to load traffic data'
    nodesData.value.bandwidth = 'N/A'
  } finally {
    trafficLoading.value = false
  }
}

// 获取健康数据
const fetchHealthData = async () => {
  healthLoading.value = true
  healthError.value = false
  try {
    const data = await useMonitorApi.getHealth()
    healthData.value = data
  } catch (error) {
    console.error('获取健康数据失败:', error)
    healthError.value = true
  } finally {
    healthLoading.value = false
  }
}

// 获取活动列表
const fetchActivities = async () => {
  try {
    const data = await useMonitorApi.getEvents({ limit: 8 })
    const items = Array.isArray(data) ? data : (data?.items || [])
    activities.value = items.map(ev => {
      const mapping = eventTypeMap[ev.event_type] || eventTypeMap.default
      return {
        type: mapping.type,
        icon: mapping.icon,
        text: ev.message || ev.description || ev.event_type || 'Event',
        time: formatEventTime(ev.created_at || ev.timestamp),
        tag: mapping.tag,
        tagType: mapping.tagType
      }
    })
  } catch (error) {
    console.error('获取活动列表失败:', error)
    activities.value = []
  }
}

// 渲染流量图表
const renderTrafficChart = (data) => {
  if (!chartInstance) return

  const timestamps = data.timestamps || []
  const uploadBytes = data.upload_bytes || []
  const downloadBytes = data.download_bytes || []

  // 转换时间戳为可读标签
  const xAxis = timestamps.map(ts => {
    const d = new Date(ts * 1000)
    if (timeRange.value === '1h' || timeRange.value === '24h') {
      return d.getHours().toString().padStart(2, '0') + ':' + d.getMinutes().toString().padStart(2, '0')
    }
    return (d.getMonth() + 1) + '/' + d.getDate()
  })

  // bytes → Mbps (bytes * 8 / 1_000_000) 用于显示
  const toMbps = (bytes) => Number(((bytes * 8) / 1000000).toFixed(2))
  const upload = uploadBytes.map(toMbps)
  const download = downloadBytes.map(toMbps)

  const option = {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'axis',
      backgroundColor: 'rgba(255, 255, 255, 0.95)',
      borderColor: 'rgba(0, 0, 0, 0.1)',
      textStyle: { color: '#1E293B' },
      axisPointer: { lineStyle: { color: 'rgba(59, 130, 246, 0.2)' } }
    },
    legend: {
      data: ['Upload', 'Download'],
      textStyle: { color: '#475569' },
      top: 0
    },
    grid: { left: '3%', right: '4%', bottom: '3%', top: '15%', containLabel: true },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: xAxis,
      axisLine: { lineStyle: { color: 'rgba(0, 0, 0, 0.08)' } },
      axisLabel: { color: '#64748B' }
    },
    yAxis: {
      type: 'value',
      name: 'Mbps',
      nameTextStyle: { color: '#64748B' },
      axisLine: { lineStyle: { color: 'rgba(0, 0, 0, 0.08)' } },
      axisLabel: { color: '#64748B' },
      splitLine: { lineStyle: { color: 'rgba(0, 0, 0, 0.05)' } }
    },
    series: [
      {
        name: 'Upload',
        type: 'line',
        smooth: true,
        symbol: 'none',
        data: upload,
        lineStyle: { width: 3, color: '#3B82F6' },
        areaStyle: {
          color: {
            type: 'linear', x: 0, y: 0, x2: 0, y2: 1,
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
        data: download,
        lineStyle: { width: 3, color: '#22C55E' },
        areaStyle: {
          color: {
            type: 'linear', x: 0, y: 0, x2: 0, y2: 1,
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

// 初始化图表
const handleResize = () => {
  chartInstance?.resize()
}

const initTrafficChart = () => {
  if (trafficChartRef.value) {
    chartInstance = echarts.init(trafficChartRef.value)
    window.addEventListener('resize', handleResize)
  }
}

// 刷新活动
const refreshActivities = () => {
  fetchActivities()
}

// 监听时间范围变化
watch(timeRange, () => {
  fetchTrafficData()
})

// 60 秒自动刷新健康指标
const startHealthRefresh = () => {
  healthTimer = setInterval(() => {
    fetchHealthData()
  }, 60000)
}

const reloadTenantScopedData = async () => {
  cachedNodes = []
  activities.value = []
  regions.value = []
  await Promise.all([
    fetchNodesData(),
    fetchTrafficData(),
    fetchHealthData(),
    fetchActivities()
  ])
}

onMounted(async () => {
  initTrafficChart()
  await Promise.all([
    fetchNodesData(),
    fetchTrafficData(),
    fetchHealthData(),
    fetchActivities()
  ])
  startHealthRefresh()
})

useTenantChangeReload(reloadTenantScopedData)

onBeforeUnmount(() => {
  if (healthTimer) {
    clearInterval(healthTimer)
    healthTimer = null
  }
  window.removeEventListener('resize', handleResize)
  if (chartInstance) {
    chartInstance.dispose()
    chartInstance = null
  }
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

.stats-row {
  margin-bottom: 0;
}

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

.stat-card-blue .stat-icon { background: rgba(59, 130, 246, 0.1); color: #3B82F6; }
.stat-card-green .stat-icon { background: rgba(34, 197, 94, 0.1); color: #22C55E; }
.stat-card-orange .stat-icon { background: rgba(245, 158, 11, 0.1); color: #F59E0B; }
.stat-card-purple .stat-icon { background: rgba(139, 92, 246, 0.1); color: #8B5CF6; }

.stat-content { margin-bottom: 16px; }

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

.stat-progress { height: 4px; background: var(--aria-content-bg-tertiary); border-radius: 2px; overflow: hidden; }
.progress-bar { height: 100%; background: var(--aria-content-bg-tertiary); border-radius: 2px; }
.progress-fill { height: 100%; border-radius: 2px; transition: width 0.6s cubic-bezier(0.4, 0, 0.2, 1); }

.stat-card-blue .progress-fill { background: linear-gradient(90deg, #3B82F6 0%, #60A5FA 100%); }
.stat-card-green .progress-fill { background: linear-gradient(90deg, #22C55E 0%, #4ADE80 100%); }
.stat-card-orange .progress-fill { background: linear-gradient(90deg, #F59E0B 0%, #FBBF24 100%); }
.stat-card-purple .progress-fill { background: linear-gradient(90deg, #8B5CF6 0%, #A78BFA 100%); }

.content-row { margin-bottom: 0; }

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0;
  border: none;
  background: transparent;
}

.header-left { display: flex; align-items: center; gap: 10px; }
.header-icon { font-size: 20px; color: var(--aria-primary); }
.header-title { font-size: 16px; font-weight: 600; color: var(--aria-text-primary); }
.header-actions { display: flex; align-items: center; gap: 12px; }

.chart-card { min-height: 420px; }
.chart-container { width: 100%; height: 340px; }
.chart-error { margin-top: 8px; }

.activity-card { min-height: 420px; }
.activity-list { max-height: 340px; overflow-y: auto; padding: 4px; }

.activity-item {
  display: flex;
  gap: 12px;
  padding: 16px;
  border-radius: var(--aria-radius);
  transition: all var(--aria-transition-fast);
  cursor: pointer;
  border-bottom: 1px solid var(--aria-border-primary);
}
.activity-item:last-child { border-bottom: none; }
.activity-item:hover { background: var(--aria-content-bg-tertiary); }

.activity-icon-wrapper {
  width: 40px; height: 40px;
  border-radius: var(--aria-radius);
  display: flex; align-items: center; justify-content: center;
  font-size: 20px; flex-shrink: 0;
}
.activity-icon-wrapper.node { background: rgba(59, 130, 246, 0.1); color: #3B82F6; }
.activity-icon-wrapper.route { background: rgba(34, 197, 94, 0.1); color: #22C55E; }
.activity-icon-wrapper.config { background: rgba(245, 158, 11, 0.1); color: #F59E0B; }

.activity-content { flex: 1; min-width: 0; }
.activity-text { font-size: 14px; color: var(--aria-text-primary); margin-bottom: 4px; line-height: 1.5; }
.activity-meta { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.activity-time { font-size: 12px; color: var(--aria-text-muted); }

.region-list { display: flex; flex-direction: column; gap: 16px; }
.region-item {
  display: flex; align-items: center; gap: 16px; padding: 12px;
  background: var(--aria-content-bg-tertiary); border-radius: var(--aria-radius);
  transition: all var(--aria-transition-fast);
}
.region-item:hover { background: var(--aria-content-bg-secondary); border: 1px solid var(--aria-border-hover); }
.region-info { display: flex; align-items: center; gap: 12px; flex: 1; }
.region-icon { font-size: 24px; }
.region-details { display: flex; flex-direction: column; gap: 2px; }
.region-name { font-size: 14px; font-weight: 500; color: var(--aria-text-primary); }
.region-nodes { font-size: 12px; color: var(--aria-text-muted); }
.region-stats { display: flex; flex-direction: column; align-items: flex-end; gap: 6px; min-width: 100px; }
.region-percentage { font-size: 12px; font-weight: 600; color: var(--aria-text-secondary); }

.quick-actions-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 12px; }
.quick-action-item {
  display: flex; flex-direction: column; align-items: center; gap: 12px; padding: 20px;
  background: var(--aria-content-bg-tertiary); border-radius: var(--aria-radius);
  cursor: pointer; transition: all var(--aria-transition-base);
  border: 1px solid var(--aria-border-primary);
}
.quick-action-item:hover {
  background: var(--aria-content-bg-secondary); border-color: var(--aria-border-hover);
  transform: translateY(-2px); box-shadow: var(--aria-shadow);
}
.action-icon {
  width: 48px; height: 48px; border-radius: var(--aria-radius-md);
  display: flex; align-items: center; justify-content: center; font-size: 24px;
}
.action-name { font-size: 13px; font-weight: 500; color: var(--aria-text-primary); text-align: center; }

.health-metrics { display: flex; flex-direction: column; gap: 20px; }
.health-item { display: flex; flex-direction: column; gap: 8px; }
.metric-header { display: flex; justify-content: space-between; align-items: center; }
.metric-name { font-size: 13px; font-weight: 500; color: var(--aria-text-secondary); }
.metric-value { font-size: 14px; font-weight: 600; }
.metric-value.status-good { color: var(--aria-success); }
.metric-value.status-warning { color: var(--aria-warning); }
.metric-value.status-danger { color: var(--aria-danger); }
.health-error { padding: 8px 0; }

@media (max-width: 768px) {
  .dashboard { gap: 16px; }
  .stat-value { font-size: 28px; }
  .chart-card, .activity-card { min-height: auto; }
  .chart-container { height: 280px; }
  .activity-list { max-height: 280px; }
  .quick-actions-grid { grid-template-columns: repeat(2, 1fr); }
}
</style>
