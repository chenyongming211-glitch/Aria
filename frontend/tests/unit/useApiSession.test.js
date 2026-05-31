import { describe, it, expect, vi, beforeEach } from 'vitest'

const axiosState = vi.hoisted(() => ({
  requestHandler: null,
  responseHandler: null,
  responseErrorHandler: null,
  instance: null,
  post: vi.fn()
}))

vi.mock('axios', () => ({
  default: {
    create: vi.fn(() => {
      axiosState.instance = {
        interceptors: {
          request: {
            use: vi.fn((handler) => {
              axiosState.requestHandler = handler
            })
          },
          response: {
            use: vi.fn((handler, errorHandler) => {
              axiosState.responseHandler = handler
              axiosState.responseErrorHandler = errorHandler
            })
          }
        }
      }
      return axiosState.instance
    }),
    post: axiosState.post
  }
}))

describe('useApi session request interceptor', () => {
  let storage

  beforeEach(async () => {
    vi.resetModules()
    vi.clearAllMocks()
    axiosState.requestHandler = null
    axiosState.responseHandler = null
    axiosState.responseErrorHandler = null
    axiosState.instance = null
    axiosState.post.mockReset()
    storage = new Map()
    globalThis.localStorage = {
      getItem: (key) => (storage.has(key) ? storage.get(key) : null),
      setItem: (key, value) => storage.set(key, String(value)),
      removeItem: (key) => storage.delete(key),
      clear: () => storage.clear()
    }
    window.location.hash = '#/dashboard'
    await import('@/composables/useApi')
  })

  it('rejects requests after idle logout instead of sending them', async () => {
    localStorage.setItem('aria_token', 'token-1')
    localStorage.setItem('aria_last_activity', `${Date.now() - 2 * 60 * 60 * 1000}`)

    await expect(axiosState.requestHandler({ headers: {} }))
      .rejects.toThrow('Session expired')
    expect(localStorage.getItem('aria_token')).toBeNull()
  })

  it('does not refresh on every request when token expiry metadata is missing', async () => {
    localStorage.setItem('aria_token', 'token-1')
    localStorage.setItem('aria_last_activity', `${Date.now()}`)

    const config = await axiosState.requestHandler({ headers: {} })

    expect(axiosState.post).not.toHaveBeenCalled()
    expect(config.headers.Authorization).toBe('Bearer token-1')
  })
})
