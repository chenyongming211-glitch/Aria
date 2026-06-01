import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

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
  let activityHandlers
  let addEventListenerSpy

  beforeEach(() => {
    vi.resetModules()
    vi.clearAllMocks()
    vi.useRealTimers()
    axiosState.requestHandler = null
    axiosState.responseHandler = null
    axiosState.responseErrorHandler = null
    axiosState.instance = null
    axiosState.post.mockReset()
    activityHandlers = new Map()
    addEventListenerSpy = vi.spyOn(document, 'addEventListener')
      .mockImplementation((event, handler) => {
        activityHandlers.set(event, handler)
      })
    storage = new Map()
    globalThis.localStorage = {
      getItem: (key) => (storage.has(key) ? storage.get(key) : null),
      setItem: (key, value) => storage.set(key, String(value)),
      removeItem: (key) => storage.delete(key),
      clear: () => storage.clear()
    }
    window.location.hash = '#/dashboard'
  })

  afterEach(() => {
    addEventListenerSpy.mockRestore()
    vi.useRealTimers()
  })

  async function loadApiSession() {
    await import('@/composables/useApi')
  }

  function seedActiveSession(lastActivity = Date.now()) {
    localStorage.setItem('aria_token', 'token-1')
    localStorage.setItem('aria_user', JSON.stringify({ role: 'operator' }))
    localStorage.setItem('aria_last_activity', `${lastActivity}`)
    localStorage.setItem('aria_token_expire_time', `${Date.now() + 2 * 60 * 60 * 1000}`)
  }

  function runActivity(event = 'mousedown') {
    const handler = activityHandlers.get(event)
    expect(handler, `missing activity handler for ${event}`).toBeTypeOf('function')
    handler(new Event(event))
  }

  it('does not count background API requests as user activity', async () => {
    const lastActivity = Date.now() - 30 * 60 * 1000
    seedActiveSession(lastActivity)
    await loadApiSession()

    const config = await axiosState.requestHandler({ headers: {} })

    expect(config.headers.Authorization).toBe('Bearer token-1')
    expect(localStorage.getItem('aria_last_activity')).toBe(`${lastActivity}`)
  })

  it('rejects requests after idle logout instead of sending them', async () => {
    seedActiveSession(Date.now() - 2 * 60 * 60 * 1000)
    await loadApiSession()

    await expect(axiosState.requestHandler({ headers: {} }))
      .rejects.toThrow('Session expired')
    expect(localStorage.getItem('aria_token')).toBeNull()
  })

  it('does not refresh on every request when token expiry metadata is missing', async () => {
    localStorage.setItem('aria_token', 'token-1')
    localStorage.setItem('aria_last_activity', `${Date.now()}`)
    await loadApiSession()

    const config = await axiosState.requestHandler({ headers: {} })

    expect(axiosState.post).not.toHaveBeenCalled()
    expect(config.headers.Authorization).toBe('Bearer token-1')
  })

  it('expires an idle session before recording a new user activity', async () => {
    seedActiveSession(Date.now() - 2 * 60 * 60 * 1000)
    await loadApiSession()

    runActivity('mousedown')

    expect(localStorage.getItem('aria_token')).toBeNull()
    expect(localStorage.getItem('aria_last_activity')).toBeNull()
  })

  it('expires idle sessions on a timer without waiting for API or user activity', async () => {
    vi.useFakeTimers()
    const now = new Date('2026-06-01T08:00:00Z')
    vi.setSystemTime(now)
    seedActiveSession(now.getTime())
    await loadApiSession()

    vi.advanceTimersByTime(60 * 60 * 1000 + 1000)

    expect(localStorage.getItem('aria_token')).toBeNull()
    expect(localStorage.getItem('aria_last_activity')).toBeNull()
  })
})