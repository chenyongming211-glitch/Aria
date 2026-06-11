// src/router/index.js
import { createRouter, createWebHashHistory } from 'vue-router'
import { getActivePinia } from 'pinia'
import useUserStore, { permissionsForRole, tokenRequiresPasswordChange } from '@/stores/user'
import { clearSession, isIdleSessionExpired } from '@/utils/session'
import Layout from '@/components/layout/Layout.vue'

const routes = [
  {
    path: '/',
    component: Layout,
    redirect: '/dashboard',
    children: [
      {
        path: 'dashboard',
        name: 'Dashboard',
        component: () => import('@/views/Dashboard.vue'),
        meta: { title: 'Dashboard', requiresAuth: true, permission: 'monitoring:read' }
      },
      {
        path: 'nodes',
        name: 'Nodes',
        component: () => import('@/views/Nodes.vue'),
        meta: { title: 'Nodes', requiresAuth: true, permission: 'nodes:read' }
      },
      {
        path: 'connectivity',
        redirect: '/connectivity/routing'
      },
      {
        path: 'connectivity/routing',
        name: 'Routing',
        component: () => import('@/views/Routing.vue'),
        meta: { title: 'Routing Management', requiresAuth: true, section: 'connectivity', permission: 'routes:read' }
      },
      {
        path: 'connectivity/topology',
        name: 'VpnTopology',
        component: () => import('@/views/VpnTopology.vue'),
        meta: { title: 'VPN Topology', requiresAuth: true, section: 'connectivity', permission: 'monitoring:read' }
      },
      {
        path: 'policy-center',
        name: 'Policies',
        component: () => import('@/views/Policies.vue'),
        meta: { title: 'Policy Center', requiresAuth: true, section: 'policy-center', permission: 'policies:read' }
      },
      {
        path: 'policy-center/bandwidth-control',
        name: 'BandwidthControl',
        component: () => import('@/views/BandwidthControl.vue'),
        meta: { title: 'Bandwidth Control', requiresAuth: true, section: 'policy-center', permission: 'qos:read' }
      },
      {
        path: 'policy-center/acl-rules',
        name: 'ACLRules',
        component: () => import('@/views/ACLRules.vue'),
        meta: { title: 'ACL Rules Management', requiresAuth: true, section: 'policy-center', permission: 'acls:read' }
      },
      {
        path: 'platform',
        redirect: '/platform/tokens'
      },
      {
        path: 'platform/tokens',
        name: 'Tokens',
        component: () => import('@/views/Tokens.vue'),
        meta: { title: 'Tokens', requiresAuth: true, section: 'platform', permission: 'tokens:read' }
      },
      {
        path: 'platform/tenants',
        name: 'TenantManagement',
        component: () => import('@/views/TenantManagement.vue'),
        meta: { title: 'Tenant Management', requiresAuth: true, section: 'platform', permission: 'users:read' }
      },
      {
        path: 'platform/roles',
        name: 'Roles',
        component: () => import('@/views/Roles.vue'),
        meta: { title: 'Role Management', requiresAuth: true, section: 'platform', permission: 'roles:read' }
      },
      {
        path: 'monitoring',
        name: 'Monitoring',
        component: () => import('@/views/Monitoring.vue'),
        meta: { title: 'Monitoring Center', requiresAuth: true, permission: 'monitoring:read' }
      },
      {
        path: 'monitoring/nodes/:nodeId',
        name: 'NodeMonitorDetail',
        component: () => import('@/views/NodeMonitorDetail.vue'),
        meta: { title: 'Node Monitor Detail', requiresAuth: true, permission: 'monitoring:read' }
      },
      {
        path: 'ai-copilot',
        name: 'AiAssistant',
        component: () => import('@/views/AIAssistant.vue'),
        meta: { title: 'AI Assistant', requiresAuth: true, permission: 'ai:use' }
      },
      {
        path: 'platform/settings',
        name: 'Settings',
        component: () => import('@/views/Settings.vue'),
        meta: { title: 'Settings', requiresAuth: true, section: 'platform', role: 'super_admin' }
      },
      {
        path: 'routing',
        redirect: '/connectivity/routing'
      },
      {
        path: 'policies',
        redirect: '/policy-center'
      },
      {
        path: 'bandwidth-control',
        redirect: '/policy-center/bandwidth-control'
      },
      {
        path: 'acl-rules',
        redirect: '/policy-center/acl-rules'
      },
      {
        path: 'tokens',
        redirect: '/platform/tokens'
      },
      {
        path: 'tenant-management',
        redirect: '/platform/tenants'
      },
      {
        path: 'settings',
        redirect: '/platform/settings'
      },
      {
        path: 'ai-assistant',
        redirect: '/ai-copilot'
      }
    ]
  },
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/Login.vue'),
    meta: { title: 'Login', requiresAuth: false }
  },
  {
    path: '/change-password',
    name: 'ChangePassword',
    component: () => import('@/views/ChangePassword.vue'),
    meta: { title: 'Change Password', requiresAuth: true, allowPasswordChange: true }
  }
]

const router = createRouter({
  history: createWebHashHistory(),
  routes
})

const readPermissions = () => {
  try {
    const raw = localStorage.getItem('aria_permissions')
    return raw ? JSON.parse(raw) : []
  } catch (error) {
    console.warn('Failed to parse cached permissions:', error)
    return []
  }
}

let refreshedPermissionKey = ''

const activeUserStore = () => {
  if (!getActivePinia()) return null
  try {
    return useUserStore()
  } catch (error) {
    console.warn('Failed to access user store for route permissions:', error)
    return null
  }
}

const clearCachedSession = () => {
  refreshedPermissionKey = ''
  clearSession()
}

const readUser = () => {
  try {
    const raw = localStorage.getItem('aria_user')
    return raw ? JSON.parse(raw) : null
  } catch (error) {
    console.warn('Failed to parse cached user:', error)
    return null
  }
}

const hasValidCachedUser = () => {
  const user = readUser()
  return Boolean(user?.role)
}

const loadRoutePermissions = async (user) => {
  if (user?.role === 'super_admin') return ['*']

  const store = activeUserStore()
  const fallbackPermissions = permissionsForRole(user?.role)
  if (!store || !user?.tenant_id) {
    const cachedPermissions = readPermissions()
    const permissions = cachedPermissions.length > 0 ? cachedPermissions : fallbackPermissions
    if (cachedPermissions.length === 0 && fallbackPermissions.length > 0) {
      localStorage.setItem('aria_permissions', JSON.stringify(fallbackPermissions))
    }
    return permissions
  }

  const key = `${user.tenant_id}:${user.role}`
  if (refreshedPermissionKey !== key) {
    try {
      await store.loadPermissions(user.tenant_id, user.role)
    } catch (error) {
      console.warn('Failed to refresh route permissions:', error)
    }
    refreshedPermissionKey = key
  }

  if (store.permissions?.length > 0) return store.permissions

  const cachedPermissions = readPermissions()
  const permissions = cachedPermissions.length > 0 ? cachedPermissions : fallbackPermissions
  if (cachedPermissions.length === 0 && fallbackPermissions.length > 0) {
    localStorage.setItem('aria_permissions', JSON.stringify(fallbackPermissions))
  }
  return permissions
}

const hasRoutePermission = async (to) => {
  const requiredRole = to.meta?.role
  if (requiredRole) {
    const user = readUser()
    if (user?.role !== requiredRole) return false
  }

  const required = to.meta?.permission
  if (!required) return true

  const user = readUser()
  if (user?.role === 'super_admin') return true

  const permissions = await loadRoutePermissions(user)
  if (permissions.includes('*')) return true

  if (Array.isArray(required)) {
    return required.some((p) => permissions.includes(p))
  }
  return permissions.includes(required)
}

const requiresPasswordChange = (token) => {
  return localStorage.getItem('aria_must_change_password') === 'true' || tokenRequiresPasswordChange(token)
}

const routePathForChild = (child) => {
  if (!child?.path || child.path.includes(':')) return null
  return child.path.startsWith('/') ? child.path : `/${child.path}`
}

const findAccessibleFallbackPath = async (blockedPath) => {
  const root = routes.find((route) => route.path === '/')
  for (const child of root?.children || []) {
    if (child.redirect || !child.meta?.requiresAuth) continue
    const path = routePathForChild(child)
    if (!path || path === blockedPath) continue
    if (await hasRoutePermission(child)) return path
  }
  return '/login'
}

router.beforeEach(async (to, from, next) => {
  const token = localStorage.getItem('aria_token')
  const expireTime = localStorage.getItem('aria_token_expire_time')
  
  // 检查是否过期
  let isExpired = false
  if (expireTime) {
    isExpired = Date.now() > parseInt(expireTime, 10)
  }

  const hasInvalidCachedSession = token && !isExpired && !hasValidCachedUser()
  const hasIdleExpiredSession = token && !isExpired && isIdleSessionExpired()
  const hasForcedPasswordChange = token && !isExpired && requiresPasswordChange(token)

  if (hasInvalidCachedSession) {
    console.warn('Invalid cached session, redirecting to login')
    clearCachedSession()
    if (to.path === '/login') {
      next()
    } else {
      next('/login')
    }
    return
  }

  if (hasIdleExpiredSession) {
    console.warn('Session idle timeout exceeded, redirecting to login')
    clearCachedSession()
    if (to.path === '/login') {
      next()
    } else {
      next('/login')
    }
    return
  }

  if (hasForcedPasswordChange && !to.meta?.allowPasswordChange) {
    console.warn('Password change required, redirecting to change-password')
    next('/change-password')
    return
  }
  
  if (to.meta.requiresAuth && (!token || isExpired)) {
    if (isExpired) {
      console.warn('Token expired, redirecting to login')
      clearCachedSession()
    }
    next('/login')
  } else if (to.path === '/login' && token && !isExpired && hasForcedPasswordChange) {
    next('/change-password')
  } else if (to.path === '/change-password' && token && !isExpired && !hasForcedPasswordChange) {
    next('/dashboard')
  } else if (to.path === '/login' && token && !isExpired && !hasForcedPasswordChange) {
    next('/dashboard')
  } else if (to.meta.requiresAuth && !(await hasRoutePermission(to))) {
    const fallbackPath = await findAccessibleFallbackPath(to.path)
    console.warn(`Permission denied for route ${to.path}, redirecting to ${fallbackPath}`)
    if (fallbackPath === '/login') {
      clearCachedSession()
    }
    next(fallbackPath)
  } else {
    next()
  }
})

export default router
