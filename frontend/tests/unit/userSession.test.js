import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

vi.mock('@/composables/useApi', () => ({
  default: {
    get: vi.fn(),
    post: vi.fn()
  }
}))

import api from '@/composables/useApi'
import useUserStore from '@/stores/user'

const makeJwt = (payload) => [
  Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url'),
  Buffer.from(JSON.stringify(payload)).toString('base64url'),
  'signature'
].join('.')

const flushPromises = () => new Promise(resolve => setTimeout(resolve, 0))

describe('user session persistence', () => {
  let storage

  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    storage = new Map()
    globalThis.localStorage = {
      getItem: (key) => (storage.has(key) ? storage.get(key) : null),
      setItem: (key, value) => storage.set(key, String(value)),
      removeItem: (key) => storage.delete(key),
      clear: () => storage.clear()
    }
  })

  it('clears token-only sessions instead of leaving a half-authenticated browser state', () => {
    localStorage.setItem('aria_token', 'stale-token')
    localStorage.setItem('aria_token_expire_time', `${Date.now() + 60_000}`)
    localStorage.setItem('aria_permissions', JSON.stringify(['nodes:read']))

    const userStore = useUserStore()

    expect(userStore.loadSession()).toBe(false)
    expect(userStore.isAuthenticated).toBe(false)
    expect(localStorage.getItem('aria_token')).toBeNull()
    expect(localStorage.getItem('aria_permissions')).toBeNull()
  })

  it('clears malformed cached users without throwing during startup', () => {
    localStorage.setItem('aria_token', 'stale-token')
    localStorage.setItem('aria_token_expire_time', `${Date.now() + 60_000}`)
    localStorage.setItem('aria_user', '{not-json')

    const userStore = useUserStore()

    expect(() => userStore.loadSession()).not.toThrow()
    expect(userStore.isAuthenticated).toBe(false)
    expect(localStorage.getItem('aria_token')).toBeNull()
    expect(localStorage.getItem('aria_user')).toBeNull()
  })

  it('drops malformed cached permissions while keeping a valid cached user', () => {
    localStorage.setItem('aria_token', 'valid-token')
    localStorage.setItem('aria_user', JSON.stringify({
      id: 'user-1',
      username: 'operator',
      role: 'admin'
    }))
    localStorage.setItem('aria_permissions', '{not-json')

    const userStore = useUserStore()

    expect(userStore.loadSession()).toBe(true)
    expect(userStore.isAuthenticated).toBe(true)
    expect(userStore.permissions).toEqual([])
    expect(localStorage.getItem('aria_permissions')).toBeNull()
  })

  it('normalizes cached backend users and restores super_admin wildcard permissions', () => {
    localStorage.setItem('aria_token', 'valid-token')
    localStorage.setItem('aria_user', JSON.stringify({
      id: 'user-1',
      username: 'sysadmin',
      role: 'super_admin'
    }))
    localStorage.setItem('aria_permissions', JSON.stringify([]))

    const userStore = useUserStore()

    expect(userStore.loadSession()).toBe(true)
    expect(userStore.user).toMatchObject({
      username: 'sysadmin',
      name: 'sysadmin',
      initials: 'SY',
      role: 'super_admin'
    })
    expect(userStore.permissions).toEqual(['*'])
    expect(JSON.parse(localStorage.getItem('aria_user'))).toMatchObject({
      name: 'sysadmin',
      initials: 'SY'
    })
  })

  it('repairs stale cached user roles from JWT claims during startup', () => {
    localStorage.setItem('aria_token', makeJwt({
      uid: 'user-1',
      unm: 'sysadmin',
      rol: 'super_admin',
      tid: '',
      exp: Math.floor(Date.now() / 1000) + 3600
    }))
    localStorage.setItem('aria_user', JSON.stringify({
      id: 'user-1',
      username: 'sysadmin',
      role: 'admin'
    }))
    localStorage.setItem('aria_permissions', JSON.stringify([]))

    const userStore = useUserStore()

    expect(userStore.loadSession()).toBe(true)
    expect(userStore.user).toMatchObject({
      id: 'user-1',
      username: 'sysadmin',
      role: 'super_admin'
    })
    expect(userStore.permissions).toEqual(['*'])
    expect(JSON.parse(localStorage.getItem('aria_user'))).toMatchObject({
      role: 'super_admin'
    })
    expect(JSON.parse(localStorage.getItem('aria_permissions'))).toEqual(['*'])
  })

  it('normalizes logged-in backend users for header display', async () => {
    api.post.mockResolvedValue({
      data: {
        success: true,
        data: {
          token: 'new-token',
          expires_in: 7200,
          require_password_change: false,
          user: {
            id: 'user-1',
            username: 'sysadmin',
            role: 'super_admin',
            tenant_id: ''
          }
        }
      }
    })

    const userStore = useUserStore()
    const result = await userStore.login({ username: 'sysadmin', password: 'secret' })

    expect(result.success).toBe(true)
    expect(userStore.user).toMatchObject({
      username: 'sysadmin',
      name: 'sysadmin',
      initials: 'SY',
      role: 'super_admin'
    })
    expect(JSON.parse(localStorage.getItem('aria_user'))).toMatchObject({
      name: 'sysadmin',
      initials: 'SY'
    })
  })

  it('loads operator permissions from the current auth context without requiring roles:read', async () => {
    api.get.mockResolvedValue({
      data: {
        data: {
          role: 'operator',
          tenant_id: 'tenant-1',
          permissions: ['nodes:read', 'custom:use']
        }
      }
    })
    const userStore = useUserStore()

    await userStore.loadPermissions('tenant-1', 'operator')

    expect(api.get).toHaveBeenCalledWith('/v2/auth/permissions')
    expect(userStore.permissions).toEqual(['nodes:read', 'custom:use'])
    expect(JSON.parse(localStorage.getItem('aria_permissions'))).toEqual(userStore.permissions)
  })

  it('restores role permissions immediately and refreshes them from the server', async () => {
    api.get.mockResolvedValue({
      data: {
        data: {
          role: 'operator',
          tenant_id: 'tenant-1',
          permissions: ['nodes:read', 'custom:use']
        }
      }
    })
    localStorage.setItem('aria_token', 'valid-token')
    localStorage.setItem('aria_user', JSON.stringify({
      id: 'user-1',
      username: 'operator',
      role: 'operator',
      tenant_id: 'tenant-1'
    }))

    const userStore = useUserStore()

    expect(userStore.loadSession()).toBe(true)
    expect(userStore.permissions).toEqual(expect.arrayContaining([
      'nodes:read',
      'routes:read',
      'acls:read',
      'monitoring:read'
    ]))
    expect(JSON.parse(localStorage.getItem('aria_permissions'))).toEqual(userStore.permissions)

    await flushPromises()

    expect(api.get).toHaveBeenCalledWith('/v2/auth/permissions')
    expect(userStore.permissions).toEqual(['nodes:read', 'custom:use'])
    expect(JSON.parse(localStorage.getItem('aria_permissions'))).toEqual(['nodes:read', 'custom:use'])
  })

  it('waits for permission loading before resolving login', async () => {
    let resolveRoles
    api.post.mockResolvedValue({
      data: {
        success: true,
        data: {
          token: 'new-token',
          expires_in: 7200,
          require_password_change: false,
          user: {
            id: 'user-1',
            username: 'admin',
            role: 'admin',
            tenant_id: 'tenant-1'
          }
        }
      }
    })
    api.get.mockReturnValue(new Promise((resolve) => {
      resolveRoles = resolve
    }))

    const userStore = useUserStore()
    let settled = false
    const loginPromise = userStore.login({ username: 'admin', password: 'secret' })
      .then((result) => {
        settled = true
        return result
      })

    await Promise.resolve()
    await Promise.resolve()
    expect(settled).toBe(false)

    expect(api.get).toHaveBeenCalledWith('/v2/auth/permissions')

    resolveRoles({
      data: {
        data: {
          role: 'admin',
          tenant_id: 'tenant-1',
          permissions: ['nodes:read', 'roles:read']
        }
      }
    })

    const result = await loginPromise

    expect(result.success).toBe(true)
    expect(userStore.permissions).toEqual(['nodes:read', 'roles:read'])
  })
})