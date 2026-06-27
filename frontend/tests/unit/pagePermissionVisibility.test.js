import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import useActualUserStore from '@/stores/user'

const {
  permissionSet,
  mockNodeStore,
  mockUserStore,
  mockRoute
} = vi.hoisted(() => ({
  permissionSet: new Set(),
  mockNodeStore: {
    nodes: [],
    loading: false,
    loadNodes: vi.fn(async () => {}),
    loadNodeDetail: vi.fn(async () => null),
    updateNodeRemote: vi.fn(async () => {}),
    deleteNode: vi.fn(() => {})
  },
  mockUserStore: {
    user: { role: 'admin' },
    permissions: []
  },
  mockRoute: {
    name: 'Dashboard',
    path: '/dashboard',
    meta: { titleKey: 'nav.dashboard' }
  }
}))

const hasPermission = (permission) => permissionSet.has('*') || permissionSet.has(permission)
const hasAnyPermission = (permissions) => permissions.some((permission) => hasPermission(permission))

vi.mock('@/composables/usePermission', () => ({
  usePermission: () => ({
    hasPermission,
    hasAnyPermission
  })
}))

vi.mock('/src/composables/usePermission', () => ({
  usePermission: () => ({
    hasPermission,
    hasAnyPermission
  })
}))

vi.mock('/src/stores/node', () => ({
  default: () => mockNodeStore
}))

vi.mock('/src/composables/useAgentProxyApi', () => ({
  useAgentProxyApi: {
    sendAgentCommand: vi.fn(async () => ({})),
    getAgentStatus: vi.fn(async () => ({})),
    getAgentCommands: vi.fn(async () => ({ items: [] }))
  }
}))

vi.mock('/src/composables/useMonitorApi', () => ({
  useMonitorApi: {
    getNodeMetrics: vi.fn(async () => ({ upload_mbps: 0, download_mbps: 0, latency_ms: 0 })),
    getStats: vi.fn(async () => ({ active_alerts_count: 1 })),
    getEvents: vi.fn(async () => ({ items: [], total: 0 })),
    getAlerts: vi.fn(async () => ({
      items: [
        {
          id: 'alert-1',
          alert_type: 'sync_failed',
          severity: 'warning',
          title: 'Sync failed',
          message: 'sync failed',
          context: {}
        }
      ]
    })),
    resolveAlert: vi.fn(async () => ({}))
  }
}))

vi.mock('vue-router', () => ({
  useRoute: () => mockRoute,
  useRouter: () => ({
    push: vi.fn()
  })
}))

vi.mock('@/composables/useTenantApi', () => ({
  useTenantApi: {
    getTenantNodes: vi.fn(async () => [])
  }
}))

vi.mock('@/composables/useRouteApi', () => ({
  useRouteApi: {
    getRoutes: vi.fn(async () => []),
    addRoute: vi.fn(async () => ({})),
    updateRoute: vi.fn(async () => ({})),
    deleteRoute: vi.fn(async () => ({}))
  }
}))

vi.mock('@/composables/useQosApi', () => ({
  useQosApi: {
    getQoSRules: vi.fn(async () => []),
    getQoSRulesByNode: vi.fn(async () => []),
    createQoSRule: vi.fn(async () => ({})),
    updateQoSRule: vi.fn(async () => ({})),
    deleteQoSRule: vi.fn(async () => ({})),
    retryQoSPolicySync: vi.fn(async () => ({})),
    getProtocolName: vi.fn((protocol) => String(protocol))
  }
}))

vi.mock('@/composables/useAclApi', () => ({
  useAclApi: {
    getACLRulesByNode: vi.fn(async () => []),
    createACLRule: vi.fn(async () => ({})),
    updateACLRule: vi.fn(async () => ({})),
    deleteACLRule: vi.fn(async () => ({})),
    retryACLPolicySync: vi.fn(async () => ({}))
  }
}))

vi.mock('@/composables/useIpGroupApi', () => ({
  useIpGroupApi: {
    listIPGroups: vi.fn(async () => []),
    createIPGroup: vi.fn(async () => ({})),
    updateIPGroup: vi.fn(async () => ({})),
    deleteIPGroup: vi.fn(async () => ({})),
    formatGroupLabel: vi.fn((group) => group?.name || 'any')
  }
}))

vi.mock('@/composables/useTokenApi', () => ({
  useTokenApi: {
    getAllTokens: vi.fn(async () => []),
    createToken: vi.fn(async () => ({})),
    getTokenDetail: vi.fn(async () => ({})),
    revokeToken: vi.fn(async () => ({}))
  }
}))

vi.mock('@/composables/useSettingsApi', () => ({
  useSettingsApi: {
    listBackups: vi.fn(async () => []),
    createBackup: vi.fn(async () => ({})),
    uploadBackup: vi.fn(async () => ({})),
    downloadBackup: vi.fn(async () => ({})),
    restoreBackup: vi.fn(async () => ({})),
    deleteBackup: vi.fn(async () => ({}))
  }
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    sidebarCollapsed: false,
    version: 'test',
    lang: 'zh',
    toggleSidebar: vi.fn(),
    setLang: vi.fn()
  }),
  useUserStore: () => mockUserStore,
  useTenantStore: () => ({})
}))

vi.mock('/src/stores', () => ({
  useAppStore: () => ({
    sidebarCollapsed: false,
    version: 'test',
    lang: 'zh',
    toggleSidebar: vi.fn(),
    setLang: vi.fn()
  }),
  useUserStore: () => mockUserStore,
  useTenantStore: () => ({})
}))

vi.mock('/src/stores/index', () => ({
  useAppStore: () => ({
    sidebarCollapsed: false,
    version: 'test',
    lang: 'zh',
    toggleSidebar: vi.fn(),
    setLang: vi.fn()
  }),
  useUserStore: () => mockUserStore,
  useTenantStore: () => ({})
}))

vi.mock('@/i18n', () => ({
  t: (key) => key
}))

vi.mock('element-plus', () => ({
  ElMessage: {
    success: vi.fn(),
    error: vi.fn(),
    warning: vi.fn(),
    info: vi.fn()
  },
  ElMessageBox: {
    alert: vi.fn(async () => {}),
    confirm: vi.fn(async () => {})
  },
  ElNotification: vi.fn()
}))

import ACLRules from '@/views/ACLRules.vue'
import IPGroups from '@/views/IPGroups.vue'
import Tokens from '@/views/Tokens.vue'
import Settings from '@/views/Settings.vue'
import Nodes from '@/views/Nodes.vue'
import Routing from '@/views/Routing.vue'
import BandwidthControl from '@/views/BandwidthControl.vue'
import Monitoring from '@/views/Monitoring.vue'
import Layout from '@/components/layout/Layout.vue'

const elementStubs = {
  'router-view': { template: '<div></div>' },
  transition: { template: '<div><slot /></div>' },
  TenantSelector: { template: '<div></div>' },
  'el-container': { template: '<div><slot /></div>' },
  'el-aside': { template: '<aside><slot /></aside>' },
  'el-header': { template: '<header><slot /></header>' },
  'el-main': { template: '<main><slot /></main>' },
  'el-divider': { template: '<span></span>' },
  'el-menu': {
    props: ['defaultActive'],
    template: '<nav class="sidebar-menu" :data-active="defaultActive"><slot /></nav>'
  },
  'el-menu-item': {
    props: ['index'],
    template: '<div class="el-menu-item" :data-index="index"><slot name="title" /><slot /></div>'
  },
  'el-sub-menu': {
    props: ['index'],
    template: '<div class="el-sub-menu" :data-index="index"><slot name="title" /><slot /></div>'
  },
  'el-dropdown': { template: '<div><slot /><slot name="dropdown" /></div>' },
  'el-dropdown-menu': { template: '<div><slot /></div>' },
  'el-dropdown-item': { template: '<div><slot /></div>' },
  'el-avatar': { template: '<span><slot /></span>' },
  'el-card': { template: '<div><slot name="header" /><slot /></div>' },
  'el-input': { template: '<div><slot name="prefix" /><slot name="append" /></div>' },
  'el-icon': { template: '<i><slot /></i>' },
  'el-tag': { template: '<span><slot /></span>' },
  'el-select': { template: '<div><slot /></div>' },
  'el-option': { template: '<div></div>' },
  'el-tooltip': { template: '<div><slot /></div>' },
  'el-switch': { template: '<div></div>' },
  'el-pagination': { template: '<div></div>' },
  'el-form': { template: '<form><slot /></form>' },
  'el-form-item': { template: '<div><slot /></div>' },
  'el-input-number': { template: '<div></div>' },
  'el-row': { template: '<div><slot /></div>' },
  'el-col': { template: '<div><slot /></div>' },
  'el-tabs': { template: '<div><slot /></div>' },
  'el-tab-pane': { template: '<div><slot /></div>' },
  'el-badge': { template: '<span><slot /></span>' },
  'el-radio-group': { template: '<div><slot /></div>' },
  'el-radio': { template: '<div><slot /></div>' },
  'el-slider': { template: '<div></div>' },
  'el-checkbox-group': { template: '<div><slot /></div>' },
  'el-checkbox': { template: '<div><slot /></div>' },
  'el-upload': { template: '<div><slot /></div>' },
  'el-alert': { template: '<div><slot /></div>' },
  'el-empty': { template: '<div></div>' },
  'el-descriptions': { template: '<div><slot /></div>' },
  'el-descriptions-item': { template: '<div><slot /></div>' },
  'el-dialog': { template: '<div><slot /><slot name="footer" /></div>' },
  'el-popconfirm': { template: '<div><slot name="reference" /></div>' },
  'el-table': { template: '<div><slot /></div>' },
  'el-table-column': {
    template: '<div><slot :row="row" /></div>',
    data() {
      return {
        row: {
          id: 'node-1',
          name: 'rule-1',
          node_id: 'node-1',
          region: 'sh',
          mode: 'kernel',
          status: 'online',
          min_port: 0,
          max_port: 65535,
          protocol: 6,
          action: 'allow',
          policy_status: 'error',
          policyStatus: 'error',
          pending_cmds: 0,
          last_delivery_command_id: '',
          last_command_error: 'apply failed',
          used_count: 0
        }
      }
    }
  },
  'el-button': {
    props: ['disabled'],
    template: '<button :disabled="disabled"><slot /></button>'
  }
}

const mountWithStubs = (component) => {
  syncPiniaUserStore()
  return mount(component, {
    global: {
      stubs: elementStubs,
      directives: {
        loading: {}
      }
    }
  })
}

const syncPiniaUserStore = () => {
  const userStore = useActualUserStore()
  userStore.user = { ...mockUserStore.user }
  userStore.permissions = Array.from(permissionSet)
}

describe('page-level RBAC button visibility', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    permissionSet.clear()
    mockNodeStore.loadNodes.mockClear()
    mockUserStore.user = { role: 'admin' }
    mockUserStore.permissions = []
    mockRoute.name = 'Dashboard'
    mockRoute.path = '/dashboard'
    mockRoute.meta = { titleKey: 'nav.dashboard' }

    const storage = new Map()
    globalThis.localStorage = {
      getItem: (key) => (storage.has(key) ? storage.get(key) : null),
      setItem: (key, value) => storage.set(key, String(value)),
      removeItem: (key) => storage.delete(key),
      clear: () => storage.clear()
    }
  })

  it('shows/hides ACLRules write actions based on acls:write', async () => {
    permissionSet.add('acls:write')
    const allowed = mountWithStubs(ACLRules)
    await flushPromises()
    expect(allowed.text()).toContain('新建规则')
    expect(allowed.text()).toContain('编辑')
    expect(allowed.text()).toContain('删除')
    expect(allowed.text()).toContain('重试')

    permissionSet.clear()
    const denied = mountWithStubs(ACLRules)
    await flushPromises()
    expect(denied.text()).not.toContain('新建规则')
    expect(denied.text()).not.toContain('编辑')
    expect(denied.text()).not.toContain('删除')
    expect(denied.text()).not.toContain('重试')
  })

  it('shows/hides Tokens create and revoke actions based on tokens:write', async () => {
    permissionSet.add('tokens:write')
    const allowed = mountWithStubs(Tokens)
    await flushPromises()
    expect(allowed.text()).toContain('common.add')
    expect(allowed.text()).toContain('common.revoke')

    permissionSet.clear()
    const denied = mountWithStubs(Tokens)
    await flushPromises()
    expect(denied.text()).not.toContain('common.add')
    expect(denied.text()).not.toContain('common.revoke')
  })

  it('disables Settings write buttons unless the user is super_admin', async () => {
    mockUserStore.user = { role: 'super_admin' }
    const allowed = mountWithStubs(Settings)
    await flushPromises()
    const allowedButtons = allowed.findAll('button')
    expect(allowedButtons.some((b) => b.attributes('disabled') !== undefined)).toBe(false)

    mockUserStore.user = { role: 'admin' }
    const denied = mountWithStubs(Settings)
    await flushPromises()
    const deniedButtons = denied.findAll('button')
    expect(deniedButtons.some((b) => b.attributes('disabled') !== undefined)).toBe(true)
  })

  it('shows/hides Nodes write actions based on nodes:write', async () => {
    permissionSet.add('nodes:write')
    const allowed = mountWithStubs(Nodes)
    await flushPromises()
    expect(allowed.text()).toContain('Add Node')
    expect(allowed.text()).toContain('Save Changes')

    permissionSet.clear()
    const denied = mountWithStubs(Nodes)
    await flushPromises()
    expect(denied.text()).not.toContain('Add Node')
    expect(denied.text()).not.toContain('Save Changes')
  })

  it('shows/hides Nodes command actions based on commands:write', async () => {
    permissionSet.add('commands:write')
    const allowed = mountWithStubs(Nodes)
    allowed.vm.selectedNode = {
      id: 'node-1',
      hostname: 'node-1',
      region: 'sh',
      status: 'online',
      routes: [],
      recentCommands: []
    }
    await allowed.vm.$nextTick()
    expect(allowed.text()).toContain('Force Sync')
    expect(allowed.text()).toContain('Health Check')

    permissionSet.clear()
    const denied = mountWithStubs(Nodes)
    denied.vm.selectedNode = {
      id: 'node-1',
      hostname: 'node-1',
      region: 'sh',
      status: 'online',
      routes: [],
      recentCommands: []
    }
    await denied.vm.$nextTick()
    expect(denied.text()).not.toContain('Force Sync')
    expect(denied.text()).not.toContain('Health Check')
  })

  it('renders the Nodes detail basic information heading once', async () => {
    permissionSet.add('commands:write')
    const wrapper = mountWithStubs(Nodes)
    wrapper.vm.selectedNode = {
      id: 'node-1',
      hostname: 'node-1',
      region: 'sh',
      status: 'online',
      routes: [],
      recentCommands: []
    }
    await wrapper.vm.$nextTick()

    const matches = wrapper.text().match(/Basic Information/g) || []
    expect(matches).toHaveLength(1)
  })

  it('labels monitoring node detail routes as monitoring and keeps the monitoring menu active', () => {
    mockUserStore.user = { role: 'super_admin' }
    permissionSet.add('*')
    mockRoute.name = 'NodeMonitorDetail'
    mockRoute.path = '/monitoring/nodes/node-1'
    mockRoute.meta = { titleKey: 'nav.monitoringCenter' }

    const wrapper = mountWithStubs(Layout)

    expect(wrapper.find('.page-title').text()).toBe('nav.monitoringCenter')
    expect(wrapper.find('.sidebar-menu').attributes('data-active')).toBe('/monitoring')
  })

  it('uses the active route titleKey for the top header title', () => {
    mockUserStore.user = { role: 'super_admin' }
    permissionSet.add('*')
    mockRoute.name = 'BandwidthControl'
    mockRoute.path = '/policy-center/bandwidth-control'
    mockRoute.meta = { titleKey: 'nav.bandwidthControl' }

    const wrapper = mountWithStubs(Layout)

    expect(wrapper.find('.page-title').text()).toBe('nav.bandwidthControl')
  })

  it('hides tenant management menu from non-super_admin users', () => {
    mockUserStore.user = { role: 'admin' }
    permissionSet.add('*')

    const denied = mountWithStubs(Layout)

    expect(denied.find('[data-index="/platform/tenants"]').exists()).toBe(false)

    mockUserStore.user = { role: 'super_admin' }
    const allowed = mountWithStubs(Layout)

    expect(allowed.find('[data-index="/platform/tenants"]').exists()).toBe(true)
  })

  it('does not show an empty platform menu for users:read-only users', () => {
    mockUserStore.user = { role: 'admin' }
    permissionSet.add('users:read')

    const wrapper = mountWithStubs(Layout)

    expect(wrapper.find('[data-index="platform"]').exists()).toBe(false)
  })

  it('shows/hides Routing write actions based on routes:write', async () => {
    permissionSet.add('routes:write')
    const allowed = mountWithStubs(Routing)
    await flushPromises()
    const allowedButtons = allowed.findAll('button').map((button) => button.text())
    expect(allowedButtons).toContain('添加路由')
    expect(allowedButtons).toContain('编辑')
    expect(allowedButtons).toContain('删除')

    permissionSet.clear()
    const denied = mountWithStubs(Routing)
    await flushPromises()
    const deniedButtons = denied.findAll('button').map((button) => button.text())
    expect(deniedButtons).not.toContain('添加路由')
    expect(deniedButtons).not.toContain('编辑')
    expect(deniedButtons).not.toContain('删除')
  })

  it('shows/hides Bandwidth write actions based on qos:write', async () => {
    permissionSet.add('qos:write')
    const allowed = mountWithStubs(BandwidthControl)
    await flushPromises()
    const allowedButtons = allowed.findAll('button').map((button) => button.text())
    expect(allowedButtons).toContain('添加规则')
    expect(allowedButtons).toContain('编辑')
    expect(allowedButtons).toContain('删除')
    expect(allowedButtons).toContain('重试')
    expect(allowedButtons).toContain('保存并应用')

    permissionSet.clear()
    const denied = mountWithStubs(BandwidthControl)
    await flushPromises()
    const deniedButtons = denied.findAll('button').map((button) => button.text())
    expect(deniedButtons).not.toContain('添加规则')
    expect(deniedButtons).not.toContain('编辑')
    expect(deniedButtons).not.toContain('删除')
    expect(deniedButtons).not.toContain('重试')
    expect(deniedButtons).not.toContain('保存并应用')
  })

  it('shows/hides IPGroups write actions based on ip-groups:write', async () => {
    permissionSet.add('ip-groups:write')
    const allowed = mountWithStubs(IPGroups)
    await flushPromises()
    expect(allowed.text()).toContain('新建 Group')
    expect(allowed.text()).toContain('保存')

    permissionSet.clear()
    const denied = mountWithStubs(IPGroups)
    await flushPromises()
    expect(denied.text()).not.toContain('新建 Group')
    expect(denied.text()).not.toContain('保存')
  })

  it('shows the IP Group menu only with ip-groups:read', () => {
    mockUserStore.user = { role: 'admin' }
    permissionSet.add('ip-groups:read')

    const allowed = mountWithStubs(Layout)
    expect(allowed.find('[data-index="/policy-center/ip-groups"]').exists()).toBe(true)

    permissionSet.clear()
    const denied = mountWithStubs(Layout)
    expect(denied.find('[data-index="/policy-center/ip-groups"]').exists()).toBe(false)
  })

  it('shows/hides Monitoring resolve actions based on commands:write', async () => {
    permissionSet.add('commands:write')
    const allowed = mountWithStubs(Monitoring)
    await flushPromises()
    expect(allowed.text()).toContain('Resolve')

    permissionSet.clear()
    const denied = mountWithStubs(Monitoring)
    await flushPromises()
    expect(denied.text()).not.toContain('Resolve')
  })
})
