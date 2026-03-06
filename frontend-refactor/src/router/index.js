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
        path: 'routing',
        name: 'Routing',
        component: () => import('@/views/Routing.vue'),
        meta: { title: 'Routing Management', requiresAuth: true }
      },
      {
        path: 'policies',
        name: 'Policies',
        component: () => import('@/views/Policies.vue'),
        meta: { title: 'Policies', requiresAuth: true }
      },
      {
        path: 'bandwidth-control',
        name: 'BandwidthControl',
        component: () => import('@/views/BandwidthControl.vue'),
        meta: { title: 'Bandwidth Control', requiresAuth: true }
      },
      {
        path: 'tokens',
        name: 'Tokens',
        component: () => import('@/views/Tokens.vue'),
        meta: { title: 'Tokens', requiresAuth: true }
      },
      {
        path: 'acl-rules',
        name: 'ACLRules',
        component: () => import('@/views/ACLRules.vue'),
        meta: { title: 'ACL Rules Management', requiresAuth: true }
      },
      {
        path: 'tenant-management',
        name: 'TenantManagement',
        component: () => import('@/views/TenantManagement.vue'),
        meta: { title: 'Tenant Management', requiresAuth: true }
      },
      {
        path: 'monitoring',
        name: 'Monitoring',
        component: () => import('@/views/Monitoring.vue'),
        meta: { title: 'Monitoring Center', requiresAuth: true }
      },
      {
        path: 'ai-assistant',
        name: 'AiAssistant',
        component: () => import('@/views/AIAssistant.vue'),
        meta: { title: 'AI Assistant', requiresAuth: true }
      },
      {
        path: 'settings',
        name: 'Settings',
        component: () => import('@/views/Settings.vue'),
        meta: { title: 'Settings', requiresAuth: true }
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