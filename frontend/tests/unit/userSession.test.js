import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

vi.mock('@/composables/useApi', () => ({
  default: {
    get: vi.fn(),
    post: vi.fn()
  }
}))

import api from '@/composables/useApi'
import useUserStore, { normalizeRoleName } from '@/stores/user'
import { clearSession } from '@/utils/session'

const makeJwt = (payload) => [
  Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url'),
  Buffer.from(JSON.stringify(payload)).toString('base64url'),
  'signature'
].join('.')

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

  it('normalizes legacy owner roles to admin', () => {
    expect(normalizeRoleName('OWNER')).toBe('admin')
    expect(normalizeRoleName('member')).toBe('operator')
  })

  it('clears token-only sessions instead of leaving a half-authenticated browser state', async () => {
    localStorage.setItem('aria_token', 'stale-token')
    localStorage.setItem('aria_token_expire_time', `${Date.now() + 60_000}`)
    localStorage.setItem('aria_permissions', JSON.stringify(['nodes:read']))

    const userStore = useUserStore()

    await expect(userStore.loadSession()).resolves.toBe(false)
    expect(userStore.isAuthenticated).toBe(false)
    expect(localStorage.getItem('aria_token')).toBeNull()
    expect(localStorage.getItem('aria_permissions')).toBeNull()
  })

  it('clears malformed cached users without throwing during startup', async () => {
    localStorage.setItem('aria_token', 'stale-token')
    localStorage.setItem('aria_token_expire_time', `${Date.now() + 60_000}`)
    localStorage.setItem('aria_user', '{not-json')

    const userStore = useUserStore()

    await expect(userStore.loadSession()).resolves.toBe(false)
    expect(userStore.isAuthenticated).toBe(false)
    expect(localStorage.getItem('aria_token')).toBeNull()
    expect(localStorage.getItem('aria_user')).toBeNull()
  })

  it('initializes last activity when restoring an existing session', async () => {
    localStorage.setItem('aria_token', 'valid-token')
    localStorage.setItem('aria_user', JSON.stringify({
      id: 'user-1',
      username: 'operator',
      role: 'operator',
      tenant_id: 'tenant-1'
    }))

    const userStore = useUserStore()

    await expect(userStore.loadSession()).resolves.toBe(true)
    expect(Number(localStorage.getItem('aria_last_activity'))).toBeGreaterThan(0)
  })

  it('drops malformed cached permissions while keeping a valid cached user', async () => {
    localStorage.setItem('aria_token', 'valid-token')
    localStorage.setItem('aria_user', JSON.stringify({
      id: 'user-1',
      username: 'operator',
      role: 'admin'
    }))
    localStorage.setItem('aria_permissions', '{not-json')

    const userStore = useUserStore()

    await expect(userStore.loadSession()).resolves.toBe(true)
    expect(userStore.isAuthenticated).toBe(true)
    expect(userStore.permissions).toEqual([])
    expect(localStorage.getItem('aria_permissions')).toBeNull()
  })

  it('normalizes cached backend users and restores super_admin wildcard permissions', async () => {
    localStorage.setItem('aria_token', 'valid-token')
    localStorage.setItem('aria_user', JSON.stringify({
      id: 'user-1',
      username: 'sysadmin',
      role: 'super_admin'
    }))
    localStorage.setItem('aria_permissions', JSON.stringify([]))

    const userStore = useUserStore()

    await expect(userStore.loadSession()).resolves.toBe(true)
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

  it('clears the in-memory user store when session storage is cleared outside logout', async () => {
    localStorage.setItem('aria_token', 'valid-token')
    localStorage.setItem('aria_user', JSON.stringify({
      id: 'user-1',
      username: 'sysadmin',
      role: 'super_admin'
    }))

    const userStore = useUserStore()

    await expect(userStore.loadSession()).resolves.toBe(true)
    expect(userStore.isAuthenticated).toBe(true)

    clearSession()

    expect(userStore.isAuthenticated).toBe(false)
    expect(userStore.user).toBeNull()
    expect(userStore.permissions).toEqual([])
  })

  it('repairs stale cached user roles from JWT claims during startup', async () => {
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

    await expect(userStore.loadSession()).resolves.toBe(true)
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

  it('does not log login responses containing bearer tokens', async () => {
    const logSpy = vi.spyOn(console, 'log').mockImplementation(() => {})
    api.post.mockResolvedValue({
      data: {
        success: true,
        data: {
          token: 'jwt.secret.value',
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
    expect(logSpy).not.toHaveBeenCalled()
    logSpy.mockRestore()
  })

  it('derives missing login user data from JWT claims instead of defaulting to admin', async () => {
    api.post.mockResolvedValue({
      data: {
        success: true,
        data: {
          token: makeJwt({
            uid: 'user-2',
            unm: 'viewer',
            rol: 'viewer',
            tid: 'tenant-1',
            exp: Math.floor(Date.now() / 1000) + 3600
          }),
          expires_in: 7200,
          require_password_change: false
        }
      }
    })
    api.get.mockResolvedValue({
      data: {
        data: {
          role: 'viewer',
          tenant_id: 'tenant-1',
          permissions: ['nodes:read']
        }
      }
    })

    const userStore = useUserStore()
    const result = await userStore.login({ username: 'viewer', password: 'secret' })

    expect(result.success).toBe(true)
    expect(userStore.user).toMatchObject({
      id: 'user-2',
      username: 'viewer',
      role: 'viewer',
      tenant_id: 'tenant-1'
    })
    expect(userStore.user.role).not.toBe('admin')
  })

  it('persists the must-change-password flag during forced-password login', async () => {
    api.post.mockResolvedValue({
      data: {
        success: true,
        data: {
          token: 'new-token',
          expires_in: 7200,
          require_password_change: true,
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
    expect(result.requirePasswordChange).toBe(true)
    expect(userStore.mustChangePassword).toBe(true)
    expect(localStorage.getItem('aria_must_change_password')).toBe('true')
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

  it('clears permissions instead of falling back to built-in role permissions when auth lookup fails', async () => {
    localStorage.setItem('aria_permissions', JSON.stringify(['nodes:write', 'routes:write']))
    api.get.mockImplementation((url) => {
      if (url === '/v2/auth/permissions') {
        return Promise.reject(new Error('permission lookup unavailable'))
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`))
    })

    const userStore = useUserStore()

    const loaded = await userStore.loadPermissions('tenant-1', 'admin')

    expect(api.get).toHaveBeenCalledWith('/v2/auth/permissions')
    expect(api.get).not.toHaveBeenCalledWith('/v2/tenants/tenant-1/roles')
    expect(loaded).toEqual([])
    expect(userStore.permissions).toEqual([])
    expect(JSON.parse(localStorage.getItem('aria_permissions'))).toEqual([])
  })

  it('waits for backend permissions during session restore instead of writing role defaults', async () => {
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

    await expect(userStore.loadSession()).resolves.toBe(true)

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

  it('updates expiry and cached user data from refresh responses', async () => {
    api.post.mockResolvedValue({
      data: {
        success: true,
        data: {
          token: makeJwt({
            uid: 'user-1',
            unm: 'alice',
            rol: 'viewer',
            tid: 'tenant-2',
            exp: Math.floor(Date.now() / 1000) + 3600
          }),
          expires_in: 1800,
          require_password_change: false,
          user: {
            id: 'user-1',
            username: 'alice',
            role: 'viewer',
            tenant_id: 'tenant-2'
          }
        }
      }
    })

    const userStore = useUserStore()
    userStore.user = {
      id: 'user-1',
      username: 'alice',
      role: 'admin',
      tenant_id: 'tenant-1'
    }
    localStorage.setItem('aria_token_expire_time', `${Date.now() + 60_000}`)
    localStorage.setItem('aria_must_change_password', 'true')

    const beforeRefresh = Date.now()
    const result = await userStore.refreshToken()

    expect(result.success).toBe(true)
    expect(userStore.user).toMatchObject({
      role: 'viewer',
      tenant_id: 'tenant-2'
    })
    expect(JSON.parse(localStorage.getItem('aria_user'))).toMatchObject({
      role: 'viewer',
      tenant_id: 'tenant-2'
    })
    expect(Number(localStorage.getItem('aria_token_expire_time'))).toBeGreaterThanOrEqual(beforeRefresh + 1799 * 1000)
    expect(localStorage.getItem('aria_must_change_password')).toBeNull()
  })
})
