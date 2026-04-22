import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

const {
  permissionSet,
  mockNodeStore
} = vi.hoisted(() => ({
  permissionSet: new Set(),
  mockNodeStore: {
    nodes: [],
    loading: false,
    loadNodes: vi.fn(async () => {}),
    loadNodeDetail: vi.fn(async () => null),
    updateNodeRemote: vi.fn(async () => {}),
    deleteNode: vi.fn(() => {})
  }
}))

const hasPermission = (permission) => permissionSet.has('*') || permissionSet.has(permission)

vi.mock('@/composables/usePermission', () => ({
  usePermission: () => ({
    hasPermission
  })
}))

vi.mock('/src/composables/usePermission', () => ({
  usePermission: () => ({
    hasPermission
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
    getNodeMetrics: vi.fn(async () => ({ upload_mbps: 0, download_mbps: 0, latency_ms: 0 }))
  }
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: vi.fn()
  })
}))

vi.mock('@/composables/useTenantApi', () => ({
  useTenantApi: {
    getTenantNodes: vi.fn(async () => [])
  }
}))

vi.mock('@/composables/useAclApi', () => ({
  useAclApi: {
    getACLRulesByNode: vi.fn(async () => []),
    createACLRule: vi.fn(async () => ({})),
    updateACLRule: vi.fn(async () => ({})),
    deleteACLRule: vi.fn(async () => ({}))
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
    deleteBackup: vi.fn(async () => ({})),
    downloadBackupUrl: vi.fn((id) => `/v2/settings/backups/${id}/download`)
  }
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    lang: 'zh',
    setLang: vi.fn()
  })
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
import Tokens from '@/views/Tokens.vue'
import Settings from '@/views/Settings.vue'
import Nodes from '@/views/Nodes.vue'

const elementStubs = {
  'el-card': { template: '<div><slot name="header" /><slot /></div>' },
  'el-input': { template: '<div><slot name="prefix" /><slot name="append" /></div>' },
  'el-icon': { template: '<i><slot /></i>' },
  'el-tag': { template: '<span><slot /></span>' },
  'el-select': { template: '<div><slot /></div>' },
  'el-option': { template: '<div></div>' },
  'el-divider': { template: '<div></div>' },
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
          policy_status: 'idle',
          pending_cmds: 0,
          last_delivery_command_id: '',
          last_command_error: '',
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

const mountWithStubs = (component) =>
  mount(component, {
    global: {
      stubs: elementStubs,
      directives: {
        loading: {}
      }
    }
  })

describe('page-level RBAC button visibility', () => {
  beforeEach(() => {
    permissionSet.clear()
    mockNodeStore.loadNodes.mockClear()

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

    permissionSet.clear()
    const denied = mountWithStubs(ACLRules)
    await flushPromises()
    expect(denied.text()).not.toContain('新建规则')
    expect(denied.text()).not.toContain('编辑')
    expect(denied.text()).not.toContain('删除')
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

  it('disables Settings write buttons when settings:write is missing', async () => {
    permissionSet.add('settings:write')
    const allowed = mountWithStubs(Settings)
    await flushPromises()
    const allowedButtons = allowed.findAll('button')
    expect(allowedButtons.some((b) => b.attributes('disabled') !== undefined)).toBe(false)

    permissionSet.clear()
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
})
