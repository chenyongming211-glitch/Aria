import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

const {
  routeState,
  aclApiMock,
  qosApiMock,
  routeApiMock,
  tenantApiMock,
  ipGroupApiMock,
  routerPushMock
} = vi.hoisted(() => ({
  routeState: {
    query: {},
    fullPath: '/'
  },
  aclApiMock: {
    getACLRulesByNode: vi.fn(async () => [
      {
        id: 'acl-1',
        policy_ref: 'acl-1',
        node_id: 'node-1',
        name: 'allow-office',
        last_delivery_command_id: 'cmd-1',
        stats: {}
      },
      {
        id: 'acl-2',
        policy_ref: 'acl-2',
        node_id: 'node-1',
        name: 'deny-lab',
        last_delivery_command_id: 'cmd-2',
        stats: {}
      }
    ])
  },
  qosApiMock: {
    getQoSRulesByNode: vi.fn(async () => [
      {
        id: 'qos-1',
        policy_ref: 'qos-1',
        node_id: 'node-1',
        description: 'limit office',
        last_delivery_command_id: 'cmd-1',
        stats: {}
      },
      {
        id: 'qos-2',
        policy_ref: 'qos-2',
        node_id: 'node-1',
        description: 'limit lab',
        last_delivery_command_id: 'cmd-2',
        stats: {}
      }
    ])
  },
  routeApiMock: {
    getRoutes: vi.fn(async () => [
      {
        id: '10.0.1.0/24',
        policyRef: '10.0.1.0/24',
        policy_ref: '10.0.1.0/24',
        nodeId: 'node-1',
        nodeName: 'edge-1',
        cidr: '10.0.1.0/24',
        lastDeliveryCommandId: 'cmd-1'
      },
      {
        id: '10.0.2.0/24',
        policyRef: '10.0.2.0/24',
        policy_ref: '10.0.2.0/24',
        nodeId: 'node-1',
        nodeName: 'edge-1',
        cidr: '10.0.2.0/24',
        lastDeliveryCommandId: 'cmd-2'
      }
    ])
  },
  tenantApiMock: {
    getTenantNodes: vi.fn(async () => [{ id: 'node-1', hostname: 'edge-1' }])
  },
  ipGroupApiMock: {
    listIPGroups: vi.fn(async () => [])
  },
  routerPushMock: vi.fn()
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: routerPushMock }),
  useRoute: () => routeState
}))

vi.mock('@/composables/useAclApi', () => ({
  useAclApi: aclApiMock
}))

vi.mock('@/composables/useQosApi', () => ({
  useQosApi: qosApiMock
}))

vi.mock('@/composables/useRouteApi', () => ({
  useRouteApi: routeApiMock
}))

vi.mock('@/composables/useTenantApi', () => ({
  useTenantApi: tenantApiMock
}))

vi.mock('@/composables/useIpGroupApi', () => ({
  useIpGroupApi: ipGroupApiMock
}))

vi.mock('@/composables/usePermission', () => ({
  usePermission: () => ({
    hasPermission: () => true
  })
}))

vi.mock('@/composables/useTenantChangeReload', () => ({
  useTenantChangeReload: vi.fn()
}))

vi.mock('element-plus', () => ({
  ElMessage: {
    error: vi.fn(),
    success: vi.fn(),
    warning: vi.fn()
  },
  ElMessageBox: {
    confirm: vi.fn(async () => true)
  }
}))

import ACLRules from '@/views/ACLRules.vue'
import BandwidthControl from '@/views/BandwidthControl.vue'
import IPGroups from '@/views/IPGroups.vue'
import Routing from '@/views/Routing.vue'

const stubs = {
  PageHeader: { template: '<div><slot name="actions" /><slot /></div>' },
  FilterBar: { template: '<div><slot name="filters" /><slot name="actions" /></div>' },
  DataPanel: { template: '<div><slot /></div>' },
  ActionIconButton: { template: '<button @click="$emit(\'click\')"><slot /></button>' },
  'el-card': { template: '<div><slot name="header" /><slot /></div>' },
  'el-row': { template: '<div><slot /></div>' },
  'el-col': { template: '<div><slot /></div>' },
  'el-select': { template: '<div><slot /></div>' },
  'el-option': { template: '<div></div>' },
  'el-input': { template: '<input />' },
  'el-input-number': { template: '<input />' },
  'el-button': { template: '<button @click="$emit(\'click\')"><slot /></button>' },
  'el-icon': { template: '<i><slot /></i>' },
  'el-alert': { template: '<div></div>' },
  'el-tag': { template: '<span><slot /></span>' },
  'el-switch': { template: '<input />' },
  'el-tooltip': { template: '<span><slot /></span>' },
  'el-table': { template: '<div><slot /></div>' },
  'el-table-column': { template: '<div></div>' },
  'el-pagination': { template: '<div></div>' },
  'el-dialog': { template: '<div><slot /><slot name="footer" /></div>' },
  'el-form': { template: '<form><slot /></form>' },
  'el-form-item': { template: '<div><slot /></div>' }
}

const mountPage = (component) => mount(component, {
  global: {
    stubs,
    directives: {
      loading: {}
    }
  }
})

describe('policy special page context filters', () => {
  beforeEach(() => {
    routeState.query = {}
    routeState.fullPath = '/'
    routerPushMock.mockClear()
    aclApiMock.getACLRulesByNode.mockClear()
    qosApiMock.getQoSRulesByNode.mockClear()
    routeApiMock.getRoutes.mockClear()
    tenantApiMock.getTenantNodes.mockClear()
    ipGroupApiMock.listIPGroups.mockClear()
  })

  it('filters ACL rules by policyRef without mutating the name search box', async () => {
    routeState.query = { nodeId: 'node-1', policyRef: 'acl-1', commandId: 'stale-cmd' }
    routeState.fullPath = '/acl-rules?nodeId=node-1&policyRef=acl-1&commandId=stale-cmd'

    const wrapper = mountPage(ACLRules)
    await flushPromises()

    expect(wrapper.vm.filters.name).toBe('')
    expect(wrapper.vm.visibleRules.map((rule) => rule.id)).toEqual(['acl-1'])
    expect(wrapper.find('[data-testid="policy-context-banner"]').text()).toContain('acl-1')
  })

  it('filters QoS rules by policyRef even when commandId is stale', async () => {
    routeState.query = { node_id: 'node-1', policy_ref: 'qos-1', command_id: 'stale-cmd' }
    routeState.fullPath = '/bandwidth?node_id=node-1&policy_ref=qos-1&command_id=stale-cmd'

    const wrapper = mountPage(BandwidthControl)
    await flushPromises()

    expect(wrapper.vm.visibleRules.map((rule) => rule.id)).toEqual(['qos-1'])
    expect(wrapper.find('[data-testid="policy-context-banner"]').text()).toContain('QoS')
  })

  it('filters routes by policy_ref without using the free-text search field', async () => {
    routeState.query = { node_id: 'node-1', policy_ref: '10.0.1.0/24', command_id: 'stale-cmd' }
    routeState.fullPath = '/routing?node_id=node-1&policy_ref=10.0.1.0/24&command_id=stale-cmd'

    const wrapper = mountPage(Routing)
    await flushPromises()

    expect(wrapper.vm.searchQuery).toBe('')
    expect(wrapper.vm.filteredRoutes.map((route) => route.cidr)).toEqual(['10.0.1.0/24'])
    expect(wrapper.find('[data-testid="policy-context-banner"]').text()).toContain('10.0.1.0/24')
  })

  it('does not show a context banner for ordinary policy page visits', async () => {
    const wrapper = mountPage(BandwidthControl)
    await flushPromises()

    expect(wrapper.find('[data-testid="policy-context-banner"]').exists()).toBe(false)
  })

  it('clears ACL route context without changing the free-text filter', async () => {
    routeState.query = { nodeId: 'node-1', policyRef: 'acl-1', commandId: 'cmd-1' }
    routeState.fullPath = '/acl-rules?nodeId=node-1&policyRef=acl-1&commandId=cmd-1'

    const wrapper = mountPage(ACLRules)
    await flushPromises()

    await wrapper.find('[data-testid="clear-policy-context"]').trigger('click')

    expect(wrapper.vm.filters.name).toBe('')
    expect(routerPushMock).toHaveBeenCalledWith({ name: 'ACLRules' })
  })

  it('opens node detail from route context and preserves policy trace', async () => {
    routeState.query = { node_id: 'node-1', policy_ref: '10.0.1.0/24', command_id: 'cmd-1' }
    routeState.fullPath = '/routing?node_id=node-1&policy_ref=10.0.1.0/24&command_id=cmd-1'

    const wrapper = mountPage(Routing)
    await flushPromises()

    await wrapper.find('[data-testid="open-context-node"]').trigger('click')

    expect(routerPushMock).toHaveBeenCalledWith({
      name: 'NodeMonitorDetail',
      params: { nodeId: 'node-1' },
      query: {
        commandId: 'cmd-1',
        policyRef: '10.0.1.0/24',
        policyDomain: 'route'
      }
    })
  })

  it('returns from a special page to Policy Center with the same context', async () => {
    routeState.query = { nodeId: 'node-1', policyRef: 'qos-1', commandId: 'cmd-1' }
    routeState.fullPath = '/bandwidth?nodeId=node-1&policyRef=qos-1&commandId=cmd-1'

    const wrapper = mountPage(BandwidthControl)
    await flushPromises()

    await wrapper.find('[data-testid="open-context-policy-center"]').trigger('click')

    expect(routerPushMock).toHaveBeenCalledWith({
      name: 'Policies',
      query: {
        nodeId: 'node-1',
        commandId: 'cmd-1',
        policyRef: 'qos-1',
        kind: 'qos'
      }
    })
  })

  it('returns node-only special page context to Policy Center', async () => {
    routeState.query = { nodeId: 'node-1' }
    routeState.fullPath = '/acl-rules?nodeId=node-1'

    const wrapper = mountPage(ACLRules)
    await flushPromises()

    await wrapper.find('[data-testid="open-context-policy-center"]').trigger('click')

    expect(routerPushMock).toHaveBeenCalledWith({
      name: 'Policies',
      query: {
        nodeId: 'node-1',
        kind: 'acl'
      }
    })
  })

  it('keeps policy context visible on IP Group management and returns to Policy Center', async () => {
    routeState.query = { node_id: 'node-1', policy_ref: 'acl-1', policy_domain: 'acl', command_id: 'cmd-1' }
    routeState.fullPath = '/ip-groups?node_id=node-1&policy_ref=acl-1&policy_domain=acl&command_id=cmd-1'

    const wrapper = mountPage(IPGroups)
    await flushPromises()

    expect(wrapper.find('[data-testid="policy-context-banner"]').text()).toContain('acl-1')

    await wrapper.find('[data-testid="open-context-policy-center"]').trigger('click')

    expect(routerPushMock).toHaveBeenCalledWith({
      name: 'Policies',
      query: {
        nodeId: 'node-1',
        policyRef: 'acl-1',
        kind: 'acl',
        commandId: 'cmd-1'
      }
    })
  })

  it('keeps kind-only IP Group context visible and returns to the filtered Policy Center', async () => {
    routeState.query = { kind: 'acl' }
    routeState.fullPath = '/ip-groups?kind=acl'

    const wrapper = mountPage(IPGroups)
    await flushPromises()

    expect(wrapper.find('[data-testid="policy-context-banner"]').exists()).toBe(true)

    await wrapper.find('[data-testid="open-context-policy-center"]').trigger('click')

    expect(routerPushMock).toHaveBeenCalledWith({
      name: 'Policies',
      query: {
        kind: 'acl'
      }
    })
  })

  it('opens node detail from IP Group context without losing policy trace', async () => {
    routeState.query = { nodeId: 'node-1', policyRef: 'acl-1', kind: 'acl', commandId: 'cmd-1' }
    routeState.fullPath = '/ip-groups?nodeId=node-1&policyRef=acl-1&kind=acl&commandId=cmd-1'

    const wrapper = mountPage(IPGroups)
    await flushPromises()

    await wrapper.find('[data-testid="open-context-node"]').trigger('click')

    expect(routerPushMock).toHaveBeenCalledWith({
      name: 'NodeMonitorDetail',
      params: { nodeId: 'node-1' },
      query: {
        policyRef: 'acl-1',
        policyDomain: 'acl',
        commandId: 'cmd-1'
      }
    })
  })
})
