import { describe, it, expect, beforeEach } from 'vitest'
import router from '@/router'

const routePermissionCases = [
  ['Nodes', 'nodes:read'],
  ['Routing', 'routes:read'],
  ['Policies', 'policies:read'],
  ['ACLRules', 'acls:read'],
  ['BandwidthControl', 'qos:read'],
  ['Tokens', 'tokens:read'],
  ['TenantManagement', 'users:read'],
  ['Roles', 'roles:read'],
  ['Monitoring', 'monitoring:read'],
  ['AiAssistant', 'ai:use']
]

describe('router RBAC metadata', () => {
  let storage

  beforeEach(() => {
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

  it('restricts Settings to super_admin role metadata', () => {
    const route = router.getRoutes().find((r) => r.name === 'Settings')
    expect(route, 'missing route Settings').toBeTruthy()
    expect(route.meta.role).toBe('super_admin')
    expect(route.meta.permission).toBeUndefined()
  })

  it('blocks navigation without required permission', async () => {
    localStorage.setItem('aria_token', 'dummy-token')
    localStorage.setItem('aria_token_expire_time', `${Date.now() + 60_000}`)
    localStorage.setItem('aria_permissions', JSON.stringify(['nodes:read']))

    await router.push('/dashboard')
    await router.isReady()
    await router.push('/platform/tokens')

    expect(router.currentRoute.value.path).toBe('/dashboard')
  })

  it('allows super_admin navigation when permission cache is empty', async () => {
    localStorage.setItem('aria_token', 'dummy-token')
    localStorage.setItem('aria_token_expire_time', `${Date.now() + 60_000}`)
    localStorage.setItem('aria_user', JSON.stringify({ role: 'super_admin' }))
    localStorage.setItem('aria_permissions', JSON.stringify([]))

    await router.push('/dashboard')
    await router.isReady()
    await router.push('/nodes')

    expect(router.currentRoute.value.path).toBe('/nodes')
  })

  it('blocks Settings navigation for non-super_admin users', async () => {
    localStorage.setItem('aria_token', 'dummy-token')
    localStorage.setItem('aria_token_expire_time', `${Date.now() + 60_000}`)
    localStorage.setItem('aria_permissions', JSON.stringify(['*']))
    localStorage.setItem('aria_user', JSON.stringify({ role: 'admin' }))

    await router.push('/dashboard')
    await router.isReady()
    await router.push('/platform/settings')

    expect(router.currentRoute.value.path).toBe('/dashboard')
  })
})
