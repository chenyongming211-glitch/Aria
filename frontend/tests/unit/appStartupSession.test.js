import { describe, it, expect, vi, beforeEach } from 'vitest'

const {
  mockApp,
  mockAppStore,
  mockUserStore,
  sessionDeferred
} = vi.hoisted(() => ({
  mockApp: {
    component: vi.fn(),
    use: vi.fn(function use() {
      return this
    }),
    mount: vi.fn()
  },
  mockAppStore: {
    fetchVersion: vi.fn(),
    startVersionWatcher: vi.fn()
  },
  sessionDeferred: {
    promise: null,
    resolve: null,
    reject: null
  },
  mockUserStore: {
    loadSession: vi.fn()
  }
}))

vi.mock('vue', async (importOriginal) => {
  const actual = await importOriginal()
  return {
    ...actual,
    createApp: vi.fn(() => mockApp)
  }
})

vi.mock('pinia', async (importOriginal) => {
  const actual = await importOriginal()
  return {
    ...actual,
    createPinia: vi.fn(() => ({}))
  }
})

vi.mock('@element-plus/icons-vue', () => ({
  House: {},
  Monitor: {}
}))

vi.mock('element-plus', () => ({
  default: {}
}))

vi.mock('@/router', () => ({
  default: {}
}))

vi.mock('@/stores/app', () => ({
  default: () => mockAppStore
}))

vi.mock('@/stores/user', () => ({
  default: () => mockUserStore
}))

vi.mock('@/App.vue', () => ({
  default: {}
}))

describe('application startup session hydration', () => {
  beforeEach(() => {
    vi.resetModules()
    mockApp.component.mockClear()
    mockApp.use.mockClear()
    mockApp.mount.mockClear()
    mockAppStore.fetchVersion.mockClear()
    mockAppStore.startVersionWatcher.mockClear()
    sessionDeferred.promise = new Promise((resolve, reject) => {
      sessionDeferred.resolve = resolve
      sessionDeferred.reject = reject
    })
    mockUserStore.loadSession.mockReset()
    mockUserStore.loadSession.mockReturnValue(sessionDeferred.promise)
    document.head.innerHTML = ''
    document.body.innerHTML = '<div id="app"></div>'
  })

  it('waits for cached user session hydration before mounting protected layouts', async () => {
    await import('@/main')

    expect(mockUserStore.loadSession).toHaveBeenCalledTimes(1)
    expect(mockApp.mount).not.toHaveBeenCalled()

    sessionDeferred.resolve(true)
    await sessionDeferred.promise
    await Promise.resolve()

    expect(mockApp.mount).toHaveBeenCalledTimes(1)
    expect(mockUserStore.loadSession.mock.invocationCallOrder[0])
      .toBeLessThan(mockApp.mount.mock.invocationCallOrder[0])
    expect(mockAppStore.startVersionWatcher).toHaveBeenCalledTimes(1)
  })

  it('renders a deterministic startup failure when session hydration rejects', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})

    await import('@/main')

    sessionDeferred.reject(new Error('session unavailable'))
    await sessionDeferred.promise.catch(() => {})
    await Promise.resolve()

    expect(mockApp.mount).not.toHaveBeenCalled()
    expect(document.querySelector('#app')?.textContent).toContain('Application failed to start')
    expect(consoleError).toHaveBeenCalledWith('[Startup] Application bootstrap failed:', expect.any(Error))
  })
})
