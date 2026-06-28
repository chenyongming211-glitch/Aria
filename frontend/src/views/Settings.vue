<template>
  <div class="settings">
    <el-card>
      <template #header>
        <div class="card-header">
          <div>
            <h3>{{ currentLang === 'zh' ? 'Settings' : 'Settings' }}</h3>
            <p class="card-subtitle">
              {{ currentLang === 'zh'
                ? '当前仅保留真实可用的配置备份能力；其余系统设置项暂未开放。'
                : 'Only real backup management is currently enabled. Other system settings remain unavailable.' }}
            </p>
          </div>
        </div>
      </template>

      <el-alert
        :title="currentLang === 'zh' ? '产品化收口中' : 'Productization In Progress'"
        type="info"
        :closable="false"
        show-icon
      >
        <template #default>
          {{ currentLang === 'zh'
            ? '通用、网络、安全、通知等设置项此前为本地假动作，现已从页面下线，避免误导。'
            : 'General, network, security, and notification settings were placeholder-only and have been removed from this page to avoid misleading behavior.' }}
        </template>
      </el-alert>

      <div class="backup-layout">
        <el-card class="backup-card">
          <template #header>
            <div class="card-header">
              <span>{{ currentLang === 'zh' ? '配置备份' : 'Configuration Backups' }}</span>
            </div>
          </template>

          <div class="backup-actions">
            <p class="backup-description">
              {{ currentLang === 'zh'
                ? '创建当前控制面配置快照，包含租户、用户、角色、节点及策略数据。'
                : 'Create a snapshot of the current control-plane configuration, including tenants, users, roles, nodes, and policy data.' }}
            </p>
            <div class="backup-action-buttons">
              <input
                ref="uploadInputRef"
                class="backup-upload-input"
                type="file"
                accept=".json,application/json"
                @change="handleUploadBackup"
              />
              <el-button :disabled="!canWriteSettings || uploading" :loading="uploading" @click="triggerUploadBackup">
                {{ currentLang === 'zh' ? '上传备份' : 'Upload Backup' }}
              </el-button>
              <el-button type="primary" :disabled="!canWriteSettings || creating" :loading="creating" @click="createBackup">
                {{ currentLang === 'zh' ? '立即创建备份' : 'Create Backup' }}
              </el-button>
            </div>
          </div>
        </el-card>

        <el-card class="backup-card">
          <template #header>
            <div class="card-header">
              <span>{{ currentLang === 'zh' ? '最近备份' : 'Recent Backups' }}</span>
              <el-button text @click="loadBackups">
                {{ currentLang === 'zh' ? '刷新' : 'Refresh' }}
              </el-button>
            </div>
          </template>

          <el-table :data="backupHistory" v-loading="loading" style="width: 100%">
            <el-table-column prop="filename" :label="currentLang === 'zh' ? '文件名' : 'Filename'" min-width="280" />
            <el-table-column prop="size" :label="currentLang === 'zh' ? '大小' : 'Size'" width="120" />
            <el-table-column prop="created_at" :label="currentLang === 'zh' ? '创建时间' : 'Created At'" width="180" />
            <el-table-column prop="created_by" :label="currentLang === 'zh' ? '创建人' : 'Created By'" width="120" />
            <el-table-column :label="currentLang === 'zh' ? '操作' : 'Actions'" width="180">
              <template #default="{ row }">
                <el-button
                  size="small"
                  :disabled="!canWriteSettings || downloadingIds.has(row.id)"
                  :loading="downloadingIds.has(row.id)"
                  @click="downloadBackup(row)"
                >
                  {{ currentLang === 'zh' ? '下载' : 'Download' }}
                </el-button>
                <el-button
                  size="small"
                  type="warning"
                  :disabled="!canWriteSettings || restoringIds.has(row.id)"
                  :loading="restoringIds.has(row.id)"
                  @click="openRestoreDialog(row)"
                >
                  {{ currentLang === 'zh' ? '恢复' : 'Restore' }}
                </el-button>
                <el-popconfirm
                  :title="currentLang === 'zh' ? '确定删除这个备份吗？' : 'Delete this backup?'"
                  @confirm="deleteBackup(row.id)"
                >
                  <template #reference>
                    <el-button
                      size="small"
                      type="danger"
                      :disabled="!canWriteSettings || deletingIds.has(row.id)"
                      :loading="deletingIds.has(row.id)"
                    >
                      {{ currentLang === 'zh' ? '删除' : 'Delete' }}
                    </el-button>
                  </template>
                </el-popconfirm>
              </template>
            </el-table-column>
          </el-table>

          <el-empty v-if="!loading && backupHistory.length === 0" :description="currentLang === 'zh' ? '暂无备份' : 'No backups yet'" />
        </el-card>
      </div>
    </el-card>

    <el-dialog
      v-if="restoreDialogVisible"
      v-model="restoreDialogVisible"
      :title="currentLang === 'zh' ? '恢复备份' : 'Restore Backup'"
      width="680px"
    >
      <div class="restore-dialog">
        <el-alert
          :title="currentLang === 'zh' ? '恢复会覆盖所选控制面配置表' : 'Restore replaces selected control-plane configuration tables'"
          type="warning"
          :closable="false"
          show-icon
        />

        <div class="restore-section">
          <div class="restore-label">{{ currentLang === 'zh' ? '备份文件' : 'Backup File' }}</div>
          <div class="restore-value">{{ selectedBackup?.filename || selectedBackup?.id || '-' }}</div>
        </div>

        <div class="restore-section">
          <div class="restore-label">{{ currentLang === 'zh' ? '恢复范围' : 'Restore Scope' }}</div>
          <el-checkbox-group v-model="restoreTables" class="restore-table-grid" @change="previewRestore">
            <el-checkbox v-for="table in restoreTableOptions" :key="table.value" :label="table.value">
              {{ currentLang === 'zh' ? table.zh : table.en }}
            </el-checkbox>
          </el-checkbox-group>
        </div>

        <div class="restore-section">
          <div class="restore-label">{{ currentLang === 'zh' ? 'Dry-run 预览' : 'Dry-run Preview' }}</div>
          <el-skeleton v-if="previewing" :rows="3" animated />
          <div v-else-if="restorePlan" class="restore-plan">
            <div v-for="table in restorePlanRows" :key="table.name" class="restore-plan-row">
              <span>{{ table.name }}</span>
              <strong>{{ table.count }}</strong>
            </div>
          </div>
          <el-empty v-else :description="currentLang === 'zh' ? '暂无预览' : 'No preview yet'" />
        </div>

        <div class="restore-section">
          <div class="restore-label">{{ currentLang === 'zh' ? '确认短语' : 'Confirmation Phrase' }}</div>
          <el-input v-model="restoreConfirm" :placeholder="restoreRequiredConfirm" />
        </div>
      </div>

      <template #footer>
        <el-button @click="restoreDialogVisible = false">
          {{ currentLang === 'zh' ? '取消' : 'Cancel' }}
        </el-button>
        <el-button :loading="previewing" @click="previewRestore">
          {{ currentLang === 'zh' ? '重新预览' : 'Preview Again' }}
        </el-button>
        <el-button
          type="warning"
          :disabled="!restoreCanApply"
          :loading="selectedBackup ? restoringIds.has(selectedBackup.id) : false"
          @click="applyRestore"
        >
          {{ currentLang === 'zh' ? '确认恢复' : 'Apply Restore' }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useAppStore, useUserStore } from '@/stores'
import { useSettingsApi } from '@/composables/useSettingsApi'

const appStore = useAppStore()
const userStore = useUserStore()

const currentLang = computed(() => appStore.lang)
const canWriteSettings = computed(() => userStore.user?.role === 'super_admin')

const loading = ref(false)
const creating = ref(false)
const uploading = ref(false)
const backupHistory = ref([])
const downloadingIds = ref(new Set())
const deletingIds = ref(new Set())
const restoringIds = ref(new Set())
const uploadInputRef = ref(null)
const restoreDialogVisible = ref(false)
const selectedBackup = ref(null)
const restoreRequiredConfirm = 'RESTORE ARIA CONFIG'
const restoreConfirm = ref('')
const restorePlan = ref(null)
const previewing = ref(false)
const restoreTableOptions = [
  { value: 'tenants', zh: '租户', en: 'Tenants' },
  { value: 'roles', zh: '角色', en: 'Roles' },
  { value: 'users', zh: '用户', en: 'Users' },
  { value: 'tokens', zh: '令牌', en: 'Tokens' },
  { value: 'nodes', zh: '节点', en: 'Nodes' },
  { value: 'ip_groups', zh: 'IP Group', en: 'IP Groups' },
  { value: 'ip_group_members', zh: 'IP Group 成员', en: 'IP Group Members' },
  { value: 'acl_rules', zh: 'ACL 规则', en: 'ACL Rules' },
  { value: 'qos_rules', zh: 'QoS 规则', en: 'QoS Rules' },
  { value: 'blacklist_rules', zh: '黑名单规则', en: 'Blacklist Rules' }
]
const restoreTables = ref(restoreTableOptions.map(table => table.value))

const restorePlanRows = computed(() => {
  const counts = restorePlan.value?.table_counts
  if (!counts || typeof counts !== 'object') {
    return []
  }
  return Object.entries(counts).map(([name, count]) => ({ name, count }))
})
const restoreCanApply = computed(() => (
  Boolean(selectedBackup.value?.id) &&
  restoreConfirm.value.trim() === restoreRequiredConfirm &&
  restoreTables.value.length > 0 &&
  !previewing.value
))

const formatRestoreSummary = (result) => {
  const restoredTables = result?.restored_tables
  if (!restoredTables || typeof restoredTables !== 'object') {
    return ''
  }

  const entries = Object.entries(restoredTables)
    .filter(([, count]) => Number(count) > 0)
    .map(([table, count]) => `${table}: ${count}`)

  return entries.join(', ')
}

const loadBackups = async () => {
  loading.value = true
  try {
    backupHistory.value = await useSettingsApi.listBackups()
  } catch (error) {
    ElMessage.error(error.message || (currentLang.value === 'zh' ? '加载备份失败' : 'Failed to load backups'))
  } finally {
    loading.value = false
  }
}

const createBackup = async () => {
  creating.value = true
  try {
    const created = await useSettingsApi.createBackup()
    backupHistory.value = [created, ...backupHistory.value.filter(item => item.id !== created.id)]
    ElMessage.success(currentLang.value === 'zh' ? '备份创建成功' : 'Backup created successfully')
  } catch (error) {
    ElMessage.error(error.message || (currentLang.value === 'zh' ? '创建备份失败' : 'Failed to create backup'))
  } finally {
    creating.value = false
  }
}

const triggerUploadBackup = () => {
  uploadInputRef.value?.click()
}

const handleUploadBackup = async (event) => {
  const file = event?.target?.files?.[0]
  if (!file) {
    return
  }

  uploading.value = true
  try {
    const uploaded = await useSettingsApi.uploadBackup(file)
    backupHistory.value = [uploaded, ...backupHistory.value.filter(item => item.id !== uploaded.id)]
    ElMessage.success(currentLang.value === 'zh' ? '备份上传成功' : 'Backup uploaded successfully')
  } catch (error) {
    ElMessage.error(error.message || (currentLang.value === 'zh' ? '上传备份失败' : 'Failed to upload backup'))
  } finally {
    uploading.value = false
    if (event?.target) {
      event.target.value = ''
    }
  }
}

const downloadBackup = async (backup) => {
  const next = new Set(downloadingIds.value)
  next.add(backup.id)
  downloadingIds.value = next
  try {
    await useSettingsApi.downloadBackup(backup)
  } catch (error) {
    ElMessage.error(error.message || (currentLang.value === 'zh' ? '下载备份失败' : 'Failed to download backup'))
  } finally {
    const reset = new Set(downloadingIds.value)
    reset.delete(backup.id)
    downloadingIds.value = reset
  }
}

const deleteBackup = async (backupId) => {
  const next = new Set(deletingIds.value)
  next.add(backupId)
  deletingIds.value = next
  try {
    await useSettingsApi.deleteBackup(backupId)
    backupHistory.value = backupHistory.value.filter(item => item.id !== backupId)
    ElMessage.success(currentLang.value === 'zh' ? '备份已删除' : 'Backup deleted')
  } catch (error) {
    ElMessage.error(error.message || (currentLang.value === 'zh' ? '删除备份失败' : 'Failed to delete backup'))
  } finally {
    const reset = new Set(deletingIds.value)
    reset.delete(backupId)
    deletingIds.value = reset
  }
}

const openRestoreDialog = async (backup) => {
  selectedBackup.value = backup
  restoreConfirm.value = ''
  restoreTables.value = restoreTableOptions.map(table => table.value)
  restorePlan.value = null
  restoreDialogVisible.value = true
  await previewRestore()
}

const previewRestore = async () => {
  if (!selectedBackup.value?.id || restoreTables.value.length === 0) {
    restorePlan.value = null
    return
  }
  previewing.value = true
  try {
    restorePlan.value = await useSettingsApi.restoreBackupDryRun(selectedBackup.value.id, restoreTables.value)
  } catch (error) {
    restorePlan.value = null
    ElMessage.error(error.message || (currentLang.value === 'zh' ? '恢复预览失败' : 'Failed to preview restore'))
  } finally {
    previewing.value = false
  }
}

const applyRestore = async () => {
  const backupId = selectedBackup.value?.id
  if (!backupId) {
    return
  }
  const next = new Set(restoringIds.value)
  next.add(backupId)
  restoringIds.value = next
  try {
    const restored = await useSettingsApi.restoreBackup(backupId, {
      confirm: restoreConfirm.value.trim(),
      tables: restoreTables.value
    })
    const summary = formatRestoreSummary(restored)
    ElMessage.success(
      currentLang.value === 'zh'
        ? `备份恢复完成${summary ? `：${summary}` : ''}`
        : `Backup restored successfully${summary ? `: ${summary}` : ''}`
    )
    restoreDialogVisible.value = false
    await loadBackups()
  } catch (error) {
    ElMessage.error(error.message || (currentLang.value === 'zh' ? '恢复备份失败' : 'Failed to restore backup'))
  } finally {
    const reset = new Set(restoringIds.value)
    reset.delete(backupId)
    restoringIds.value = reset
  }
}

onMounted(() => {
  loadBackups()
})
</script>

<style scoped>
.settings {
  padding: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
}

.card-subtitle {
  margin: 6px 0 0;
  color: #606266;
  font-size: 13px;
}

.backup-layout {
  margin-top: 20px;
  display: grid;
  gap: 20px;
}

.backup-card {
  margin-bottom: 0;
}

.backup-actions {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
}

.backup-action-buttons {
  display: flex;
  gap: 12px;
  align-items: center;
}

.backup-upload-input {
  display: none;
}

.backup-description {
  margin: 0;
  color: #606266;
}

.restore-dialog {
  display: grid;
  gap: 18px;
}

.restore-section {
  display: grid;
  gap: 8px;
}

.restore-label {
  font-size: 13px;
  font-weight: 600;
  color: #475569;
}

.restore-value {
  color: #1f2937;
  word-break: break-all;
}

.restore-table-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 8px 12px;
}

.restore-plan {
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  overflow: hidden;
}

.restore-plan-row {
  display: flex;
  justify-content: space-between;
  padding: 8px 12px;
  border-bottom: 1px solid #eef2f7;
  color: #475569;
}

.restore-plan-row:last-child {
  border-bottom: none;
}
</style>
