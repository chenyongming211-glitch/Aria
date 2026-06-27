import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

const {
  routeState,
  routerPush,
  tenantApiMock,
  aclApiMock,
  qosApiMock,
  routeApiMock,
  ipGroupApiMock,
  policyApiMock
} = vi.hoisted(() => ({
  routeState: {
    params: {},
    query: {},
    fullPath: '/'
  },
  routerPush: vi.fn(),
  tenantApiMock: {
    getTenantNodes: vi.fn(async () => [
      { id: 'node-1', hostname: 'node-1', region: 'hangzhou' },
      { id: 'node-2', hostname: 'node-2', region: 'shanghai' }
    ])
  },
  aclApiMock: {
    getACLRulesByNode: vi.fn(async () => []),
    retryACLPolicySync: vi.fn(async () => ({
      policy_ref: 'acl-special',
      policy_status: 'pending',
      last_delivery_command_id: 'cmd-acl-retry'
    }))
  },
  qosApiMock: {
    getQoSRulesByNode: vi.fn(async () => []),
    retryQoSPolicySync: vi.fn(async () => ({
      policy_ref: 'qos-special',
      policyStatus: 'pending',
      last_delivery_command_id: 'cmd-qos-retry'
    }))
  },
  routeApiMock: {
    getRoutes: vi.fn(async () => [
      {
        id: 'route-1',
        nodeId: 'node-1',
        nodeName: 'node-1',
        publicIp: '1.1.1.1',
        region: 'hangzhou',
        cidr: '10.1.0.0/24'
      },
      {
        id: 'route-2',
        nodeId: 'node-2',
        nodeName: 'node-2',
        publicIp: '2.2.2.2',
        region: 'shanghai',
        cidr: '10.2.0.0/24'
      }
    ])
  },
  ipGroupApiMock: {
    listIPGroups: vi.fn(async () => [])
  },
  policyApiMock: {
    listPolicies: vi.fn(async () => [
      {
        policyId: 'policy-acl-2',
        policyRef: 'acl-special',
        nodeId: 'node-2',
        nodeName: 'node-2',
        kind: 'acl',
        name: 'ACL special',
        status: 'error',
        deliveryHistory: [
          {
            id: 'delivery-acl-2',
            command_id: 'cmd-2',
            policy_ref: 'acl-special',
            policy_domain: 'acl',
            command_status: 'failed'
          }
        ]
      },
      {
        policyId: 'policy-qos-2',
        policyRef: 'qos-special',
        nodeId: 'node-2',
        nodeName: 'node-2',
        kind: 'qos',
        name: 'QoS special',
        status: 'pending',
        deliveryHistory: []
      },
      {
        policyId: 'policy-route-2',
        policyRef: '10.2.0.0/24',
        nodeId: 'node-2',
        nodeName: 'node-2',
        kind: 'route',
        name: 'Route special',
        status: 'applied',
        deliveryHistory: []
      }
    ]),
    retryPolicySync: vi.fn(async () => ({}))
  }
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: routerPush
  }),
  useRoute: () => routeState
}))

vi.mock('@/composables/useTenantApi', () => ({
  useTenantApi: tenantApiMock
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

vi.mock('@/composables/useIpGroupApi', () => ({
  useIpGroupApi: ipGroupApiMock
}))

vi.mock('@/composables/usePolicyApi', () => ({
  usePolicyApi: policyApiMock
}))

vi.mock('@/composables/usePermission', () => ({
  usePermission: () => ({
    hasPermission: () => true,
    hasAnyPermission: () => true,
    hasAllPermissions: () => true,
    canAccessRoute: () => true
  })
}))

vi.mock('@/composables/useTenantChangeReload', () => ({
  useTenantChangeReload: vi.fn()
}))

vi.mock('element-plus', () => ({
  ElMessage: {
    success: vi.fn(),
    error: vi.fn(),
    warning: vi.fn()
  },
  ElMessageBox: {
    confirm: vi.fn(async () => true)
  }
}))

import Policies from '@/views/Policies.vue'
import ACLRules from '@/views/ACLRules.vue'
import BandwidthControl from '@/views/BandwidthControl.vue'
import Routing from '@/views/Routing.vue'

const elementStubs = {
  'el-row': { template: '<div><slot /></div>' },
  'el-col': { template: '<div><slot /></div>' },
  'el-card': { template: '<div><slot name="header" /><slot /></div>' },
  'el-input': { template: '<div><slot name="prefix" /><slot name="append" /></div>' },
  'el-input-number': { template: '<div></div>' },
  'el-select': { template: '<div><slot /></div>' },
  'el-option': { template: '<div></div>' },
  'el-divider': { template: '<div></div>' },
  'el-button': { template: '<button @click="$emit(\'click\')"><slot /></button>' },
  'el-icon': { template: '<i><slot /></i>' },
  'el-tag': { template: '<span><slot /></span>' },
  'el-alert': { template: '<div><slot /></div>' },
  'el-pagination': { template: '<div></div>' },
  'el-tooltip': { template: '<div><slot /></div>' },
  'el-drawer': { template: '<div><slot /></div>' },
  'el-dialog': { template: '<div><slot /><slot name="footer" /></div>' },
  'el-descriptions': { template: '<div><slot /></div>' },
  'el-descriptions-item': { template: '<div><slot /></div>' },
  'el-form': { template: '<form><slot /></form>' },
  'el-form-item': { template: '<div><slot /></div>' },
  'el-switch': { template: '<input type="checkbox" />' },
  'el-table': { template: '<div><slot /></div>' },
  'el-table-column': {
    template: '<div><slot :row="row" /></div>',
    data() {
      return {
        row: {
          id: 'row-1',
          node_id: 'node-2',
          nodeId: 'node-2',
          policyId: 'policy-acl-2',
          policyRef: 'acl-special',
          kind: 'acl',
          deliveryHistory: []
        }
      }
    }
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

describe('policy page context handoff', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    routerPush.mockReset()
    tenantApiMock.getTenantNodes.mockClear()
    aclApiMock.getACLRulesByNode.mockClear()
    aclApiMock.retryACLPolicySync.mockClear()
    qosApiMock.getQoSRulesByNode.mockClear()
    qosApiMock.retryQoSPolicySync.mockClear()
    routeApiMock.getRoutes.mockClear()
    ipGroupApiMock.listIPGroups.mockClear()
    policyApiMock.listPolicies.mockClear()
    routeState.params = {}
    routeState.query = {}
    routeState.fullPath = '/'
  })

  it('preserves policy context when opening the ACL page from Policy Center', async () => {
    routeState.query = {
      nodeId: 'node-2',
      policyRef: 'acl-special',
      kind: 'acl',
      commandId: 'cmd-2'
    }
    routeState.fullPath = '/policies?nodeId=node-2&policyRef=acl-special&kind=acl&commandId=cmd-2'

    const wrapper = mountWithStubs(Policies)
    await flushPromises()

    wrapper.vm.goToKind('acl')

    expect(routerPush).toHaveBeenCalledWith({
      name: 'ACLRules',
      query: {
        nodeId: 'node-2',
        policyRef: 'acl-special',
        commandId: 'cmd-2'
      }
    })
  })

  it('preserves policy context when opening the QoS page from Policy Center', async () => {
    routeState.query = {
      nodeId: 'node-2',
      policyRef: 'qos-special',
      kind: 'qos',
      commandId: 'cmd-qos'
    }
    routeState.fullPath = '/policies?nodeId=node-2&policyRef=qos-special&kind=qos&commandId=cmd-qos'

    const wrapper = mountWithStubs(Policies)
    await flushPromises()

    wrapper.vm.goToKind('qos')

    expect(routerPush).toHaveBeenCalledWith({
      name: 'BandwidthControl',
      query: {
        nodeId: 'node-2',
        policyRef: 'qos-special',
        commandId: 'cmd-qos'
      }
    })
  })

  it('preserves policy context when opening the routing page from Policy Center', async () => {
    routeState.query = {
      nodeId: 'node-2',
      policyRef: '10.2.0.0/24',
      kind: 'route',
      commandId: 'cmd-route'
    }
    routeState.fullPath = '/policies?nodeId=node-2&policyRef=10.2.0.0%2F24&kind=route&commandId=cmd-route'

    const wrapper = mountWithStubs(Policies)
    await flushPromises()

    wrapper.vm.goToKind('route')

    expect(routerPush).toHaveBeenCalledWith({
      name: 'Routing',
      query: {
        nodeId: 'node-2',
        policyRef: '10.2.0.0/24',
        commandId: 'cmd-route'
      }
    })
  })

  it('loads the ACL page against the node and policy from route context', async () => {
    routeState.query = {
      nodeId: 'node-2',
      policyRef: 'acl-special',
      commandId: 'cmd-2'
    }
    routeState.fullPath = '/acl?nodeId=node-2&policyRef=acl-special&commandId=cmd-2'

    const wrapper = mountWithStubs(ACLRules)
    await flushPromises()

    expect(wrapper.vm.filters.node_id).toBe('node-2')
    expect(wrapper.vm.filters.name).toBe('acl-special')
    expect(aclApiMock.getACLRulesByNode).toHaveBeenCalledWith('node-2', expect.objectContaining({
      name: 'acl-special'
    }))
  })

  it('routes ACL retry to the node command trace', async () => {
    routeState.query = {
      nodeId: 'node-2',
      policyRef: 'acl-special',
      commandId: 'cmd-2'
    }
    routeState.fullPath = '/acl?nodeId=node-2&policyRef=acl-special&commandId=cmd-2'

    const wrapper = mountWithStubs(ACLRules)
    await flushPromises()
    routerPush.mockReset()

    await wrapper.vm.handleRetry({
      id: 'acl-special',
      node_id: 'node-2',
      name: 'ACL special',
      policy_status: 'error'
    })

    expect(aclApiMock.retryACLPolicySync).toHaveBeenCalledWith(expect.objectContaining({
      id: 'acl-special',
      node_id: 'node-2'
    }))
    expect(routerPush).toHaveBeenCalledWith({
      name: 'NodeMonitorDetail',
      params: { nodeId: 'node-2' },
      query: {
        commandId: 'cmd-acl-retry',
        focus: 'commands',
        policyRef: 'acl-special',
        policyDomain: 'acl'
      }
    })
  })

  it('loads the QoS page against the node from route context', async () => {
    routeState.query = {
      nodeId: 'node-2',
      policyRef: 'qos-special',
      commandId: 'cmd-qos'
    }
    routeState.fullPath = '/qos?nodeId=node-2&policyRef=qos-special&commandId=cmd-qos'

    const wrapper = mountWithStubs(BandwidthControl)
    await flushPromises()

    expect(wrapper.vm.selectedNodeId).toBe('node-2')
    expect(qosApiMock.getQoSRulesByNode).toHaveBeenCalledWith('node-2')
  })

  it('routes QoS retry to the node command trace', async () => {
    routeState.query = {
      nodeId: 'node-2',
      policyRef: 'qos-special',
      commandId: 'cmd-qos'
    }
    routeState.fullPath = '/qos?nodeId=node-2&policyRef=qos-special&commandId=cmd-qos'

    const wrapper = mountWithStubs(BandwidthControl)
    await flushPromises()
    routerPush.mockReset()

    await wrapper.vm.handleRetry({
      id: 'qos-special',
      node_id: 'node-2',
      description: 'QoS special',
      policyStatus: 'error'
    })

    expect(qosApiMock.retryQoSPolicySync).toHaveBeenCalledWith('node-2', expect.objectContaining({
      id: 'qos-special',
      node_id: 'node-2'
    }))
    expect(routerPush).toHaveBeenCalledWith({
      name: 'NodeMonitorDetail',
      params: { nodeId: 'node-2' },
      query: {
        commandId: 'cmd-qos-retry',
        focus: 'commands',
        policyRef: 'qos-special',
        policyDomain: 'qos'
      }
    })
  })

  it('loads routing page filters from route context', async () => {
    routeState.query = {
      nodeId: 'node-2',
      policyRef: '10.2.0.0/24',
      commandId: 'cmd-route'
    }
    routeState.fullPath = '/routing?nodeId=node-2&policyRef=10.2.0.0%2F24&commandId=cmd-route'

    const wrapper = mountWithStubs(Routing)
    await flushPromises()

    expect(wrapper.vm.currentRoute.nodeId).toBe('node-2')
    expect(wrapper.vm.searchQuery).toBe('10.2.0.0/24')
    expect(wrapper.vm.filteredRoutes).toHaveLength(1)
    expect(wrapper.vm.filteredRoutes[0].nodeId).toBe('node-2')
  })
})
