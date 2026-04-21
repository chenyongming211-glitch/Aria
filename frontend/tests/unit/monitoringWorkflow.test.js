import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

const {
  routerPush,
  routeState,
  monitorApiMock,
  policyApiMock,
  localStorageMock
} = vi.hoisted(() => ({
  routerPush: vi.fn(),
  routeState: {
    params: { nodeId: 'node-1' },
    query: {},
    fullPath: '/monitoring/nodes/node-1'
  },
  monitorApiMock: {
    getStats: vi.fn(async () => ({
      total_nodes: 2,
      online_nodes: 1,
      active_alerts_count: 1,
      failed_commands_count: 1
    })),
    getEvents: vi.fn(async () => ({
      items: [
        {
          id: 'event-1',
          source: 'audit',
          event_type: 'policy_failed',
          node_id: 'node-1',
          title: 'Policy failed',
          detail: {
            policy_ref: 'acl-1',
            policy_domain: 'acl',
            command_id: 'cmd-1'
          },
          created_at: '2026-04-21T10:00:00Z'
        }
      ],
      total: 1
    })),
    getAlerts: vi.fn(async () => ({
      items: [
        {
          id: 'alert-1',
          node_id: 'node-1',
          alert_type: 'policy_failed',
          severity: 'warning',
          title: 'Policy apply failed',
          message: 'delivery failed',
          context: {
            policy_ref: 'acl-1',
            policy_domain: 'acl',
            command_id: 'cmd-1'
          }
        }
      ]
    })),
    resolveAlert: vi.fn(async () => ({})),
    getNodeDetail: vi.fn(async () => ({
      hostname: 'node-1',
      availability_status: 'online',
      state_convergence: 'diverged',
      desired_state_version: 'desired-1',
      applied_state_version: 'applied-1',
      recent_commands: [
        { id: 'cmd-1', command: 'sync', status: 'failed', message: 'sync failed', created_at: '2026-04-21T10:00:00Z' }
      ],
      recent_policy_deliveries: [
        { id: 'delivery-1', policy_ref: 'acl-1', policy_domain: 'acl', command_id: 'cmd-1', command_status: 'failed', created_at: '2026-04-21T10:00:00Z' }
      ],
      active_alerts: [
        { id: 'alert-1', alert_type: 'policy_failed', severity: 'warning', title: 'Policy apply failed', created_at: '2026-04-21T10:00:00Z' }
      ]
    }))
  },
  policyApiMock: {
    listPolicies: vi.fn(async () => ([
      {
        policyId: 'policy-1',
        policyRef: 'acl-1',
        nodeId: 'node-1',
        nodeName: 'node-1',
        kind: 'acl',
        name: 'ACL 1',
        status: 'error',
        stateConvergence: 'diverged',
        observedState: 'error',
        observedMessage: 'delivery failed',
        deliveryHistory: [
          {
            id: 'delivery-1',
            command_id: 'cmd-1',
            policy_ref: 'acl-1',
            policy_domain: 'acl',
            command_status: 'failed',
            updated_at: '2026-04-21T10:00:00Z'
          }
        ]
      },
      {
        policyId: 'policy-2',
        policyRef: 'route-1',
        nodeId: 'node-2',
        nodeName: 'node-2',
        kind: 'route',
        name: 'Route 1',
        status: 'applied',
        stateConvergence: 'converged',
        observedState: 'healthy',
        deliveryHistory: []
      }
    ]))
  },
  localStorageMock: {
    getItem: vi.fn(() => null),
    setItem: vi.fn(),
    removeItem: vi.fn(),
    clear: vi.fn()
  }
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: routerPush
  }),
  useRoute: () => routeState
}))

vi.mock('@/composables/useMonitorApi', () => ({
  useMonitorApi: monitorApiMock
}))

vi.mock('@/composables/usePolicyApi', () => ({
  usePolicyApi: policyApiMock
}))

vi.mock('element-plus', () => ({
  ElMessage: {
    success: vi.fn(),
    error: vi.fn(),
    warning: vi.fn()
  }
}))

import Monitoring from '@/views/Monitoring.vue'
import NodeMonitorDetail from '@/views/NodeMonitorDetail.vue'
import Policies from '@/views/Policies.vue'

const elementStubs = {
  'el-row': { template: '<div><slot /></div>' },
  'el-col': { template: '<div><slot /></div>' },
  'el-card': { template: '<div><slot name="header" /><slot /></div>' },
  'el-input': { template: '<div><slot name="prefix" /><slot name="append" /></div>' },
  'el-select': { template: '<div><slot /></div>' },
  'el-option': { template: '<div></div>' },
  'el-button': { template: '<button @click="$emit(\'click\')"><slot /></button>' },
  'el-icon': { template: '<i><slot /></i>' },
  'el-tag': { template: '<span><slot /></span>' },
  'el-empty': { template: '<div><slot /></div>' },
  'el-alert': { template: '<div><slot /></div>' },
  'el-pagination': { template: '<div></div>' },
  'el-tooltip': { template: '<div><slot /></div>' },
  'el-drawer': { template: '<div><slot /></div>' },
  'el-descriptions': { template: '<div><slot /></div>' },
  'el-descriptions-item': { template: '<div><slot /></div>' },
  'el-table': { template: '<div><slot /></div>' },
  'el-table-column': {
    template: '<div><slot :row="row" /></div>',
    data() {
      return {
        row: {
          id: 'row-1',
          node_id: 'node-1',
          alert_type: 'policy_failed',
          severity: 'warning',
          title: 'Title',
          message: 'Message',
          context: { policy_ref: 'acl-1', policy_domain: 'acl', command_id: 'cmd-1' },
          detail: { policy_ref: 'acl-1', policy_domain: 'acl', command_id: 'cmd-1' },
          command: 'sync',
          status: 'failed',
          created_at: '2026-04-21T10:00:00Z',
          command_status: 'failed',
          policy_ref: 'acl-1'
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

describe('monitoring workflow routing', () => {
  beforeEach(() => {
    routerPush.mockReset()
    monitorApiMock.getEvents.mockClear()
    monitorApiMock.getAlerts.mockClear()
    policyApiMock.listPolicies.mockClear()
    routeState.params = { nodeId: 'node-1' }
    routeState.query = {}
    routeState.fullPath = '/monitoring/nodes/node-1'
    globalThis.localStorage = localStorageMock
  })

  it('routes from an alert to node detail with command and policy context', async () => {
    const wrapper = mountWithStubs(Monitoring)
    await flushPromises()

    wrapper.vm.goToNodeFromAlert({
      id: 'alert-1',
      node_id: 'node-1',
      alert_type: 'policy_failed',
      context: {
        command_id: 'cmd-1',
        policy_ref: 'acl-1',
        policy_domain: 'acl'
      }
    })

    expect(routerPush).toHaveBeenCalledWith({
      name: 'NodeMonitorDetail',
      params: { nodeId: 'node-1' },
      query: {
        focus: 'commands',
        alertId: 'alert-1',
        eventType: 'policy_failed',
        commandId: 'cmd-1',
        policyRef: 'acl-1',
        policyDomain: 'acl'
      }
    })
  })

  it('routes from an event to policy center with node and policy filters', async () => {
    const wrapper = mountWithStubs(Monitoring)
    await flushPromises()

    wrapper.vm.goToPolicyFromContext('node-1', {
      policy_ref: 'acl-1',
      policy_domain: 'acl'
    })

    expect(routerPush).toHaveBeenCalledWith({
      name: 'Policies',
      query: {
        nodeId: 'node-1',
        policyRef: 'acl-1',
        kind: 'acl'
      }
    })
  })

  it('turns failed commands stat card into an event-feed shortcut', async () => {
    const wrapper = mountWithStubs(Monitoring)
    await flushPromises()

    await wrapper.vm.handleStatCardClick('failed')

    expect(wrapper.vm.filterEventType).toBe('command_failed')
    expect(monitorApiMock.getEvents).toHaveBeenLastCalledWith({
      limit: 50,
      offset: 0,
      event_type: 'command_failed'
    })
  })

  it('routes from nodes stat card to the nodes page', async () => {
    const wrapper = mountWithStubs(Monitoring)
    await flushPromises()

    await wrapper.vm.handleStatCardClick('nodes')

    expect(routerPush).toHaveBeenCalledWith({ name: 'Nodes' })
  })
})

describe('node monitor detail context handling', () => {
  beforeEach(() => {
    routerPush.mockReset()
    routeState.params = { nodeId: 'node-1' }
    routeState.query = {
      focus: 'commands',
      commandId: 'cmd-1',
      policyRef: 'acl-1',
      policyDomain: 'acl',
      alertId: 'alert-1',
      eventType: 'policy_failed'
    }
    routeState.fullPath = '/monitoring/nodes/node-1?focus=commands&commandId=cmd-1&policyRef=acl-1'
  })

  it('derives context summary and highlights the targeted command row', async () => {
    const wrapper = mountWithStubs(NodeMonitorDetail)
    await flushPromises()

    expect(wrapper.vm.contextDescription).toContain('Event: policy_failed')
    expect(wrapper.vm.contextDescription).toContain('Policy: acl-1')
    expect(wrapper.vm.contextDescription).toContain('Command: cmd-1')
    expect(wrapper.vm.commandRowClassName({ row: { id: 'cmd-1' } })).toBe('context-match-row')
    expect(wrapper.vm.policyRowClassName({ row: { policy_ref: 'acl-1', command_id: 'other' } })).toBe('context-match-row')
    expect(wrapper.vm.alertRowClassName({ row: { id: 'alert-1' } })).toBe('context-match-row')
  })

  it('routes to policy center with current node and policy filters', async () => {
    const wrapper = mountWithStubs(NodeMonitorDetail)
    await flushPromises()

    wrapper.vm.openPolicyCenter()

    expect(routerPush).toHaveBeenCalledWith({
      name: 'Policies',
      query: {
        nodeId: 'node-1',
        policyRef: 'acl-1',
        kind: 'acl'
      }
    })
  })
})

describe('policy center context handling', () => {
  beforeEach(() => {
    routerPush.mockReset()
    routeState.params = { nodeId: 'node-1' }
    routeState.query = {
      nodeId: 'node-1',
      policyRef: 'acl-1',
      kind: 'acl',
      commandId: 'cmd-1'
    }
    routeState.fullPath = '/policy-center?nodeId=node-1&policyRef=acl-1&kind=acl&commandId=cmd-1'
  })

  it('syncs route filters and auto-focuses the matching policy drawer', async () => {
    const wrapper = mountWithStubs(Policies)
    await flushPromises()

    expect(wrapper.vm.filters.nodeId).toBe('node-1')
    expect(wrapper.vm.filters.keyword).toBe('acl-1')
    expect(wrapper.vm.filters.kind).toBe('acl')
    expect(wrapper.vm.selectedPolicy.policyId).toBe('policy-1')
    expect(wrapper.vm.detailVisible).toBe(true)
    expect(wrapper.vm.policyRowClassName({ row: { policyId: 'policy-1' } })).toBe('context-match-row')
    expect(wrapper.vm.isDeliveryMatch({ command_id: 'cmd-1', policy_ref: 'acl-1' })).toBe(true)
  })

  it('routes back to node detail while preserving policy delivery context', async () => {
    const wrapper = mountWithStubs(Policies)
    await flushPromises()

    wrapper.vm.openNodeDetail({
      nodeId: 'node-1',
      policyRef: 'acl-1',
      kind: 'acl'
    })

    expect(routerPush).toHaveBeenCalledWith({
      name: 'NodeMonitorDetail',
      params: { nodeId: 'node-1' },
      query: {
        commandId: 'cmd-1',
        focus: 'policies',
        policyRef: 'acl-1',
        policyDomain: 'acl'
      }
    })
  })
})
