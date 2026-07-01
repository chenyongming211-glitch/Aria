<template>
  <div class="settings">
    <el-card>
      <template #header>
        <div class="card-header">
          <div>
            <h3>{{ t('settingsBackup.title') }}</h3>
            <p class="card-subtitle">
              {{ t('settingsBackup.subtitle') }}
            </p>
          </div>
        </div>
      </template>

      <el-alert
        :title="t('settingsBackup.productizationTitle')"
        type="info"
        :closable="false"
        show-icon
      >
        <template #default>
          {{ t('settingsBackup.productizationBody') }}
        </template>
      </el-alert>

      <div class="backup-layout">
        <el-card class="backup-card">
          <template #header>
            <div class="card-header">
              <span>{{ t('settingsBackup.configurationBackups') }}</span>
            </div>
          </template>

          <div class="backup-actions">
            <p class="backup-description">
              {{ t('settingsBackup.description') }}
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
                {{ t('settingsBackup.uploadBackup') }}
              </el-button>
              <el-button type="primary" :disabled="!canWriteSettings || creating" :loading="creating" @click="createBackup">
                {{ t('settingsBackup.createBackup') }}
              </el-button>
            </div>
          </div>
        </el-card>

        <el-card class="backup-card">
          <template #header>
            <div class="card-header">
              <span>{{ t('settingsBackup.recentBackups') }}</span>
              <el-button text @click="loadBackups">
                {{ t('common.refresh') }}
              </el-button>
            </div>
          </template>

          <el-table :data="backupHistory" v-loading="loading" style="width: 100%">
            <el-table-column prop="filename" :label="t('common.filename')" min-width="280" />
            <el-table-column prop="size" :label="t('common.size')" width="120" />
            <el-table-column prop="created_at" :label="t('common.createdAt')" width="180" />
            <el-table-column prop="created_by" :label="t('common.createdBy')" width="120" />
            <el-table-column :label="t('common.actions')" width="180">
              <template #default="{ row }">
                <el-button
                  size="small"
                  :disabled="!canWriteSettings || downloadingIds.has(row.id)"
                  :loading="downloadingIds.has(row.id)"
                  @click="downloadBackup(row)"
                >
                  {{ t('common.download') }}
                </el-button>
                <el-button
                  size="small"
                  type="warning"
                  :disabled="!canWriteSettings || restoringIds.has(row.id)"
                  :loading="restoringIds.has(row.id)"
                  @click="openRestoreDialog(row)"
                >
                  {{ t('common.restore') }}
                </el-button>
                <el-popconfirm
                  :title="t('settingsBackup.deleteConfirm')"
                  @confirm="deleteBackup(row.id)"
                >
                  <template #reference>
                    <el-button
                      size="small"
                      type="danger"
                      :disabled="!canWriteSettings || deletingIds.has(row.id)"
                      :loading="deletingIds.has(row.id)"
                    >
                      {{ t('common.delete') }}
                    </el-button>
                  </template>
                </el-popconfirm>
              </template>
            </el-table-column>
          </el-table>

          <el-empty v-if="!loading && backupHistory.length === 0" :description="t('settingsBackup.noBackups')" />
        </el-card>
      </div>
    </el-card>

    <el-dialog
      v-if="restoreDialogVisible"
      v-model="restoreDialogVisible"
      :title="t('settingsBackup.restoreBackup')"
      width="680px"
    >
      <div class="restore-dialog">
        <el-alert
          :title="t('settingsBackup.restoreWarning')"
          type="warning"
          :closable="false"
          show-icon
        />

        <div class="restore-section">
          <div class="restore-label">{{ t('settingsBackup.backupFile') }}</div>
          <div class="restore-value">{{ selectedBackup?.filename || selectedBackup?.id || '-' }}</div>
        </div>

        <div class="restore-section">
          <div class="restore-label">{{ t('settingsBackup.restoreScope') }}</div>
          <el-checkbox-group v-model="restoreTables" class="restore-table-grid" @change="previewRestore">
            <el-checkbox v-for="table in restoreTableOptions" :key="table.value" :label="table.value">
              {{ t(table.labelKey) }}
            </el-checkbox>
          </el-checkbox-group>
        </div>

        <div class="restore-section">
          <div class="restore-label">{{ t('settingsBackup.dryRunPreview') }}</div>
          <el-skeleton v-if="previewing" :rows="3" animated />
          <div v-else-if="restorePlan" class="restore-plan">
            <div v-for="table in restorePlanRows" :key="table.name" class="restore-plan-row">
              <span>{{ table.name }}</span>
              <strong>{{ table.count }}</strong>
            </div>
          </div>
          <el-empty v-else :description="t('settingsBackup.noPreview')" />
        </div>

        <div class="restore-section">
          <div class="restore-label">{{ t('settingsBackup.confirmationPhrase') }}</div>
          <el-input v-model="restoreConfirm" :placeholder="restoreRequiredConfirm" />
        </div>
      </div>

      <template #footer>
        <el-button @click="restoreDialogVisible = false">
          {{ t('common.cancel') }}
        </el-button>
        <el-button :loading="previewing" @click="previewRestore">
          {{ t('settingsBackup.previewAgain') }}
        </el-button>
        <el-button
          type="warning"
          :disabled="!restoreCanApply"
          :loading="selectedBackup ? restoringIds.has(selectedBackup.id) : false"
          @click="applyRestore"
        >
          {{ t('settingsBackup.applyRestore') }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useUserStore } from '@/stores'
import { useSettingsApi } from '@/composables/useSettingsApi'
import { t } from '@/i18n'

const userStore = useUserStore()

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
  { value: 'tenants', labelKey: 'settingsBackup.tables.tenants' },
  { value: 'roles', labelKey: 'settingsBackup.tables.roles' },
  { value: 'users', labelKey: 'settingsBackup.tables.users' },
  { value: 'tokens', labelKey: 'settingsBackup.tables.tokens' },
  { value: 'nodes', labelKey: 'settingsBackup.tables.nodes' },
  { value: 'ip_groups', labelKey: 'settingsBackup.tables.ipGroups' },
  { value: 'ip_group_members', labelKey: 'settingsBackup.tables.ipGroupMembers' },
  { value: 'acl_rules', labelKey: 'settingsBackup.tables.aclRules' },
  { value: 'qos_rules', labelKey: 'settingsBackup.tables.qosRules' },
  { value: 'blacklist_rules', labelKey: 'settingsBackup.tables.blacklistRules' }
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

const showCatchError = (err, fallback) => {
  if (err instanceof Error && err.message) {
    ElMessage.error(err.message)
    return
  }
  if (typeof err === 'string' && err) {
    ElMessage.error(err)
    return
  }
  ElMessage.error(fallback)
}

const loadBackups = async () => {
  loading.value = true
  try {
    backupHistory.value = await useSettingsApi.listBackups()
  } catch (error) {
    showCatchError(error, t('settingsBackup.loadFailed'))
  } finally {
    loading.value = false
  }
}

const createBackup = async () => {
  creating.value = true
  try {
    const created = await useSettingsApi.createBackup()
    backupHistory.value = [created, ...backupHistory.value.filter(item => item.id !== created.id)]
    ElMessage.success(t('settingsBackup.createSuccess'))
  } catch (error) {
    showCatchError(error, t('settingsBackup.createFailed'))
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
    ElMessage.success(t('settingsBackup.uploadSuccess'))
  } catch (error) {
    showCatchError(error, t('settingsBackup.uploadFailed'))
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
    showCatchError(error, t('settingsBackup.downloadFailed'))
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
    ElMessage.success(t('settingsBackup.deleteSuccess'))
  } catch (error) {
    showCatchError(error, t('settingsBackup.deleteFailed'))
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
    showCatchError(error, t('settingsBackup.previewFailed'))
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
    ElMessage.success(t('settingsBackup.restoreSuccess').replace('{summary}', summary ? `: ${summary}` : ''))
    restoreDialogVisible.value = false
    await loadBackups()
  } catch (error) {
    showCatchError(error, t('settingsBackup.restoreFailed'))
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
