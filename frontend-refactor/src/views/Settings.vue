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
        <el-tab-pane label="General" name="general">
          <el-form :model="generalForm" label-width="120px" style="max-width: 600px;">
            <el-form-item label="System Name">
              <el-input v-model="generalForm.systemName" />
            </el-form-item>
            <el-form-item label="Default Region">
              <el-select v-model="generalForm.defaultRegion" placeholder="Select region">
                <el-option label="Shanghai (sh)" value="sh" />
                <el-option label="Beijing (bj)" value="bj" />
                <el-option label="Guangzhou (gz)" value="gz" />
                <el-option label="Hong Kong (hk)" value="hk" />
              </el-select>
            </el-form-item>
            <el-form-item label="Timezone">
              <el-select v-model="generalForm.timezone" placeholder="Select timezone">
                <el-option label="UTC+8 (China Standard Time)" value="CST" />
                <el-option label="UTC+0 (GMT)" value="GMT" />
                <el-option label="UTC-5 (EST)" value="EST" />
              </el-select>
            </el-form-item>
            <el-form-item label="Theme">
              <el-radio-group v-model="generalForm.theme">
                <el-radio label="light">Light</el-radio>
                <el-radio label="dark">Dark</el-radio>
                <el-radio label="auto">Auto</el-radio>
              </el-radio-group>
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="saveGeneralSettings">Save</el-button>
              <el-button @click="resetGeneralSettings">Reset</el-button>
            </el-form-item>
          </el-form>
        </el-tab-pane>

        <el-tab-pane label="Network" name="network">
          <el-form :model="networkForm" label-width="150px" style="max-width: 600px;">
            <el-form-item label="Listen Port">
              <el-input-number v-model="networkForm.listenPort" :min="1" :max="65535" />
            </el-form-item>
            <el-form-item label="MTU Size">
              <el-input-number v-model="networkForm.mtu" :min="576" :max="9000" />
            </el-form-item>
            <el-form-item label="Encryption">
              <el-switch v-model="networkForm.encryption" />
            </el-form-item>
            <el-form-item label="Compression">
              <el-switch v-model="networkForm.compression" />
            </el-form-item>
            <el-form-item label="Enable NAT Traversal">
              <el-switch v-model="networkForm.enableNatTraversal" />
            </el-form-item>
            <el-form-item label="STUN Server">
              <el-input v-model="networkForm.stunServer" placeholder="stun:stun.example.com:19302" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="saveNetworkSettings">Save</el-button>
              <el-button @click="resetNetworkSettings">Reset</el-button>
            </el-form-item>
          </el-form>
        </el-tab-pane>

        <el-tab-pane label="Security" name="security">
          <el-form :model="securityForm" label-width="150px" style="max-width: 600px;">
            <el-form-item label="Enable Firewall">
              <el-switch v-model="securityForm.enableFirewall" />
            </el-form-item>
            <el-form-item label="Enable DDOS Protection">
              <el-switch v-model="securityForm.enableDdosProtection" />
            </el-form-item>
            <el-form-item label="Connection Limit">
              <el-slider v-model="securityForm.connectionLimit" :min="1" :max="10000" show-input />
            </el-form-item>
            <el-form-item label="Rate Limit (requests/min)">
              <el-slider v-model="securityForm.rateLimit" :min="10" :max="10000" show-input />
            </el-form-item>
            <el-form-item label="Block Suspicious IPs">
              <el-switch v-model="securityForm.blockSuspiciousIps" />
            </el-form-item>
            <el-form-item label="Two-factor Auth">
              <el-switch v-model="securityForm.twoFactorAuth" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="saveSecuritySettings">Save</el-button>
              <el-button @click="resetSecuritySettings">Reset</el-button>
            </el-form-item>
          </el-form>
        </el-tab-pane>

        <el-tab-pane label="Notifications" name="notifications">
          <el-form :model="notificationForm" label-width="150px" style="max-width: 600px;">
            <el-form-item label="Email Notifications">
              <el-switch v-model="notificationForm.emailEnabled" />
            </el-form-item>
            <el-form-item label="Email Address">
              <el-input v-model="notificationForm.emailAddress" :disabled="!notificationForm.emailEnabled" />
            </el-form-item>
            <el-form-item label="Webhook Notifications">
              <el-switch v-model="notificationForm.webhookEnabled" />
            </el-form-item>
            <el-form-item label="Webhook URL">
              <el-input v-model="notificationForm.webhookUrl" :disabled="!notificationForm.webhookEnabled" />
            </el-form-item>
            <el-form-item label="Notification Types">
              <el-checkbox-group v-model="notificationForm.notificationTypes">
                <el-checkbox label="alerts">Alerts</el-checkbox>
                <el-checkbox label="warnings">Warnings</el-checkbox>
                <el-checkbox label="maintenance">Maintenance Events</el-checkbox>
                <el-checkbox label="security">Security Events</el-checkbox>
              </el-checkbox-group>
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="saveNotificationSettings">Save</el-button>
              <el-button @click="resetNotificationSettings">Reset</el-button>
              <el-button @click="testNotifications" style="float: right;">Test Notifications</el-button>
            </el-form-item>
          </el-form>
        </el-tab-pane>

        <el-tab-pane label="Backup & Restore" name="backup">
          <el-card class="backup-card">
            <template #header>
              <div class="card-header">
                <span>Configuration Backup</span>
              </div>
            </template>
            <p>Create a backup of your system configuration:</p>
            <el-button type="primary" @click="createBackup" icon="Document">
              Create Backup
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
                  <el-button size="small" type="primary" @click="restoreFromBackup(row)">Restore</el-button>
                  <el-popconfirm
                    title="Are you sure to delete this backup?"
                    @confirm="deleteBackup(row.id)"
                  >
                    <template #reference>
                      <el-button size="small" type="danger">Delete</el-button>
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
import { ref } from 'vue'
import { UploadFilled } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'

const activeTab = ref('general')

// Form models
const generalForm = ref({
  systemName: 'Aria Controller',
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

const uploadUrl = ref('/api/settings/upload-backup')
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