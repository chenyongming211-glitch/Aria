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
                <el-button size="small" @click="downloadBackup(row)">
                  {{ currentLang === 'zh' ? '下载' : 'Download' }}
                </el-button>
                <el-popconfirm
                  :title="currentLang === 'zh' ? '恢复会覆盖当前配置，确定继续吗？' : 'Restore will replace the current configuration. Continue?'"
                  @confirm="restoreBackup(row.id)"
                >
                  <template #reference>
                    <el-button
                      size="small"
                      type="warning"
                      :disabled="!canWriteSettings || restoringIds.has(row.id)"
                      :loading="restoringIds.has(row.id)"
                    >
                      {{ currentLang === 'zh' ? '恢复' : 'Restore' }}
                    </el-button>
                  </template>
                </el-popconfirm>
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
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useAppStore } from '@/stores'
import { usePermission } from '@/composables/usePermission'
import { useSettingsApi } from '@/composables/useSettingsApi'

const appStore = useAppStore()
const { hasPermission } = usePermission()

const currentLang = computed(() => appStore.lang)
const canWriteSettings = computed(() => hasPermission('settings:write'))

const loading = ref(false)
const creating = ref(false)
const uploading = ref(false)
const backupHistory = ref([])
const deletingIds = ref(new Set())
const restoringIds = ref(new Set())
const uploadInputRef = ref(null)

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

const downloadBackup = (backup) => {
  const url = useSettingsApi.downloadBackupUrl(backup.id)
  if (typeof window !== 'undefined') {
    window.location.assign(url)
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

const restoreBackup = async (backupId) => {
  const next = new Set(restoringIds.value)
  next.add(backupId)
  restoringIds.value = next
  try {
    await useSettingsApi.restoreBackup(backupId)
    ElMessage.success(currentLang.value === 'zh' ? '备份恢复完成' : 'Backup restored successfully')
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
</style>
