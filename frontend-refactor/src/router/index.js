// src/router/index.js
import { createRouter, createWebHashHistory } from 'vue-router'
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
        meta: { title: 'Nodes', requiresAuth: true }
      },
      {
        path: 'connectivity',
        redirect: '/connectivity/routing'
      },
      {
        path: 'connectivity/routing',
        name: 'Routing',
        component: () => import('@/views/Routing.vue'),
        meta: { title: 'Routing Management', requiresAuth: true, section: 'connectivity' }
      },
      {
        path: 'connectivity/topology',
        name: 'VpnTopology',
        component: () => import('@/views/VpnTopology.vue'),
        meta: { title: 'VPN Topology', requiresAuth: true, section: 'connectivity' }
      },
      {
        path: 'policy-center',
        name: 'Policies',
        component: () => import('@/views/Policies.vue'),
        meta: { title: 'Policy Center', requiresAuth: true, section: 'policy-center' }
      },
      {
        path: 'policy-center/bandwidth-control',
        name: 'BandwidthControl',
        component: () => import('@/views/BandwidthControl.vue'),
        meta: { title: 'Bandwidth Control', requiresAuth: true, section: 'policy-center' }
      },
      {
        path: 'policy-center/acl-rules',
        name: 'ACLRules',
        component: () => import('@/views/ACLRules.vue'),
        meta: { title: 'ACL Rules Management', requiresAuth: true, section: 'policy-center' }
      },
      {
        path: 'platform',
        redirect: '/platform/tokens'
      },
      {
        path: 'platform/tokens',
        name: 'Tokens',
        component: () => import('@/views/Tokens.vue'),
        meta: { title: 'Tokens', requiresAuth: true, section: 'platform' }
      },
      {
        path: 'platform/tenants',
        name: 'TenantManagement',
        component: () => import('@/views/TenantManagement.vue'),
        meta: { title: 'Tenant Management', requiresAuth: true, section: 'platform' }
      },
      {
        path: 'monitoring',
        name: 'Monitoring',
        component: () => import('@/views/Monitoring.vue'),
        meta: { title: 'Monitoring Center', requiresAuth: true }
      },
      {
        path: 'ai-copilot',
        name: 'AiAssistant',
        component: () => import('@/views/AIAssistant.vue'),
        meta: { title: 'AI Assistant', requiresAuth: true }
      },
      {
        path: 'platform/settings',
        name: 'Settings',
        component: () => import('@/views/Settings.vue'),
        meta: { title: 'Settings', requiresAuth: true, section: 'platform' }
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

router.beforeEach((to, from, next) => {
  const token = sessionStorage.getItem('aria_token')
  
  if (to.meta.requiresAuth && !token) {
    next('/login')
  } else if (to.path === '/login' && token) {
    next('/dashboard')
  } else {
    next()
  }
})

export default router
