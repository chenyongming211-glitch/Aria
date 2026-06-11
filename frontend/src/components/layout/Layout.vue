<!-- src/components/layout/Layout.vue - 深色侧边栏 + 浅色内容区 -->
<template>
  <el-container class="layout-container">
    <!-- 侧边栏（深色） -->
    <el-aside :width="sidebarWidth" class="sidebar">
      <!-- Logo 区域 -->
      <div class="sidebar-header">
        <div class="logo-wrapper">
          <div class="logo-glow"></div>
          <img src="/aria-logo.png" alt="Aria Logo" class="logo-image" />
        </div>
        <transition name="fade">
          <h1 v-if="!isCollapsed" class="logo-text">Aria <span class="pro-badge">PRO</span></h1>
        </transition>
      </div>

      <!-- 导航菜单 -->
      <el-menu
        :default-active="activeMenu"
        :collapse="isCollapsed"
        :default-openeds="defaultOpeneds"
        :unique-opened="true"
        class="sidebar-menu"
        router
      >
        <el-menu-item index="/dashboard">
          <el-icon><House /></el-icon>
          <template #title>{{ t('nav.dashboard') }}</template>
        </el-menu-item>
        <el-menu-item v-if="canAccess('nodes:read')" index="/nodes">
          <el-icon><Monitor /></el-icon>
          <template #title>{{ t('nav.nodes') }}</template>
        </el-menu-item>
        <el-sub-menu v-if="canAnyAccess(['routes:read', 'monitoring:read'])" index="connectivity">
          <template #title>
            <el-icon><Position /></el-icon>
            <span>{{ t('nav.connectivity') }}</span>
          </template>
          <el-menu-item v-if="canAccess('routes:read')" index="/connectivity/routing">
            <el-icon><Position /></el-icon>
            <template #title>{{ t('nav.routingManagement') }}</template>
          </el-menu-item>
          <el-menu-item v-if="canAccess('monitoring:read')" index="/connectivity/topology">
            <el-icon><Connection /></el-icon>
            <template #title>{{ t('nav.vpnTopology') }}</template>
          </el-menu-item>
        </el-sub-menu>
        <el-sub-menu v-if="canAnyAccess(['policies:read', 'acls:read', 'qos:read'])" index="policy-center">
          <template #title>
            <el-icon><Tickets /></el-icon>
            <span>{{ t('nav.policyCenter') }}</span>
          </template>
          <el-menu-item v-if="canAccess('policies:read')" index="/policy-center">
            <el-icon><Tickets /></el-icon>
            <template #title>{{ t('nav.policyCenter') }}</template>
          </el-menu-item>
          <el-menu-item v-if="canAccess('acls:read')" index="/policy-center/acl-rules">
            <el-icon><Lock /></el-icon>
            <template #title>{{ t('nav.aclManagement') }}</template>
          </el-menu-item>
          <el-menu-item v-if="canAccess('qos:read')" index="/policy-center/bandwidth-control">
            <el-icon><Coin /></el-icon>
            <template #title>{{ t('nav.bandwidthControl') }}</template>
          </el-menu-item>
        </el-sub-menu>
        <el-menu-item v-if="canAccess('monitoring:read')" index="/monitoring">
          <el-icon><DataLine /></el-icon>
          <template #title>{{ t('nav.monitoringCenter') }}</template>
        </el-menu-item>
        <el-menu-item v-if="canAccess('ai:use')" index="/ai-copilot">
          <el-icon><ChatLineRound /></el-icon>
          <template #title>{{ t('nav.aiCopilot') }}</template>
        </el-menu-item>
        <el-sub-menu v-if="isSuperAdmin || canAnyAccess(['tokens:read', 'users:read', 'roles:read'])" index="platform">
          <template #title>
            <el-icon><Setting /></el-icon>
            <span>{{ t('nav.platform') }}</span>
          </template>
          <el-menu-item v-if="canAccess('tokens:read')" index="/platform/tokens">
            <el-icon><Key /></el-icon>
            <template #title>{{ t('nav.tokenManagement') }}</template>
          </el-menu-item>
          <el-menu-item v-if="canAccess('users:read')" index="/platform/tenants">
            <el-icon><User /></el-icon>
            <template #title>{{ t('nav.tenantManagement') }}</template>
          </el-menu-item>
          <el-menu-item v-if="canAccess('roles:read')" index="/platform/roles">
            <el-icon><Lock /></el-icon>
            <template #title>{{ t('nav.roleManagement') }}</template>
          </el-menu-item>
          <el-menu-item v-if="isSuperAdmin" index="/platform/settings">
            <el-icon><Setting /></el-icon>
            <template #title>{{ t('nav.settings') }}</template>
          </el-menu-item>
        </el-sub-menu>
      </el-menu>

      <!-- 侧边栏底部 -->
      <div class="sidebar-footer">
        <div class="status-indicator">
          <div class="status-dot online"></div>
          <span v-if="!isCollapsed" class="status-text">System Online</span>
        </div>
      </div>
    </el-aside>

    <!-- 主内容区域 -->
    <el-container class="main-container">
      <!-- 顶部导航栏 -->
      <el-header class="top-header">
        <div class="header-content">
          <!-- 左侧：折叠按钮和标题 -->
          <div class="header-left">
            <el-button
              :icon="isCollapsed ? Expand : Fold"
              @click="toggleSidebar"
              class="menu-toggle"
              text
            />
            <div class="page-title">{{ getPageTitle }}</div>
          </div>

          <!-- 右侧：操作区 -->
          <div class="header-right">
            <el-button
              link
              :icon="Document"
              @click="openHelp"
              class="header-btn"
            >
              {{ t('header.help') }}
            </el-button>

            <el-divider direction="vertical" class="header-divider" />

            <el-button
              link
              @click="toggleLang"
              class="header-btn lang-btn"
            >
              {{ currentLang === 'en' ? '中文' : 'EN' }}
            </el-button>

            <el-divider direction="vertical" class="header-divider" />

            <span class="version-badge">v{{ appVersion }}</span>

            <el-divider direction="vertical" class="header-divider" />

            <!-- 租户选择器 -->
            <TenantSelector />

            <el-divider direction="vertical" class="header-divider" />

            <!-- 用户菜单 -->
            <el-dropdown @command="handleUserAction" trigger="click">
              <div class="user-profile">
                <el-avatar :size="36" class="user-avatar">
                  {{ currentUser?.initials }}
                </el-avatar>
                <span class="user-name">{{ currentUser?.name }}</span>
                <el-icon class="dropdown-icon"><CaretBottom /></el-icon>
              </div>
              <template #dropdown>
                <el-dropdown-menu class="user-dropdown">
                  <el-dropdown-item command="profile">
                    <el-icon><User /></el-icon>
                    <span>{{ t('header.profile') }}</span>
                  </el-dropdown-item>
                  <el-dropdown-item v-if="isSuperAdmin" command="settings">
                    <el-icon><Setting /></el-icon>
                    <span>{{ t('nav.settings') }}</span>
                  </el-dropdown-item>
                  <el-dropdown-item command="logout" divided>
                    <el-icon><SwitchButton /></el-icon>
                    <span>{{ t('header.logout') }}</span>
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </div>
      </el-header>

      <!-- 主内容（浅色） -->
      <el-main class="main-content">
        <router-view v-slot="{ Component }">
          <transition name="page-fade" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup>
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  House,
  Monitor,
  DataLine,
  User,
  Setting,
  Fold,
  Expand,
  Document,
  Key,
  Tickets,
  Position,
  Connection,
  Coin,
  Lock,
  ChatLineRound,
  CaretBottom,
  SwitchButton
} from '@element-plus/icons-vue'
import { useAppStore, useUserStore, useTenantStore } from '@/stores'
import { usePermission } from '@/composables/usePermission'
import { t } from '@/i18n'
import TenantSelector from './TenantSelector.vue'

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const userStore = useUserStore()
const tenantStore = useTenantStore()
const { hasPermission, hasAnyPermission } = usePermission()

const isCollapsed = computed(() => appStore.sidebarCollapsed)
const currentUser = computed(() => userStore.user)
const currentLang = computed(() => appStore.lang)
const appVersion = computed(() => appStore.version)
const sidebarWidth = computed(() => isCollapsed.value ? '72px' : '240px')
const defaultOpeneds = computed(() => isCollapsed.value ? [] : ['connectivity', 'policy-center', 'platform'])
const isSuperAdmin = computed(() => currentUser.value?.role === 'super_admin')

const canAccess = hasPermission
const canAnyAccess = hasAnyPermission

const activeMenu = computed(() => {
  if (route.name === 'NodeMonitorDetail' || route.path.startsWith('/monitoring/nodes/')) {
    return '/monitoring'
  }

  return route.path
})

const getPageTitle = computed(() => {
  const routeMap = {
    Dashboard: t('nav.dashboard'),
    Nodes: t('nav.nodes'),
    Routing: t('nav.routingManagement'),
    VpnTopology: t('nav.vpnTopology'),
    Policies: t('nav.policyCenter'),
    ACLRules: t('nav.aclManagement'),
    BandwidthControl: t('nav.bandwidthControl'),
    Tokens: t('nav.tokenManagement'),
    TenantManagement: t('nav.tenantManagement'),
    Roles: t('nav.roleManagement'),
    Monitoring: t('nav.monitoringCenter'),
    NodeMonitorDetail: t('nav.monitoringCenter'),
    AiAssistant: t('nav.aiCopilot'),
    Settings: t('nav.settings')
  }
  return routeMap[route.name] || t('common.dashboard')
})

const toggleSidebar = () => {
  appStore.toggleSidebar()
}

const toggleLang = () => {
  const newLang = currentLang.value === 'en' ? 'zh' : 'en'
  appStore.setLang(newLang)
}

const handleUserAction = (command) => {
  switch(command) {
    case 'logout':
      userStore.logout()
      router.push('/login')
      break
    case 'profile':
      // Navigate to profile page
      break
    case 'settings':
      router.push('/platform/settings')
      break
  }
}

const openHelp = () => {
  window.open('https://docs.aria.example.com', '_blank')
}
</script>

<style scoped>
/* ============================================
   Layout Container
   ============================================ */
.layout-container {
  height: 100vh;
  background: var(--aria-sidebar-bg);
}

/* ============================================
   Sidebar (深色主题)
   ============================================ */
.sidebar {
  background: var(--aria-sidebar-bg);
  border-right: 1px solid var(--aria-dark-border-primary);
  transition: width 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  display: flex;
  flex-direction: column;
  position: relative;
  z-index: 20;
  overflow: hidden;
}

/* Sidebar Header */
.sidebar-header {
  display: flex;
  align-items: center;
  padding: 20px;
  height: 72px;
  border-bottom: 1px solid var(--aria-dark-border-primary);
  transition: all 0.3s ease;
  flex-shrink: 0;
}

.logo-wrapper {
  display: flex;
  align-items: center;
  justify-content: center;
  position: relative;
  margin-right: 12px;
  flex-shrink: 0;
}

.logo-glow {
  position: absolute;
  inset: -8px;
  background: radial-gradient(circle, rgba(59, 130, 246, 0.3) 0%, transparent 70%);
  filter: blur(8px);
  opacity: 0;
  transition: opacity 0.3s ease;
}

.logo-wrapper:hover .logo-glow {
  opacity: 1;
}

.logo-image {
  width: 36px;
  height: 36px;
  object-fit: contain;
  position: relative;
  z-index: 1;
}

.logo-text {
  margin: 0;
  font-size: 18px;
  font-weight: 700;
  color: var(--aria-dark-text-primary);
  white-space: nowrap;
  letter-spacing: -0.3px;
  transition: all 0.3s ease;
}

.pro-badge {
  font-size: 10px;
  padding: 3px 8px;
  background: linear-gradient(135deg, var(--aria-primary) 0%, var(--aria-primary-dark) 100%);
  border-radius: var(--radius-full);
  margin-left: 8px;
  letter-spacing: 0.5px;
  font-weight: 600;
  color: white;
}

/* Sidebar Menu */
.sidebar-menu {
  flex: 1;
  min-height: 0;
  border: none;
  background: transparent;
  padding: 16px 12px;
  overflow-x: hidden;
  overflow-y: auto;
  scrollbar-width: thin;
  scrollbar-color: rgba(148, 163, 184, 0.35) transparent;
}

.sidebar-menu::-webkit-scrollbar {
  width: 6px;
}

.sidebar-menu::-webkit-scrollbar-track {
  background: transparent;
}

.sidebar-menu::-webkit-scrollbar-thumb {
  background: rgba(148, 163, 184, 0.35);
  border-radius: 999px;
}

:deep(.sidebar-menu .el-menu-item) {
  color: var(--aria-dark-text-secondary);
  height: 48px;
  line-height: 48px;
  border-radius: var(--aria-radius);
  margin-bottom: 4px;
  transition: all var(--aria-transition-base);
  position: relative;
  overflow: hidden;
}

:deep(.sidebar-menu .el-sub-menu__title) {
  color: var(--aria-dark-text-secondary);
  height: 48px;
  line-height: 48px;
  border-radius: var(--aria-radius);
  margin-bottom: 4px;
  transition: all var(--aria-transition-base);
}

:deep(.sidebar-menu .el-sub-menu__title:hover) {
  background: rgba(255, 255, 255, 0.05);
  color: var(--aria-dark-text-primary);
}

:deep(.sidebar-menu .el-sub-menu.is-opened > .el-sub-menu__title) {
  color: var(--aria-dark-text-primary);
}

:deep(.sidebar-menu .el-menu--inline) {
  background: transparent;
}

:deep(.sidebar-menu .el-menu--inline .el-menu-item) {
  padding-left: 52px !important;
  height: 44px;
  line-height: 44px;
}

:deep(.sidebar-menu .el-menu-item::before) {
  content: '';
  position: absolute;
  left: 0;
  top: 0;
  height: 100%;
  width: 3px;
  background: var(--aria-primary);
  transform: scaleY(0);
  transition: transform var(--aria-transition-base);
}

:deep(.sidebar-menu .el-menu-item:hover) {
  background: rgba(255, 255, 255, 0.05);
  color: var(--aria-dark-text-primary);
}

:deep(.sidebar-menu .el-menu-item.is-active) {
  background: linear-gradient(135deg, rgba(59, 130, 246, 0.2) 0%, rgba(59, 130, 246, 0.1) 100%);
  color: var(--aria-primary);
  font-weight: 500;
}

:deep(.sidebar-menu .el-menu-item.is-active::before) {
  transform: scaleY(1);
}

:deep(.sidebar-menu .el-menu-item .el-icon) {
  font-size: 20px;
  transition: transform var(--aria-transition-base);
}

:deep(.sidebar-menu .el-menu-item:hover .el-icon) {
  transform: scale(1.1);
}

/* Sidebar Footer */
.sidebar-footer {
  padding: 16px;
  border-top: 1px solid var(--aria-dark-border-primary);
  flex-shrink: 0;
}

.status-indicator {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  background: rgba(255, 255, 255, 0.05);
  border-radius: var(--aria-radius);
  border: 1px solid var(--aria-dark-border-primary);
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.status-dot.online {
  background: var(--aria-success);
  box-shadow: 0 0 8px rgba(34, 197, 94, 0.5);
  animation: pulse-dot 2s ease-in-out infinite;
}

@keyframes pulse-dot {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.6; }
}

.status-text {
  font-size: 12px;
  color: var(--aria-dark-text-secondary);
  font-weight: 500;
  white-space: nowrap;
}

/* ============================================
   Main Container
   ============================================ */
.main-container {
  display: flex;
  flex-direction: column;
  background: var(--aria-content-bg);
  overflow: hidden;
}

/* ============================================
   Top Header
   ============================================ */
.top-header {
  padding: 0;
  background: var(--aria-header-bg);
  border-bottom: 1px solid var(--aria-border-primary);
  height: 64px;
  box-shadow: var(--aria-shadow-sm);
  position: relative;
  z-index: 10;
}

.header-content {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
  height: 100%;
  max-width: 100%;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 16px;
  flex: 1;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 4px;
}

.menu-toggle {
  color: var(--aria-text-secondary);
  transition: all var(--aria-transition-fast);
}

.menu-toggle:hover {
  color: var(--aria-text-primary);
  background: var(--aria-content-bg-tertiary);
}

.page-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--aria-text-primary);
  letter-spacing: 0;
}

.header-btn {
  color: var(--aria-text-secondary);
  font-weight: 500;
  font-size: 14px;
  transition: all var(--aria-transition-fast);
}

.header-btn:hover {
  color: var(--aria-primary);
}

.lang-btn {
  min-width: 32px;
  text-align: center;
}

.header-divider {
  height: 24px;
  border-color: var(--aria-border-primary);
  margin: 0 8px;
}

.version-badge {
  color: var(--aria-text-muted);
  font-size: 12px;
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  padding: 4px 8px;
  background: var(--aria-content-bg-tertiary);
  border-radius: var(--aria-radius-sm);
  border: 1px solid var(--aria-border-primary);
}

/* User Profile */
.user-profile {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 12px;
  border-radius: var(--aria-radius);
  cursor: pointer;
  transition: all var(--aria-transition-fast);
  border: 1px solid transparent;
}

.user-profile:hover {
  background: var(--aria-content-bg-tertiary);
  border-color: var(--aria-border-primary);
}

.user-avatar {
  background: linear-gradient(135deg, var(--aria-primary) 0%, var(--aria-primary-dark) 100%);
  color: white;
  font-weight: 600;
  flex-shrink: 0;
}

.user-name {
  font-weight: 500;
  font-size: 14px;
  color: var(--aria-text-primary);
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dropdown-icon {
  color: var(--aria-text-muted);
  font-size: 14px;
  transition: transform var(--aria-transition-fast);
}

.user-profile:hover .dropdown-icon {
  transform: rotate(180deg);
}

/* User Dropdown */
:deep(.user-dropdown) {
  background: var(--aria-content-bg-secondary);
  border: 1px solid var(--aria-border-primary);
  box-shadow: var(--aria-shadow-lg);
  border-radius: var(--aria-radius-md);
  padding: 8px;
  min-width: 180px;
}

:deep(.user-dropdown .el-dropdown-menu__item) {
  color: var(--aria-text-secondary);
  padding: 10px 12px;
  border-radius: var(--aria-radius-sm);
  display: flex;
  align-items: center;
  gap: 10px;
  transition: all var(--aria-transition-fast);
}

:deep(.user-dropdown .el-dropdown-menu__item:hover) {
  background: var(--aria-content-bg-tertiary);
  color: var(--aria-text-primary);
}

:deep(.user-dropdown .el-dropdown-menu__item .el-icon) {
  font-size: 16px;
}

:deep(.user-dropdown .el-dropdown-menu__item--divided::before) {
  background: var(--aria-border-primary);
}

/* ============================================
   Main Content (浅色主题)
   ============================================ */
.main-content {
  background: var(--aria-content-bg);
  padding: 24px;
  overflow-y: auto;
  overflow-x: hidden;
}

/* ============================================
   Page Transitions
   ============================================ */
.page-fade-enter-active,
.page-fade-leave-active {
  transition: all 0.2s ease;
}

.page-fade-enter-from {
  opacity: 0;
  transform: translateY(10px);
}

.page-fade-leave-to {
  opacity: 0;
  transform: translateY(-10px);
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.3s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

/* ============================================
   Responsive
   ============================================ */
@media (max-width: 1024px) {
  .page-title {
    display: none;
  }
}

@media (max-width: 768px) {
  .sidebar {
    position: fixed;
    left: 0;
    top: 0;
    height: 100%;
    z-index: 100;
    transform: translateX(-100%);
  }

  .sidebar.mobile-open {
    transform: translateX(0);
  }

  .header-content {
    padding: 0 16px;
  }

  .header-btn {
    display: none;
  }

  .version-badge {
    display: none;
  }

  .header-divider {
    display: none;
  }

  .user-name {
    display: none;
  }

  .main-content {
    padding: 16px;
  }
}
</style>
