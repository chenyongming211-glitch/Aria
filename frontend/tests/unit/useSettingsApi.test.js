import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { useSettingsApi } from '@/composables/useSettingsApi'

vi.mock('@/composables/useApi', () => ({
  default: {
    get: vi.fn(),
    post: vi.fn(),
    delete: vi.fn()
  }
}))

import api from '@/composables/useApi'

describe('useSettingsApi', () => {
  let createObjectURL
  let revokeObjectURL
  let click
  let createElement
  let realCreateElement
  let originalCreateObjectURL
  let originalRevokeObjectURL

  beforeEach(() => {
    vi.clearAllMocks()
    createObjectURL = vi.fn(() => 'blob:backup-url')
    revokeObjectURL = vi.fn()
    click = vi.fn()
    originalCreateObjectURL = window.URL.createObjectURL
    originalRevokeObjectURL = window.URL.revokeObjectURL
    window.URL.createObjectURL = createObjectURL
    window.URL.revokeObjectURL = revokeObjectURL

    realCreateElement = document.createElement.bind(document)
    createElement = vi.spyOn(document, 'createElement').mockImplementation((tagName) => {
      const element = realCreateElement(tagName)
      if (tagName === 'a') {
        element.click = click
      }
      return element
    })
  })

  afterEach(() => {
    createElement.mockRestore()
    window.URL.createObjectURL = originalCreateObjectURL
    window.URL.revokeObjectURL = originalRevokeObjectURL
  })

  it('downloads backups through the authenticated API client as a blob', async () => {
    const blob = new Blob(['{}'], { type: 'application/json' })
    api.get.mockResolvedValue({
      data: blob,
      headers: {
        'content-disposition': 'attachment; filename="backup-1.json"'
      }
    })

    const result = await useSettingsApi.downloadBackup({ id: 'backup-1', filename: 'fallback.json' })

    expect(api.get).toHaveBeenCalledWith('/v2/settings/backups/backup-1/download', {
      responseType: 'blob'
    })
    expect(createObjectURL).toHaveBeenCalledWith(blob)
    expect(click).toHaveBeenCalled()
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:backup-url')
    expect(result).toEqual({ id: 'backup-1', filename: 'backup-1.json' })
  })

  it('rejects download calls without a backup id', async () => {
    await expect(useSettingsApi.downloadBackup({})).rejects.toThrow('backup id is required')
    expect(api.get).not.toHaveBeenCalled()
  })
})
