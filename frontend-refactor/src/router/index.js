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
        meta: { title: 'Dashboard' }
      },
      {
        path: 'nodes',
        name: 'Nodes',
        component: () => import('@/views/Nodes.vue'),
        meta: { title: 'Nodes' }
      },
      {
        path: 'routing',
        name: 'Routing',
        component: () => import('@/views/Routing.vue'),
        meta: { title: 'Routing Management' }
      },
      {
        path: 'policies',
        name: 'Policies',
        component: () => import('@/views/Policies.vue'),
        meta: { title: 'Policies' }
      },
      {
        path: 'bandwidth-control',
        name: 'BandwidthControl',
        component: () => import('@/views/BandwidthControl.vue'),
        meta: { title: 'Bandwidth Control' }
      },
      {
        path: 'tokens',
        name: 'Tokens',
        component: () => import('@/views/Tokens.vue'),
        meta: { title: 'Tokens' }
      },
      {
        path: 'acl-rules',
        name: 'ACLRules',
        component: () => import('@/views/ACLRules.vue'),
        meta: { title: 'ACL Rules Management' }
      },
      {
        path: 'tenant-management',
        name: 'TenantManagement',
        component: () => import('@/views/TenantManagement.vue'),
        meta: { title: 'Tenant Management' }
      },
      {
        path: 'monitoring',
        name: 'Monitoring',
        component: () => import('@/views/Monitoring.vue'),
        meta: { title: 'Monitoring Center' }
      },
      {
        path: 'ai-assistant',
        name: 'AiAssistant',
        component: () => import('@/views/AIAssistant.vue'),
        meta: { title: 'AI Assistant' }
      },
      {
        path: 'settings',
        name: 'Settings',
        component: () => import('@/views/Settings.vue'),
        meta: { title: 'Settings' }
      }
    ]
  },
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/Login.vue'),
    meta: { title: 'Login' }
  }
]

const router = createRouter({
  history: createWebHashHistory(),
  routes
})

export default router