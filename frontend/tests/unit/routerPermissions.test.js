import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia } from 'pinia'
import router from '@/router'

const routePermissionCases = [
  ['Dashboard', 'monitoring:read'],
  ['Nodes', 'nodes:read'],
  ['Routing', 'routes:read'],
  ['Policies', 'policies:read'],
  ['ACLRules', 'acls:read'],
  ['IPGroups', 'ip-groups:read'],
  ['BandwidthControl', 'qos:read'],
  ['Tokens', 'tokens:read'],
  ['Roles', 'roles:read'],
  ['Monitoring', 'monitoring:read']
]

describe('router RBAC metadata', () => {
  let storage

  beforeEach(() => {
    setActivePinia(undefined)
    storage = new Map()
    globalThis.localStorage = {
      getItem: (key) => (storage.has(key) ? storage.get(key) : null),
      setItem: (key, value) => storage.set(key, String(value)),
      removeItem: (key) => storage.delete(key),
      clear: () => storage.clear()
    }
  })

  it('defines permission metadata for protected pages', () => {
    for (const [name, permission] of routePermissionCases) {
      const route = router.getRoutes().find((r) => r.name === name)
      expect(route, `missing route ${name}`).toBeTruthy()
      expect(route.meta.permission).toBe(permission)
    }
  })

  it('uses translated title keys instead of fixed route titles', () => {
    const namedRoutes = router.getRoutes().filter((route) => route.name)

    for (const route of namedRoutes) {
      expect(route.meta.title, `${String(route.name)} should not use fixed title`).toBeUndefined()
      expect(route.meta.titleKey, `${String(route.name)} missing titleKey`).toMatch(/^[a-z]+[a-zA-Z0-9]*(\.[a-z]+[a-zA-Z0-9]*)+$/)
    }
  })

  it('restricts Settings to super_admin role metadata', () => {
    const route = router.getRoutes().find((r) => r.name === 'Settings')
    expect(route, 'missing route Settings').toBeTruthy()
    expect(route.meta.role).toBe('super_admin')
    expect(route.meta.permission).toBeUndefined()
  })

  it('restricts TenantManagement to super_admin role metadata', () => {
    const route = router.getRoutes().find((r) => r.name === 'TenantManagement')
    expect(route, 'missing route TenantManagement').toBeTruthy()
    expect(route.meta.role).toBe('super_admin')
    expect(route.meta.permission).toBeUndefined()
  })

  it('blocks navigation without required permission', async () => {
    localStorage.setItem('aria_token', 'dummy-token')
    localStorage.setItem('aria_token_expire_time', `${Date.now() + 60_000}`)
    localStorage.setItem('aria_last_activity', `${Date.now()}`)
    localStorage.setItem('aria_user', JSON.stringify({ role: 'admin' }))
    localStorage.setItem('aria_permissions', JSON.stringify(['monitoring:read', 'nodes:read']))

    await router.push('/dashboard')
    await router.isReady()
    await router.push('/platform/tokens')

    expect(router.currentRoute.value.path).toBe('/dashboard')
  })

  it('redirects Dashboard access without monitoring permission to the first allowed route', async () => {
    await router.replace('/login')
    await router.isReady()

    localStorage.setItem('aria_token', 'dummy-token')
    localStorage.setItem('aria_token_expire_time', `${Date.now() + 60_000}`)
    localStorage.setItem('aria_last_activity', `${Date.now()}`)
    localStorage.setItem('aria_user', JSON.stringify({ role: 'operator' }))
    localStorage.setItem('aria_permissions', JSON.stringify(['nodes:read']))

    await router.push('/dashboard')

    expect(router.currentRoute.value.path).toBe('/nodes')
  })

  it('redirects must-change-password sessions to the dedicated change-password page before protected pages', async () => {
    await router.replace('/login')
    await router.isReady()

    localStorage.setItem('aria_token', 'dummy-token')
    localStorage.setItem('aria_token_expire_time', `${Date.now() + 60_000}`)
    localStorage.setItem('aria_last_activity', `${Date.now()}`)
    localStorage.setItem('aria_user', JSON.stringify({ role: 'admin' }))
    localStorage.setItem('aria_permissions', JSON.stringify(['monitoring:read']))
    localStorage.setItem('aria_must_change_password', 'true')

    await router.push('/dashboard')

    expect(router.currentRoute.value.path).toBe('/change-password')
  })

  it('allows must-change-password sessions to stay on the dedicated change-password page', async () => {
    await router.replace('/login')
    await router.isReady()

    localStorage.setItem('aria_token', 'dummy-token')
    localStorage.setItem('aria_token_expire_time', `${Date.now() + 60_000}`)
    localStorage.setItem('aria_last_activity', `${Date.now()}`)
    localStorage.setItem('aria_user', JSON.stringify({ role: 'admin' }))
    localStorage.setItem('aria_permissions', JSON.stringify(['monitoring:read']))
    localStorage.setItem('aria_must_change_password', 'true')

    await router.push('/change-password')

    expect(router.currentRoute.value.path).toBe('/change-password')
  })

  it('redirects logged-in must-change-password sessions from login to change-password', async () => {
    localStorage.setItem('aria_token', 'dummy-token')
    localStorage.setItem('aria_token_expire_time', `${Date.now() + 60_000}`)
    localStorage.setItem('aria_last_activity', `${Date.now()}`)
    localStorage.setItem('aria_user', JSON.stringify({ role: 'admin' }))
    localStorage.setItem('aria_permissions', JSON.stringify(['monitoring:read']))

    await router.replace('/dashboard')
    await router.isReady()
    expect(router.currentRoute.value.path).toBe('/dashboard')

    localStorage.setItem('aria_must_change_password', 'true')

    await router.push('/login')

    expect(router.currentRoute.value.path).toBe('/change-password')
  })

  it('redirects token-only sessions to login before protected pages', async () => {
    await router.replace('/login')
    await router.isReady()

    localStorage.setItem('aria_token', 'dummy-token')
    localStorage.setItem('aria_token_expire_time', `${Date.now() + 60_000}`)
    localStorage.setItem('aria_permissions', JSON.stringify(['nodes:read']))

    await router.push('/dashboard')

    expect(router.currentRoute.value.path).toBe('/login')
    expect(localStorage.getItem('aria_token')).toBeNull()
    expect(localStorage.getItem('aria_permissions')).toBeNull()
  })

  it('redirects malformed cached users to login before protected pages', async () => {
    await router.replace('/login')
    await router.isReady()

    localStorage.setItem('aria_token', 'dummy-token')
    localStorage.setItem('aria_token_expire_time', `${Date.now() + 60_000}`)
    localStorage.setItem('aria_user', '{not-json')

    await router.push('/dashboard')

    expect(router.currentRoute.value.path).toBe('/login')
    expect(localStorage.getItem('aria_token')).toBeNull()
    expect(localStorage.getItem('aria_user')).toBeNull()
  })

  it('redirects idle-expired sessions to login before protected pages', async () => {
    await router.replace('/login')
    await router.isReady()

    localStorage.setItem('aria_token', 'dummy-token')
    localStorage.setItem('aria_token_expire_time', `${Date.now() + 60_000}`)
    localStorage.setItem('aria_last_activity', `${Date.now() - 2 * 60 * 60 * 1000}`)
    localStorage.setItem('aria_user', JSON.stringify({ role: 'operator' }))

    await router.push('/nodes')

    expect(router.currentRoute.value.path).toBe('/login')
    expect(localStorage.getItem('aria_token')).toBeNull()
    expect(localStorage.getItem('aria_last_activity')).toBeNull()
  })

  it('allows super_admin navigation without a cached permissions list', async () => {
    await router.replace('/login')
    await router.isReady()

    localStorage.setItem('aria_token', 'dummy-token')
    localStorage.setItem('aria_token_expire_time', `${Date.now() + 60_000}`)
    localStorage.setItem('aria_last_activity', `${Date.now()}`)
    localStorage.setItem('aria_user', JSON.stringify({ role: 'super_admin' }))

    await router.push('/nodes')

    expect(router.currentRoute.value.path).toBe('/nodes')
  })

  it('does not fall back to built-in operator permissions when cached permissions are missing', async () => {
    await router.replace('/login')
    await router.isReady()

    localStorage.setItem('aria_token', 'dummy-token')
    localStorage.setItem('aria_token_expire_time', `${Date.now() + 60_000}`)
    localStorage.setItem('aria_last_activity', `${Date.now()}`)
    localStorage.setItem('aria_user', JSON.stringify({ role: 'operator' }))

    await router.push('/nodes')

    expect(router.currentRoute.value.path).toBe('/login')
    expect(localStorage.getItem('aria_permissions')).toBeNull()
  })

  it('blocks permission routes for non-super_admin users with stale wildcard permissions', async () => {
    await router.replace('/login')
    await router.isReady()

    localStorage.setItem('aria_token', 'dummy-token')
    localStorage.setItem('aria_token_expire_time', `${Date.now() + 60_000}`)
    localStorage.setItem('aria_last_activity', `${Date.now()}`)
    localStorage.setItem('aria_user', JSON.stringify({ role: 'custom-role', tenant_id: 'tenant-1' }))
    localStorage.setItem('aria_permissions', JSON.stringify(['*']))

    await router.push('/platform/tokens')

    expect(router.currentRoute.value.path).not.toBe('/platform/tokens')
  })

  it('blocks Settings navigation for non-super_admin users', async () => {
    localStorage.setItem('aria_token', 'dummy-token')
    localStorage.setItem('aria_token_expire_time', `${Date.now() + 60_000}`)
    localStorage.setItem('aria_last_activity', `${Date.now()}`)
    localStorage.setItem('aria_permissions', JSON.stringify(['monitoring:read']))
    localStorage.setItem('aria_user', JSON.stringify({ role: 'admin' }))

    await router.push('/dashboard')
    await router.isReady()
    await router.push('/platform/settings')

    expect(router.currentRoute.value.path).toBe('/dashboard')
  })

  it('blocks TenantManagement navigation for non-super_admin users', async () => {
    localStorage.setItem('aria_token', 'dummy-token')
    localStorage.setItem('aria_token_expire_time', `${Date.now() + 60_000}`)
    localStorage.setItem('aria_last_activity', `${Date.now()}`)
    localStorage.setItem('aria_permissions', JSON.stringify(['monitoring:read']))
    localStorage.setItem('aria_user', JSON.stringify({ role: 'admin' }))

    await router.push('/dashboard')
    await router.isReady()
    await router.push('/platform/tenants')

    expect(router.currentRoute.value.path).toBe('/dashboard')
  })
})
