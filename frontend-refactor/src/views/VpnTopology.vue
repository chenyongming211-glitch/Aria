<!-- src/views/VpnTopology.vue -->
<template>
  <div class="vpn-topology">
    <el-card>
      <template #header>
        <div class="card-header">
          <h3>{{ t('nav.vpnTopology') }}</h3>
          <div class="header-actions">
            <el-button type="primary" @click="refreshTopology">
              <el-icon><Refresh /></el-icon>
              {{ t('common.refresh') }}
            </el-button>
            <el-button @click="downloadConfig">
              <el-icon><Download /></el-icon>
              Download Config
            </el-button>
          </div>
        </div>
      </template>

      <div class="topology-container">
        <el-alert
          title="Interactive VPN topology visualization coming soon"
          type="info"
          :closable="false"
          show-icon
        />
        <div class="topology-placeholder">
          <el-empty :description="t('common.loading')" />
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { Refresh, Download } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { t } from '@/i18n'

const refreshTopology = () => {
  ElMessage.success('Topology refreshed')
  // In a real app, this would fetch the latest topology data
}

const downloadConfig = () => {
  ElMessage.info('Download configuration functionality coming soon')
}
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

.topology-container {
  min-height: 500px;
}

.topology-placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 400px;
}
</style>