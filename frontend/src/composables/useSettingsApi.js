import api from './useApi'
import { API_ENDPOINTS } from '@/config/api'

export const useSettingsApi = {
  listBackups: async () => {
    const response = await api.get(API_ENDPOINTS.SETTINGS.BACKUPS)
    return response.data?.data || response.data || []
  },

  createBackup: async () => {
    const response = await api.post(API_ENDPOINTS.SETTINGS.BACKUPS)
    return response.data?.data || response.data
  },

  uploadBackup: async (file) => {
    const formData = new FormData()
    formData.append('file', file)
    const response = await api.post(API_ENDPOINTS.SETTINGS.BACKUP_UPLOAD, formData, {
      headers: {
        'Content-Type': 'multipart/form-data'
      }
    })
    return response.data?.data || response.data
  },

  deleteBackup: async (backupId) => {
    const response = await api.delete(API_ENDPOINTS.SETTINGS.BACKUP_DETAIL(backupId))
    return response.data?.data || response.data
  },

  restoreBackup: async (backupId) => {
    const response = await api.post(API_ENDPOINTS.SETTINGS.BACKUP_RESTORE(backupId))
    return response.data?.data || response.data
  },

  downloadBackupUrl: (backupId) => API_ENDPOINTS.SETTINGS.BACKUP_DOWNLOAD(backupId)
}
