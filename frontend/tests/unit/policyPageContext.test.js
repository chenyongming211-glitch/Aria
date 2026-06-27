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
    createACLRule: vi.fn(async () => ({
      id: 'acl-created',
      node_id: 'node-2',
      policy_ref: 'acl-created',
      policy_status: 'pending',
      last_delivery_command_id: 'cmd-acl-create'
    })),
    updateACLRule: vi.fn(async () => ({
      id: 'acl-special',
      node_id: 'node-2',
      policy_ref: 'acl-special',
      policy_status: 'pending',
      last_delivery_command_id: 'cmd-acl-update'
    })),
    deleteACLRule: vi.fn(async () => ({
      id: 'acl-special',
      node_id: 'node-2',
      policy_ref: 'acl-special',
      policy_status: 'pending',
      last_delivery_command_id: 'cmd-acl-delete'
    })),
    retryACLPolicySync: vi.fn(async () => ({
      policy_ref: 'acl-special',
      policy_status: 'pending',
      last_delivery_command_id: 'cmd-acl-retry'
    }))
  },
  qosApiMock: {
    getQoSRulesByNode: vi.fn(async () => []),
    createQoSRule: vi.fn(async () => ({
      id: 'qos-created',
      node_id: 'node-2',
      policy_ref: 'qos-created',
      policyStatus: 'pending',
      last_delivery_command_id: 'cmd-qos-create'
    })),
    updateQoSRule: vi.fn(async () => ({
      id: 'qos-special',
      node_id: 'node-2',
      policy_ref: 'qos-special',
      policyStatus: 'pending',
      last_delivery_command_id: 'cmd-qos-update'
    })),
    deleteQoSRule: vi.fn(async () => ({
      id: 'qos-special',
      node_id: 'node-2',
      policy_ref: 'qos-special',
      policyStatus: 'pending',
      last_delivery_command_id: 'cmd-qos-delete'
    })),
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
    ]),
    addRoute: vi.fn(async () => ({
      id: '10.3.0.0/24',
      cidr: '10.3.0.0/24',
      node_id: 'node-2',
      policy_ref: '10.3.0.0/24',
      policy_status: 'pending',
      last_delivery_command_id: 'cmd-route-create'
    })),
    updateRoute: vi.fn(async () => ({
      id: '10.2.0.0/24',
      cidr: '10.2.0.0/24',
      node_id: 'node-2',
      policy_ref: '10.2.0.0/24',
      policy_status: 'pending',
      last_delivery_command_id: 'cmd-route-update'
    })),
    deleteRoute: vi.fn(async () => ({
      id: '10.2.0.0/24',
      cidr: '10.2.0.0/24',
      node_id: 'node-2',
      policy_ref: '10.2.0.0/24',
      policy_status: 'pending',
      last_delivery_command_id: 'cmd-route-delete'
    }))
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
  'el-form': {
    template: '<form><slot /></form>',
    methods: {
      validate: () => Promise.resolve(true),
      resetFields: () => {}
    }
  },
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
    aclApiMock.createACLRule.mockClear()
    aclApiMock.updateACLRule.mockClear()
    aclApiMock.deleteACLRule.mockClear()
    aclApiMock.retryACLPolicySync.mockClear()
    qosApiMock.getQoSRulesByNode.mockClear()
    qosApiMock.createQoSRule.mockClear()
    qosApiMock.updateQoSRule.mockClear()
    qosApiMock.deleteQoSRule.mockClear()
    qosApiMock.retryQoSPolicySync.mockClear()
    routeApiMock.getRoutes.mockClear()
    routeApiMock.addRoute.mockClear()
    routeApiMock.updateRoute.mockClear()
    routeApiMock.deleteRoute.mockClear()
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

  it('uses shared UI foundation components for the Policy Center operations shell', async () => {
    const wrapper = mountWithStubs(Policies)
    await flushPromises()

    expect(wrapper.find('.ui-page-header').exists()).toBe(true)
    expect(wrapper.find('.ui-metric-strip').exists()).toBe(true)
    expect(wrapper.find('.ui-filter-bar').exists()).toBe(true)
    expect(wrapper.find('.ui-data-panel').exists()).toBe(true)
    expect(wrapper.find('.page-hero').exists()).toBe(false)
    expect(wrapper.find('.policy-card').exists()).toBe(false)
  })

  it('filters policy inventory from shared metric shortcuts', async () => {
    const wrapper = mountWithStubs(Policies)
    await flushPromises()

    const metricItems = wrapper.findAll('.ui-metric-strip__item')
    const failedMetric = metricItems.find(item => item.text().includes('Failed'))
    expect(failedMetric).toBeTruthy()

    await failedMetric.trigger('click')

    expect(wrapper.vm.filters.status).toBe('error')
    expect(wrapper.vm.filteredPolicies).toHaveLength(1)
    expect(wrapper.vm.filteredPolicies[0].policyRef).toBe('acl-special')

    const pendingMetric = wrapper.findAll('.ui-metric-strip__item').find(item => item.text().includes('Pending'))
    expect(pendingMetric).toBeTruthy()

    await pendingMetric.trigger('click')

    expect(wrapper.vm.filters.status).toBe('pending')
    expect(wrapper.vm.filteredPolicies).toHaveLength(1)
    expect(wrapper.vm.filteredPolicies[0].policyRef).toBe('qos-special')
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

  it('uses shared UI foundation components for the ACL rules operations shell', async () => {
    const wrapper = mountWithStubs(ACLRules)
    await flushPromises()

    expect(wrapper.find('.ui-page-header').exists()).toBe(true)
    expect(wrapper.find('.ui-filter-bar').exists()).toBe(true)
    expect(wrapper.find('.ui-data-panel').exists()).toBe(true)
    expect(wrapper.find('.page-header').exists()).toBe(false)
    expect(wrapper.find('.filter-section').exists()).toBe(false)
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

  it('routes ACL create to the node command trace', async () => {
    routeState.query = {
      nodeId: 'node-2',
      policyRef: 'acl-special'
    }
    routeState.fullPath = '/acl?nodeId=node-2&policyRef=acl-special'

    const wrapper = mountWithStubs(ACLRules)
    await flushPromises()
    routerPush.mockReset()

    Object.assign(wrapper.vm.form, {
      node_id: 'node-2',
      name: 'ACL create',
      action: 'allow',
      protocol: 1,
      dst_port: 0,
      direction: 'egress',
      priority: 100,
      enabled: true
    })

    await wrapper.vm.handleSubmit()
    await flushPromises()

    expect(aclApiMock.createACLRule).toHaveBeenCalledWith(expect.objectContaining({
      node_id: 'node-2',
      name: 'ACL create'
    }))
    expect(routerPush).toHaveBeenCalledWith({
      name: 'NodeMonitorDetail',
      params: { nodeId: 'node-2' },
      query: {
        commandId: 'cmd-acl-create',
        focus: 'commands',
        policyRef: 'acl-created',
        policyDomain: 'acl'
      }
    })
  })

  it('routes ACL delete to the node command trace', async () => {
    routeState.query = {
      nodeId: 'node-2',
      policyRef: 'acl-special'
    }
    routeState.fullPath = '/acl?nodeId=node-2&policyRef=acl-special'

    const wrapper = mountWithStubs(ACLRules)
    await flushPromises()
    routerPush.mockReset()

    await wrapper.vm.handleDelete({
      id: 'acl-special',
      node_id: 'node-2',
      name: 'ACL special'
    })
    await flushPromises()

    expect(aclApiMock.deleteACLRule).toHaveBeenCalledWith('acl-special', 'node-2')
    expect(routerPush).toHaveBeenCalledWith({
      name: 'NodeMonitorDetail',
      params: { nodeId: 'node-2' },
      query: {
        commandId: 'cmd-acl-delete',
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

  it('uses shared UI foundation components for the QoS operations shell', async () => {
    const wrapper = mountWithStubs(BandwidthControl)
    await flushPromises()

    expect(wrapper.find('.ui-page-header').exists()).toBe(true)
    expect(wrapper.find('.ui-filter-bar').exists()).toBe(true)
    expect(wrapper.find('.ui-data-panel').exists()).toBe(true)
    expect(wrapper.find('.card-header').exists()).toBe(false)
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

  it('routes QoS create to the node command trace', async () => {
    routeState.query = {
      nodeId: 'node-2',
      policyRef: 'qos-special'
    }
    routeState.fullPath = '/qos?nodeId=node-2&policyRef=qos-special'

    const wrapper = mountWithStubs(BandwidthControl)
    await flushPromises()
    routerPush.mockReset()

    Object.assign(wrapper.vm.form, {
      description: 'QoS create',
      bandwidth_mbps: 2,
      group_id: 'group-1',
      direction: 'egress',
      mode: 'auto',
      priority: 100,
      enabled: true
    })

    await wrapper.vm.handleSave()
    await flushPromises()

    expect(qosApiMock.createQoSRule).toHaveBeenCalledWith('node-2', expect.objectContaining({
      description: 'QoS create',
      bandwidth_mbps: 2
    }))
    expect(routerPush).toHaveBeenCalledWith({
      name: 'NodeMonitorDetail',
      params: { nodeId: 'node-2' },
      query: {
        commandId: 'cmd-qos-create',
        focus: 'commands',
        policyRef: 'qos-created',
        policyDomain: 'qos'
      }
    })
  })

  it('routes QoS delete to the node command trace', async () => {
    routeState.query = {
      nodeId: 'node-2',
      policyRef: 'qos-special'
    }
    routeState.fullPath = '/qos?nodeId=node-2&policyRef=qos-special'

    const wrapper = mountWithStubs(BandwidthControl)
    await flushPromises()
    routerPush.mockReset()

    await wrapper.vm.handleDelete({
      id: 'qos-special',
      node_id: 'node-2',
      description: 'QoS special'
    })
    await flushPromises()

    expect(qosApiMock.deleteQoSRule).toHaveBeenCalledWith('node-2', 'qos-special')
    expect(routerPush).toHaveBeenCalledWith({
      name: 'NodeMonitorDetail',
      params: { nodeId: 'node-2' },
      query: {
        commandId: 'cmd-qos-delete',
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

  it('routes Route create to the node command trace', async () => {
    routeState.query = {
      nodeId: 'node-2',
      policyRef: '10.2.0.0/24'
    }
    routeState.fullPath = '/routing?nodeId=node-2&policyRef=10.2.0.0%2F24'

    const wrapper = mountWithStubs(Routing)
    await flushPromises()
    routerPush.mockReset()

    wrapper.vm.dialogMode = 'add'
    Object.assign(wrapper.vm.currentRoute, {
      nodeId: 'node-2',
      cidr: '10.3.0.0/24',
      originalCidr: ''
    })

    await wrapper.vm.confirmRouteAction()
    await flushPromises()

    expect(routeApiMock.addRoute).toHaveBeenCalledWith('node-2', '10.3.0.0/24')
    expect(routerPush).toHaveBeenCalledWith({
      name: 'NodeMonitorDetail',
      params: { nodeId: 'node-2' },
      query: {
        commandId: 'cmd-route-create',
        focus: 'commands',
        policyRef: '10.3.0.0/24',
        policyDomain: 'route'
      }
    })
  })

  it('routes Route update to the node command trace', async () => {
    routeState.query = {
      nodeId: 'node-2',
      policyRef: '10.2.0.0/24'
    }
    routeState.fullPath = '/routing?nodeId=node-2&policyRef=10.2.0.0%2F24'

    const wrapper = mountWithStubs(Routing)
    await flushPromises()
    routerPush.mockReset()

    wrapper.vm.dialogMode = 'edit'
    Object.assign(wrapper.vm.currentRoute, {
      nodeId: 'node-2',
      cidr: '10.2.0.0/24',
      originalCidr: '10.2.0.0/25'
    })

    await wrapper.vm.confirmRouteAction()
    await flushPromises()

    expect(routeApiMock.updateRoute).toHaveBeenCalledWith('node-2', '10.2.0.0/25', '10.2.0.0/24')
    expect(routerPush).toHaveBeenCalledWith({
      name: 'NodeMonitorDetail',
      params: { nodeId: 'node-2' },
      query: {
        commandId: 'cmd-route-update',
        focus: 'commands',
        policyRef: '10.2.0.0/24',
        policyDomain: 'route'
      }
    })
  })

  it('routes Route delete to the node command trace', async () => {
    routeState.query = {
      nodeId: 'node-2',
      policyRef: '10.2.0.0/24'
    }
    routeState.fullPath = '/routing?nodeId=node-2&policyRef=10.2.0.0%2F24'

    const wrapper = mountWithStubs(Routing)
    await flushPromises()
    routerPush.mockReset()

    wrapper.vm.currentDeleteRoute = {
      id: '10.2.0.0/24',
      nodeId: 'node-2',
      nodeName: 'node-2',
      cidr: '10.2.0.0/24'
    }

    await wrapper.vm.confirmDeleteRoute()
    await flushPromises()

    expect(routeApiMock.deleteRoute).toHaveBeenCalledWith('node-2', '10.2.0.0/24')
    expect(routerPush).toHaveBeenCalledWith({
      name: 'NodeMonitorDetail',
      params: { nodeId: 'node-2' },
      query: {
        commandId: 'cmd-route-delete',
        focus: 'commands',
        policyRef: '10.2.0.0/24',
        policyDomain: 'route'
      }
    })
  })
})
