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
    vi.useRealTimers()
    reloadMock = vi.fn()
  })

  afterEach(() => {
    vi.useRealTimers()
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

  it('stops the version watcher interval', async () => {
    vi.useFakeTimers()
    api.get.mockResolvedValue({ data: { version: '0.2.86' } })

    const appStore = useAppStore()

    appStore.startVersionWatcher(1000)
    await vi.advanceTimersByTimeAsync(1000)
    expect(api.get).toHaveBeenCalledTimes(1)

    appStore.stopVersionWatcher()
    await vi.advanceTimersByTimeAsync(3000)

    expect(api.get).toHaveBeenCalledTimes(1)
  })

  it('keeps working when localStorage methods are unavailable', () => {
    const originalLocalStorage = globalThis.localStorage
    try {
      globalThis.localStorage = {}

      const appStore = useAppStore()

      expect(appStore.lang).toBe('zh')
      expect(() => appStore.setLang('en')).not.toThrow()
      expect(appStore.lang).toBe('en')
    } finally {
      globalThis.localStorage = originalLocalStorage
    }
  })
})
