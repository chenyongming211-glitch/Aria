<!-- src/views/VpnTopology.vue -->
<template>
  <div class="vpn-topology">
    <el-card>
      <template #header>
        <div class="card-header">
          <h3>{{ t('nav.vpnTopology') }}</h3>
          <div class="header-actions">
            <el-button type="primary" @click="refreshTopology" :loading="loading">
              <el-icon><Refresh /></el-icon>
              {{ t('common.refresh') }}
            </el-button>
          </div>
        </div>
      </template>

      <!-- 错误提示 -->
      <el-alert
        v-if="errorMsg"
        :title="errorMsg"
        type="error"
        :closable="true"
        show-icon
        class="topology-error"
        @close="errorMsg = ''"
      />

      <!-- 空状态 -->
      <div v-if="!loading && !errorMsg && isEmpty" class="topology-placeholder">
        <el-empty description="暂无节点数据" />
      </div>

      <!-- 拓扑图 -->
      <div
        v-show="!isEmpty || loading"
        ref="topologyChartRef"
        class="topology-chart"
        v-loading="loading"
      ></div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import * as echarts from 'echarts'
import { useMonitorApi } from '@/composables/useMonitorApi'
import { t } from '@/i18n'

const loading = ref(false)
const errorMsg = ref('')
const isEmpty = ref(true)
const topologyChartRef = ref(null)
let chartInstance = null

// 节点 id → hostname 映射（用于连接 tooltip）
let nodeMap = {}

const initChart = () => {
  if (topologyChartRef.value) {
    chartInstance = echarts.init(topologyChartRef.value)
    window.addEventListener('resize', handleResize)
  }
}

const handleResize = () => {
  chartInstance?.resize()
}

const fetchTopology = async () => {
  loading.value = true
  errorMsg.value = ''
  try {
    const data = await useMonitorApi.getTopology()
    const nodes = data?.nodes || []
    const links = data?.links || []

    if (nodes.length === 0) {
      isEmpty.value = true
      return
    }

    isEmpty.value = false
    nodeMap = {}
    nodes.forEach(n => { nodeMap[n.id] = n })

    renderChart(nodes, links)
  } catch (error) {
    console.error('获取拓扑数据失败:', error)
    errorMsg.value = 'Failed to load topology data'
    isEmpty.value = true
  } finally {
    loading.value = false
  }
}

const renderChart = (nodes, links) => {
  if (!chartInstance) return

  const chartNodes = nodes.map(n => ({
    id: n.id,
    name: n.hostname || n.id,
    symbolSize: 40,
    itemStyle: {
      color: n.status === 'online' ? '#22C55E' : '#EF4444'
    },
    label: {
      show: true,
      position: 'bottom',
      fontSize: 12,
      color: '#475569'
    },
    // 存储原始数据用于 tooltip
    _raw: n
  }))

  const chartLinks = links.map(l => {
    // 根据流量计算线宽 (bps -> 1-10px)
    // 假设 1Mbps 以上加粗，10Mbps 封顶
    const traffic = l.traffic || 0
    let width = 1.5
    if (l.status === 'active') {
      width = 1.5 + Math.min(Math.round(traffic / 1_000_000 * 2), 8)
    }

    return {
      source: l.source,
      target: l.target,
      lineStyle: l.status === 'active'
        ? { color: '#22C55E', width: width, type: 'solid', opacity: 0.8 }
        : { color: '#CBD5E1', width: 1.5, type: 'dashed', opacity: 0.4 },
      _status: l.status,
      _traffic: traffic
    }
  })

  const option = {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'item',
      formatter: (params) => {
        if (params.dataType === 'node') {
          const raw = params.data._raw
          if (!raw) return params.name
          const statusColor = raw.status === 'online' ? '#22C55E' : '#EF4444'
          return `
            <div style="font-weight:600;margin-bottom:4px">${raw.hostname || raw.id}</div>
            <div>Region: ${raw.region || 'N/A'}</div>
            <div>IP: ${raw.assigned_ip || 'N/A'}</div>
            <div>Status: <span style="color:${statusColor};font-weight:600">${raw.status}</span></div>
          `
        }
        if (params.dataType === 'edge') {
          const src = nodeMap[params.data.source]
          const tgt = nodeMap[params.data.target]
          const status = params.data._status || 'unknown'
          const traffic = params.data._traffic || 0
          const statusColor = status === 'active' ? '#22C55E' : '#94A3B8'
          
          let trafficDisplay = '0 bps'
          if (traffic >= 1_000_000) {
            trafficDisplay = (traffic / 1_000_000).toFixed(2) + ' Mbps'
          } else if (traffic >= 1_000) {
            trafficDisplay = (traffic / 1_000).toFixed(2) + ' Kbps'
          } else if (traffic > 0) {
            trafficDisplay = traffic.toFixed(0) + ' bps'
          }

          return `
            <div style="font-weight:600;margin-bottom:4px">Connection</div>
            <div>Source: ${src?.hostname || params.data.source}</div>
            <div>Target: ${tgt?.hostname || params.data.target}</div>
            <div>Status: <span style="color:${statusColor};font-weight:600">${status}</span></div>
            <div>Traffic: <span style="font-weight:600;color:#3B82F6">${trafficDisplay}</span></div>
          `
        }
        return ''
      }
    },
    series: [{
      type: 'graph',
      layout: 'force',
      roam: true,
      draggable: true,
      force: {
        repulsion: 300,
        edgeLength: [120, 200],
        gravity: 0.1
      },
      data: chartNodes,
      links: chartLinks,
      emphasis: {
        focus: 'adjacency',
        lineStyle: { width: 4 }
      },
      label: {
        show: true,
        position: 'bottom',
        fontSize: 12
      }
    }]
  }

  chartInstance.setOption(option, true)
}

const refreshTopology = () => {
  fetchTopology()
}

onMounted(() => {
  initChart()
  fetchTopology()
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleResize)
  if (chartInstance) {
    chartInstance.dispose()
    chartInstance = null
  }
})
</script>

<style scoped>
.vpn-topology {
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

.topology-error {
  margin-bottom: 16px;
}

.topology-chart {
  width: 100%;
  height: 600px;
}

.topology-placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 400px;
}
</style>
