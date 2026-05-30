import api from './useApi'
import { API_ENDPOINTS } from '@/config/api'

const contentDispositionFilename = (value) => {
  if (!value) return ''

  const encoded = /filename\*=UTF-8''([^;]+)/i.exec(value)
  if (encoded?.[1]) {
    try {
      return decodeURIComponent(encoded[1].replace(/^"|"$/g, ''))
    } catch {
      return encoded[1].replace(/^"|"$/g, '')
    }
  }

  const plain = /filename="?([^";]+)"?/i.exec(value)
  return plain?.[1]?.trim() || ''
}

const responseHeader = (headers, name) => {
  if (!headers) return ''
  if (typeof headers.get === 'function') {
    return headers.get(name) || headers.get(name.toLowerCase()) || ''
  }
  return headers[name] || headers[name.toLowerCase()] || ''
}

const saveBlob = (blob, filename) => {
  if (typeof window === 'undefined' || typeof document === 'undefined') {
    return
  }

  const url = window.URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  window.URL.revokeObjectURL(url)
}

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

  downloadBackup: async (backup) => {
    const backupId = typeof backup === 'string' ? backup : backup?.id
    if (!backupId) {
      throw new Error('backup id is required')
    }

    const response = await api.get(API_ENDPOINTS.SETTINGS.BACKUP_DOWNLOAD(backupId), {
      responseType: 'blob'
    })
    const filename = contentDispositionFilename(responseHeader(response.headers, 'content-disposition')) ||
      (typeof backup === 'object' ? backup.filename : '') ||
      `${backupId}.json`

    saveBlob(response.data, filename)
    return { id: backupId, filename }
  }
}
