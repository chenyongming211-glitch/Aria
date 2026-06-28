import { describe, it, expect, vi, beforeEach } from 'vitest'

const {
  mockApp,
  mockAppStore,
  mockUserStore
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
    mockUserStore.loadSession.mockClear()
    document.head.innerHTML = ''
    document.body.innerHTML = '<div id="app"></div>'
  })

  it('restores cached user session before mounting protected layouts', async () => {
    await import('@/main')

    expect(mockUserStore.loadSession).toHaveBeenCalledTimes(1)
    expect(mockApp.mount).toHaveBeenCalledTimes(1)
    expect(mockUserStore.loadSession.mock.invocationCallOrder[0])
      .toBeLessThan(mockApp.mount.mock.invocationCallOrder[0])
    expect(mockAppStore.startVersionWatcher).toHaveBeenCalledTimes(1)
  })
})
