<!-- src/views/Settings.vue -->
<template>
  <div class="settings">
    <el-card>
      <template #header>
        <div class="card-header">
          <h3>System Settings</h3>
        </div>
      </template>

      <el-tabs v-model="activeTab" class="settings-tabs">
        <el-tab-pane :label="labels.general" name="general">
          <el-form :model="generalForm" label-width="120px" style="max-width: 600px;">
            <el-form-item :label="labels.systemName">
              <el-input v-model="generalForm.systemName" />
            </el-form-item>
            <el-form-item :label="labels.language">
              <el-radio-group v-model="generalForm.language" @change="handleLanguageChange">
                <el-radio label="zh">简体中文</el-radio>
                <el-radio label="en">English</el-radio>
              </el-radio-group>
            </el-form-item>
            <el-form-item :label="labels.defaultRegion">
              <el-select v-model="generalForm.defaultRegion" :placeholder="currentLang === 'zh' ? '选择区域' : 'Select region'">
                <el-option label="Shanghai (sh)" value="sh" />
                <el-option label="Beijing (bj)" value="bj" />
                <el-option label="Guangzhou (gz)" value="gz" />
                <el-option label="Hong Kong (hk)" value="hk" />
              </el-select>
            </el-form-item>
            <el-form-item :label="labels.timezone">
              <el-select v-model="generalForm.timezone" :placeholder="currentLang === 'zh' ? '选择时区' : 'Select timezone'">
                <el-option label="UTC+8 (中国标准时间)" value="CST" />
                <el-option label="UTC+0 (GMT)" value="GMT" />
                <el-option label="UTC-5 (EST)" value="EST" />
              </el-select>
            </el-form-item>
            <el-form-item :label="labels.theme">
              <el-radio-group v-model="generalForm.theme">
                <el-radio label="light">{{ labels.light }}</el-radio>
                <el-radio label="dark">{{ labels.dark }}</el-radio>
                <el-radio label="auto">{{ labels.auto }}</el-radio>
              </el-radio-group>
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :disabled="!canWriteSettings" @click="saveGeneralSettings">{{ labels.save }}</el-button>
              <el-button :disabled="!canWriteSettings" @click="resetGeneralSettings">{{ labels.reset }}</el-button>
            </el-form-item>
          </el-form>
        </el-tab-pane>

        <el-tab-pane :label="labels.network" name="network">
          <el-form :model="networkForm" label-width="150px" style="max-width: 600px;">
            <el-form-item :label="labels.listenPort">
              <el-input-number v-model="networkForm.listenPort" :min="1" :max="65535" />
            </el-form-item>
            <el-form-item :label="labels.mtuSize">
              <el-input-number v-model="networkForm.mtu" :min="576" :max="9000" />
            </el-form-item>
            <el-form-item :label="labels.encryption">
              <el-switch v-model="networkForm.encryption" />
            </el-form-item>
            <el-form-item :label="labels.compression">
              <el-switch v-model="networkForm.compression" />
            </el-form-item>
            <el-form-item :label="labels.enableNatTraversal">
              <el-switch v-model="networkForm.enableNatTraversal" />
            </el-form-item>
            <el-form-item :label="labels.stunServer">
              <el-input v-model="networkForm.stunServer" placeholder="stun:stun.example.com:19302" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :disabled="!canWriteSettings" @click="saveNetworkSettings">{{ labels.save }}</el-button>
              <el-button :disabled="!canWriteSettings" @click="resetNetworkSettings">{{ labels.reset }}</el-button>
            </el-form-item>
          </el-form>
        </el-tab-pane>

        <el-tab-pane :label="labels.security" name="security">
          <el-form :model="securityForm" label-width="150px" style="max-width: 600px;">
            <el-form-item :label="labels.enableFirewall">
              <el-switch v-model="securityForm.enableFirewall" />
            </el-form-item>
            <el-form-item :label="labels.enableDdosProtection">
              <el-switch v-model="securityForm.enableDdosProtection" />
            </el-form-item>
            <el-form-item :label="labels.connectionLimit">
              <el-slider v-model="securityForm.connectionLimit" :min="1" :max="10000" show-input />
            </el-form-item>
            <el-form-item :label="labels.rateLimit">
              <el-slider v-model="securityForm.rateLimit" :min="10" :max="10000" show-input />
            </el-form-item>
            <el-form-item :label="labels.blockSuspiciousIps">
              <el-switch v-model="securityForm.blockSuspiciousIps" />
            </el-form-item>
            <el-form-item :label="labels.twoFactorAuth">
              <el-switch v-model="securityForm.twoFactorAuth" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :disabled="!canWriteSettings" @click="saveSecuritySettings">{{ labels.save }}</el-button>
              <el-button :disabled="!canWriteSettings" @click="resetSecuritySettings">{{ labels.reset }}</el-button>
            </el-form-item>
          </el-form>
        </el-tab-pane>

        <el-tab-pane :label="labels.notifications" name="notifications">
          <el-form :model="notificationForm" label-width="150px" style="max-width: 600px;">
            <el-form-item :label="currentLang === 'zh' ? '邮件通知' : 'Email Notifications'">
              <el-switch v-model="notificationForm.emailEnabled" />
            </el-form-item>
            <el-form-item :label="labels.emailAddress">
              <el-input v-model="notificationForm.emailAddress" :disabled="!notificationForm.emailEnabled" />
            </el-form-item>
            <el-form-item :label="currentLang === 'zh' ? 'Webhook 通知' : 'Webhook Notifications'">
              <el-switch v-model="notificationForm.webhookEnabled" />
            </el-form-item>
            <el-form-item :label="labels.webhookUrl">
              <el-input v-model="notificationForm.webhookUrl" :disabled="!notificationForm.webhookEnabled" />
            </el-form-item>
            <el-form-item :label="labels.alertTypes">
              <el-checkbox-group v-model="notificationForm.notificationTypes">
                <el-checkbox :label="labels.nodeOffline">{{ currentLang === 'zh' ? '节点离线' : 'Node Offline' }}</el-checkbox>
                <el-checkbox :label="labels.highLatency">{{ currentLang === 'zh' ? '高延迟' : 'High Latency' }}</el-checkbox>
                <el-checkbox :label="labels.packetLoss">{{ currentLang === 'zh' ? '丢包' : 'Packet Loss' }}</el-checkbox>
                <el-checkbox :label="currentLang === 'zh' ? '安全事件' : 'Security Events'">Security Events</el-checkbox>
              </el-checkbox-group>
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :disabled="!canWriteSettings" @click="saveNotificationSettings">{{ labels.save }}</el-button>
              <el-button :disabled="!canWriteSettings" @click="resetNotificationSettings">{{ labels.reset }}</el-button>
              <el-button :disabled="!canWriteSettings" @click="testNotifications" style="float: right;">{{ currentLang === 'zh' ? '测试通知' : 'Test Notifications' }}</el-button>
            </el-form-item>
          </el-form>
        </el-tab-pane>

        <el-tab-pane :label="labels.backup" name="backup">
          <el-card class="backup-card">
            <template #header>
              <div class="card-header">
                <span>{{ currentLang === 'zh' ? '配置备份' : 'Configuration Backup' }}</span>
              </div>
            </template>
            <p>{{ currentLang === 'zh' ? '创建系统配置备份：' : 'Create a backup of your system configuration:' }}</p>
            <el-button type="primary" :disabled="!canWriteSettings" @click="createBackup" icon="Document">
              {{ labels.backupNow }}
            </el-button>
          </el-card>

          <el-card class="backup-card">
            <template #header>
              <div class="card-header">
                <span>Configuration Restore</span>
              </div>
            </template>
            <p>Restore system configuration from a backup file:</p>
            <el-upload
              class="upload-demo"
              drag
              :action="uploadUrl"
              :on-success="handleUploadSuccess"
              :on-error="handleUploadError"
              :headers="uploadHeaders"
              :show-file-list="false"
              accept=".json,.ariaconfig"
              :disabled="!canWriteSettings"
            >
              <el-icon class="el-icon--upload"><UploadFilled /></el-icon>
              <div class="el-upload__text">
                Drop backup file here or <em>click to upload</em>
              </div>
            </el-upload>
          </el-card>

          <el-card class="backup-card">
            <template #header>
              <div class="card-header">
                <span>Recent Backups</span>
              </div>
            </template>
            <el-table :data="backupHistory" style="width: 100%">
              <el-table-column prop="filename" label="Filename" width="250" />
              <el-table-column prop="size" label="Size" width="120" />
              <el-table-column prop="createdAt" label="Created At" width="180" />
              <el-table-column prop="actions" label="Actions">
                <template #default="{ row }">
                  <el-button size="small" @click="downloadBackup(row)">Download</el-button>
                  <el-button size="small" type="primary" :disabled="!canWriteSettings" @click="restoreFromBackup(row)">Restore</el-button>
                  <el-popconfirm
                    title="Are you sure to delete this backup?"
                    @confirm="deleteBackup(row.id)"
                  >
                    <template #reference>
                      <el-button size="small" type="danger" :disabled="!canWriteSettings">Delete</el-button>
                    </template>
                  </el-popconfirm>
                </template>
              </el-table-column>
            </el-table>
          </el-card>
        </el-tab-pane>
      </el-tabs>
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { UploadFilled } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useAppStore } from '@/stores'
import { usePermission } from '@/composables/usePermission'

const appStore = useAppStore()
const { hasPermission } = usePermission()

const currentLang = computed(() => appStore.lang)
const canWriteSettings = computed(() => hasPermission('settings:write'))

const labels = computed(() => ({
  general: currentLang.value === 'zh' ? '通用设置' : 'General',
  network: currentLang.value === 'zh' ? '网络' : 'Network',
  security: currentLang.value === 'zh' ? '安全' : 'Security',
  notifications: currentLang.value === 'zh' ? '通知' : 'Notifications',
  backup: currentLang.value === 'zh' ? '备份与恢复' : 'Backup & Recovery',
  systemName: currentLang.value === 'zh' ? '系统名称' : 'System Name',
  defaultRegion: currentLang.value === 'zh' ? '默认区域' : 'Default Region',
  timezone: currentLang.value === 'zh' ? '时区' : 'Timezone',
  theme: currentLang.value === 'zh' ? '主题' : 'Theme',
  language: currentLang.value === 'zh' ? '语言' : 'Language',
  light: currentLang.value === 'zh' ? '浅色' : 'Light',
  dark: currentLang.value === 'zh' ? '深色' : 'Dark',
  auto: currentLang.value === 'zh' ? '自动' : 'Auto',
  save: currentLang.value === 'zh' ? '保存' : 'Save',
  reset: currentLang.value === 'zh' ? '重置' : 'Reset',
  listenPort: currentLang.value === 'zh' ? '监听端口' : 'Listen Port',
  mtuSize: currentLang.value === 'zh' ? 'MTU 大小' : 'MTU Size',
  encryption: currentLang.value === 'zh' ? '加密' : 'Encryption',
  compression: currentLang.value === 'zh' ? '压缩' : 'Compression',
  enableNatTraversal: currentLang.value === 'zh' ? '启用 NAT 穿透' : 'Enable NAT Traversal',
  stunServer: currentLang.value === 'zh' ? 'STUN 服务器' : 'STUN Server',
  enableFirewall: currentLang.value === 'zh' ? '启用防火墙' : 'Enable Firewall',
  enableDdosProtection: currentLang.value === 'zh' ? '启用 DDoS 防护' : 'Enable DDOS Protection',
  connectionLimit: currentLang.value === 'zh' ? '连接数限制' : 'Connection Limit',
  rateLimit: currentLang.value === 'zh' ? '速率限制 (请求/分)' : 'Rate Limit (requests/min)',
  blockSuspiciousIps: currentLang.value === 'zh' ? '阻止可疑 IP' : 'Block Suspicious IPs',
  twoFactorAuth: currentLang.value === 'zh' ? '双因素认证' : 'Two-factor Auth',
  emailEnabled: currentLang.value === 'zh' ? '启用邮件通知' : 'Enable Email',
  emailAddress: currentLang.value === 'zh' ? '邮箱地址' : 'Email Address',
  webhookEnabled: currentLang.value === 'zh' ? '启用 Webhook' : 'Enable Webhook',
  webhookUrl: currentLang.value === 'zh' ? 'Webhook URL' : 'Webhook URL',
  slackEnabled: currentLang.value === 'zh' ? '启用 Slack' : 'Enable Slack',
  slackWebhook: currentLang.value === 'zh' ? 'Slack Webhook' : 'Slack Webhook',
  alertTypes: currentLang.value === 'zh' ? '告警类型' : 'Alert Types',
  nodeOffline: currentLang.value === 'zh' ? '节点离线' : 'Node Offline',
  highLatency: currentLang.value === 'zh' ? '高延迟' : 'High Latency',
  packetLoss: currentLang.value === 'zh' ? '丢包' : 'Packet Loss',
  backupNow: currentLang.value === 'zh' ? '立即备份' : 'Backup Now',
  restore: currentLang.value === 'zh' ? '恢复' : 'Restore',
  delete: currentLang.value === 'zh' ? '删除' : 'Delete',
  download: currentLang.value === 'zh' ? '下载' : 'Download',
  filename: currentLang.value === 'zh' ? '文件名' : 'Filename',
  size: currentLang.value === 'zh' ? '大小' : 'Size',
  createdAt: currentLang.value === 'zh' ? '创建时间' : 'Created At',
  actions: currentLang.value === 'zh' ? '操作' : 'Actions',
}))

const handleLanguageChange = (lang) => {
  appStore.setLang(lang)
  ElMessage.success(currentLang.value === 'zh' ? '语言已切换' : 'Language changed')
}

const activeTab = ref('general')

// Form models
const generalForm = ref({
  systemName: 'Aria Controller',
  language: localStorage.getItem('aria-lang') || 'zh',
  defaultRegion: 'sh',
  timezone: 'CST',
  theme: 'light'
})

const networkForm = ref({
  listenPort: 8080,
  mtu: 1420,
  encryption: true,
  compression: true,
  enableNatTraversal: true,
  stunServer: 'stun:stun.cloudflare.com:3478'
})

const securityForm = ref({
  enableFirewall: true,
  enableDdosProtection: true,
  connectionLimit: 1000,
  rateLimit: 1000,
  blockSuspiciousIps: true,
  twoFactorAuth: false
})

const notificationForm = ref({
  emailEnabled: true,
  emailAddress: 'admin@example.com',
  webhookEnabled: false,
  webhookUrl: '',
  notificationTypes: ['alerts', 'warnings']
})

const backupHistory = ref([
  { id: 1, filename: 'aria-config-backup-20240115-1430.json', size: '2.4 MB', createdAt: '2024-01-15 14:30:25' },
  { id: 2, filename: 'aria-config-backup-20240114-1015.json', size: '2.3 MB', createdAt: '2024-01-14 10:15:42' },
  { id: 3, filename: 'aria-config-backup-20240113-0900.json', size: '2.2 MB', createdAt: '2024-01-13 09:00:18' }
])

const uploadUrl = ref('/api/v2/settings/backups/upload')
const uploadHeaders = ref({
  'Authorization': `Bearer ${localStorage.getItem('aria_token')}`
})

const saveGeneralSettings = () => {
  ElMessage.success('General settings saved successfully!')
}

const resetGeneralSettings = () => {
  generalForm.value = {
    systemName: 'Aria Controller',
    defaultRegion: 'sh',
    timezone: 'CST',
    theme: 'light'
  }
  ElMessage.info('General settings reset to default')
}

const saveNetworkSettings = () => {
  ElMessage.success('Network settings saved successfully!')
}

const resetNetworkSettings = () => {
  networkForm.value = {
    listenPort: 8080,
    mtu: 1420,
    encryption: true,
    compression: true,
    enableNatTraversal: true,
    stunServer: 'stun:stun.cloudflare.com:3478'
  }
  ElMessage.info('Network settings reset to default')
}

const saveSecuritySettings = () => {
  ElMessage.success('Security settings saved successfully!')
}

const resetSecuritySettings = () => {
  securityForm.value = {
    enableFirewall: true,
    enableDdosProtection: true,
    connectionLimit: 1000,
    rateLimit: 1000,
    blockSuspiciousIps: true,
    twoFactorAuth: false
  }
  ElMessage.info('Security settings reset to default')
}

const saveNotificationSettings = () => {
  ElMessage.success('Notification settings saved successfully!')
}

const resetNotificationSettings = () => {
  notificationForm.value = {
    emailEnabled: true,
    emailAddress: 'admin@example.com',
    webhookEnabled: false,
    webhookUrl: '',
    notificationTypes: ['alerts', 'warnings']
  }
  ElMessage.info('Notification settings reset to default')
}

const testNotifications = () => {
  ElMessage.info('Testing notifications...')
  // In a real app, this would trigger a test notification
}

const createBackup = () => {
  ElMessage.info('Creating backup...')
  // In a real app, this would call an API to create a backup
}

const handleUploadSuccess = () => {
  ElMessage.success('Backup uploaded successfully!')
}

const handleUploadError = () => {
  ElMessage.error('Failed to upload backup!')
}

const downloadBackup = (backup) => {
  ElMessage.info(`Downloading ${backup.filename}`)
  // In a real app, this would download the backup file
}

const restoreFromBackup = (backup) => {
  ElMessageBox.confirm(
    `Are you sure you want to restore from ${backup.filename}? This will overwrite current configuration.`,
    'Confirm Restore',
    {
      confirmButtonText: 'Restore',
      cancelButtonText: 'Cancel',
      type: 'warning'
    }
  ).then(() => {
    ElMessage.success(`Restoring from ${backup.filename}`)
    // In a real app, this would call an API to restore from backup
  }).catch(() => {
    // Cancelled
  })
}

const deleteBackup = (id) => {
  backupHistory.value = backupHistory.value.filter(item => item.id !== id)
  ElMessage.success('Backup deleted')
}
</script>

<style scoped>
.settings {
  padding: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.settings-tabs {
  margin-top: 20px;
}

.backup-card {
  margin-bottom: 20px;
}
</style>