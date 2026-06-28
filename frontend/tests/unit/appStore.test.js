import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

vi.mock('@/composables/useApi', () => ({
  default: {
    get: vi.fn()
  }
}))

import useAppStore from '@/stores/app'
import api from '@/composables/useApi'

describe('app store version handling', () => {
  let reloadMock

  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    reloadMock = vi.fn()
  })

  it('reloads the page when a later version is detected', async () => {
    api.get
      .mockResolvedValueOnce({ data: { version: '0.2.86' } })
      .mockResolvedValueOnce({ data: { version: '0.2.87' } })

    const appStore = useAppStore()

    await appStore.fetchVersion()
    await appStore.fetchVersion({ reloadOnChange: true, reload: reloadMock })

    expect(appStore.version).toBe('0.2.87')
    expect(reloadMock).toHaveBeenCalledTimes(1)
  })
})
