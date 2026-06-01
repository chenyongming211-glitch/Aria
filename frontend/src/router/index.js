// src/router/index.js
import { createRouter, createWebHashHistory } from 'vue-router'
import { permissionsForRole } from '@/stores/user'
import { clearSession, isIdleSessionExpired } from '@/utils/session'
import Layout from '@/components/Layout/Layout.vue'

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
        meta: { title: 'Dashboard', requiresAuth: true }
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

const clearCachedSession = () => {
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

const hasRoutePermission = (to) => {
  const requiredRole = to.meta?.role
  if (requiredRole) {
    const user = readUser()
    if (user?.role !== requiredRole) return false
  }

  const required = to.meta?.permission
  if (!required) return true

  const user = readUser()
  if (user?.role === 'super_admin') return true

  const cachedPermissions = readPermissions()
  const fallbackPermissions = permissionsForRole(user?.role)
  const permissions = cachedPermissions.length > 0 ? cachedPermissions : fallbackPermissions
  if (cachedPermissions.length === 0 && fallbackPermissions.length > 0) {
    localStorage.setItem('aria_permissions', JSON.stringify(fallbackPermissions))
  }
  if (permissions.includes('*')) return true

  if (Array.isArray(required)) {
    return required.some((p) => permissions.includes(p))
  }
  return permissions.includes(required)
}

router.beforeEach((to, from, next) => {
  const token = localStorage.getItem('aria_token')
  const expireTime = localStorage.getItem('aria_token_expire_time')
  
  // 检查是否过期
  let isExpired = false
  if (expireTime) {
    isExpired = Date.now() > parseInt(expireTime, 10)
  }

  const hasInvalidCachedSession = token && !isExpired && !hasValidCachedUser()
  const hasIdleExpiredSession = token && !isExpired && isIdleSessionExpired()

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
  
  if (to.meta.requiresAuth && (!token || isExpired)) {
    if (isExpired) {
      console.warn('Token expired, redirecting to login')
      clearCachedSession()
    }
    next('/login')
  } else if (to.path === '/login' && token && !isExpired) {
    next('/dashboard')
  } else if (to.meta.requiresAuth && !hasRoutePermission(to)) {
    console.warn(`Permission denied for route ${to.path}, redirecting to dashboard`)
    next('/dashboard')
  } else {
    next()
  }
})

export default router
