import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

const {
  routerPush,
  routeState,
  monitorApiMock,
  agentApiMock,
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
          id: 'alert-cert-1',
          node_id: 'node-1',
          alert_type: 'certificate_expiring',
          severity: 'warning',
          title: 'Certificate expiring',
          message: 'node certificate expires soon',
          context: {
            not_after: '2026-04-24T10:00:00Z'
          }
        },
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
      public_ip: '1.1.1.1',
      assigned_ip: '10.0.0.10',
      endpoint: '1.1.1.1:51820',
      region: 'sh',
      availability_status: 'online',
      state_convergence: 'diverged',
      desired_state_version: 'desired-1',
      applied_state_version: 'applied-1',
      certificate: {
        status: 'issued',
        serial_number: 'serial-1',
        issued_at: '2026-04-21T10:00:00Z',
        not_after: '2026-04-24T10:00:00Z'
      },
      certificate_activity: {
        last_renewed_at: '2026-04-21T08:00:00Z',
        last_renewed_serial_number: 'serial-1',
        last_renew_failed_at: '2026-04-21T09:00:00Z',
        last_renew_failure: 'runtime token expired'
      },
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
  agentApiMock: {
    sendAgentCommand: vi.fn(async () => ({
      command_id: 'cmd-new',
      command: 'sync',
      status: 'pending',
      message: 'Command queued for delivery',
      created_at: '2026-04-21T10:01:00Z'
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
    ])),
    retryPolicySync: vi.fn(async () => ({
      policyId: 'policy-1',
      policyRef: 'acl-1',
      nodeId: 'node-1',
      kind: 'acl',
      name: 'ACL 1',
      status: 'pending',
      lastDeliveryCommandId: 'cmd-retry'
    }))
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

vi.mock('@/composables/useAgentProxyApi', () => ({
  useAgentProxyApi: agentApiMock
}))

vi.mock('@/composables/usePermission', () => ({
  usePermission: () => ({
    hasPermission: () => true,
    hasAnyPermission: () => true,
    hasAllPermissions: () => true,
    canAccessRoute: () => true
  })
}))

vi.mock('/src/composables/usePermission', () => ({
  usePermission: () => ({
    hasPermission: () => true,
    hasAnyPermission: () => true,
    hasAllPermissions: () => true,
    canAccessRoute: () => true
  })
}))

vi.mock('/src/composables/usePermission.js', () => ({
  usePermission: () => ({
    hasPermission: () => true,
    hasAnyPermission: () => true,
    hasAllPermissions: () => true,
    canAccessRoute: () => true
  })
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

import Monitoring from '@/views/Monitoring.vue'
import NodeMonitorDetail from '@/views/NodeMonitorDetail.vue'
import Policies from '@/views/Policies.vue'
import AIAssistant from '@/views/AIAssistant.vue'

const elementStubs = {
  'el-row': { template: '<div><slot /></div>' },
  'el-col': { template: '<div><slot /></div>' },
  'el-card': { template: '<div><slot name="header" /><slot /></div>' },
  'el-input': { template: '<div><slot name="prefix" /><slot name="append" /></div>' },
  'el-avatar': { template: '<div><slot /></div>' },
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
  'el-dialog': { template: '<div><slot /><slot name="footer" /></div>' },
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
    setActivePinia(createPinia())
    routerPush.mockReset()
    monitorApiMock.getEvents.mockClear()
    monitorApiMock.getAlerts.mockClear()
    monitorApiMock.getStats.mockClear()
    monitorApiMock.resolveAlert.mockClear()
    agentApiMock.sendAgentCommand.mockClear()
    policyApiMock.listPolicies.mockClear()
    policyApiMock.retryPolicySync.mockClear()
    routeState.params = { nodeId: 'node-1' }
    routeState.query = {}
    routeState.fullPath = '/monitoring/nodes/node-1'
    globalThis.localStorage = localStorageMock
  })

  it('uses shared UI foundation components for the monitoring operations shell', async () => {
    const wrapper = mountWithStubs(Monitoring)
    await flushPromises()

    expect(wrapper.find('.ui-page-header').exists()).toBe(true)
    expect(wrapper.find('.ui-filter-bar').exists()).toBe(true)
    expect(wrapper.find('.ui-metric-strip').exists()).toBe(true)
    expect(wrapper.findAll('.ui-data-panel')).toHaveLength(2)
    expect(wrapper.find('.ui-status-badge').exists()).toBe(true)
    expect(wrapper.text()).toContain('Run Sync')
    expect(wrapper.text()).toContain('Health Check')
    expect(wrapper.text()).toContain('Resolve')
  })

  it('keeps failed-command metric shortcuts wired through the shared metric strip', async () => {
    const wrapper = mountWithStubs(Monitoring)
    await flushPromises()

    const failedMetric = wrapper.findAll('.ui-metric-strip__item--clickable')
      .find((item) => item.text().includes('Failed Cmds'))
    expect(failedMetric).toBeTruthy()

    await failedMetric.trigger('click')

    expect(wrapper.vm.filterEventType).toBe('command_failed')
    expect(monitorApiMock.getEvents).toHaveBeenLastCalledWith({
      limit: 50,
      offset: 0,
      event_type: 'command_failed'
    })
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

  it('turns certificate alerts stat card into certificate-focused monitoring shortcuts', async () => {
    const wrapper = mountWithStubs(Monitoring)
    await flushPromises()

    await wrapper.vm.handleStatCardClick('certificates')

    expect(wrapper.vm.filterEventType).toBe('certificate_expired')
    expect(wrapper.vm.alertsFilterMode).toBe('certificate')
    expect(wrapper.vm.filteredAlerts).toHaveLength(1)
    expect(wrapper.vm.filteredAlerts[0].alert_type).toBe('certificate_expiring')
    expect(monitorApiMock.getEvents).toHaveBeenLastCalledWith({
      limit: 50,
      offset: 0,
      event_type: 'certificate_expired'
    })
  })

  it('routes certificate renewal events to node detail with certificate focus', async () => {
    const wrapper = mountWithStubs(Monitoring)
    await flushPromises()

    wrapper.vm.goToNodeFromEvent({
      id: 'event-cert-1',
      node_id: 'node-1',
      event_type: 'certificate_renewed',
      detail: {
        renewed_from: 'cert-old-1',
        not_after: '2026-04-24T10:00:00Z'
      }
    })

    expect(routerPush).toHaveBeenCalledWith({
      name: 'NodeMonitorDetail',
      params: { nodeId: 'node-1' },
      query: {
        focus: 'certificate',
        eventId: 'event-cert-1',
        eventType: 'certificate_renewed'
      }
    })
  })

  it('queues a sync command from a sync_failed alert with alert context', async () => {
    const wrapper = mountWithStubs(Monitoring)
    await flushPromises()

    await wrapper.vm.handleAlertCommand({
      id: 'alert-sync-1',
      node_id: 'node-1',
      alert_type: 'sync_failed',
      context: {
        command_id: 'cmd-old',
        policy_ref: 'acl-1',
        policy_domain: 'acl'
      }
    }, 'sync')

    expect(agentApiMock.sendAgentCommand).toHaveBeenCalledWith('node-1', {
      command: 'sync',
      params: {
        source: 'monitoring',
        alert_id: 'alert-sync-1',
        event_type: 'sync_failed',
        command_id: 'cmd-old',
        policy_ref: 'acl-1',
        policy_domain: 'acl'
      },
      timeout: 30
    })
    expect(monitorApiMock.getStats).toHaveBeenCalled()
    expect(monitorApiMock.getEvents).toHaveBeenCalled()
    expect(monitorApiMock.getAlerts).toHaveBeenCalled()
    expect(routerPush).toHaveBeenCalledWith({
      name: 'NodeMonitorDetail',
      params: { nodeId: 'node-1' },
      query: {
        focus: 'commands',
        alertId: 'alert-sync-1',
        eventType: 'sync_failed',
        commandId: 'cmd-new',
        policyRef: 'acl-1',
        policyDomain: 'acl'
      }
    })
  })

  it('resolves an alert with monitoring operation context', async () => {
    const wrapper = mountWithStubs(Monitoring)
    await flushPromises()

    await wrapper.vm.handleResolve({
      id: 'alert-sync-1',
      alert_type: 'sync_failed',
      context: {
        command_id: 'cmd-old',
        policy_ref: 'acl-1',
        policy_domain: 'acl'
      }
    })

    expect(monitorApiMock.resolveAlert).toHaveBeenCalledWith('alert-sync-1', {
      source: 'monitoring',
      reason: 'Resolved from Monitoring',
      command_id: 'cmd-old'
    })
    expect(monitorApiMock.getStats).toHaveBeenCalled()
    expect(monitorApiMock.getEvents).toHaveBeenCalled()
    expect(monitorApiMock.getAlerts).toHaveBeenCalled()
  })

  it('routes actionable alerts to AI with full diagnostic context', async () => {
    const wrapper = mountWithStubs(Monitoring)
    await flushPromises()

    wrapper.vm.askAIForAlert({
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
      name: 'AiAssistant',
      query: {
        source: 'monitoring',
        nodeId: 'node-1',
        alertId: 'alert-1',
        eventType: 'policy_failed',
        commandId: 'cmd-1',
        policyRef: 'acl-1',
        policyDomain: 'acl'
      }
    })
  })
})

describe('node monitor detail context handling', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    routerPush.mockReset()
    monitorApiMock.resolveAlert.mockClear()
    agentApiMock.sendAgentCommand.mockClear()
    routeState.params = { nodeId: 'node-1' }
    routeState.query = {
      focus: 'certificate',
      commandId: 'cmd-1',
      policyRef: 'acl-1',
      policyDomain: 'acl',
      alertId: 'alert-1',
      eventType: 'policy_failed'
    }
    routeState.fullPath = '/monitoring/nodes/node-1?focus=certificate&commandId=cmd-1&policyRef=acl-1'
  })

  it('derives context summary and highlights the targeted command row', async () => {
    const wrapper = mountWithStubs(NodeMonitorDetail)
    await flushPromises()

    expect(wrapper.vm.contextDescription).toContain('Event: policy_failed')
    expect(wrapper.vm.contextDescription).toContain('Policy: acl-1')
    expect(wrapper.vm.contextDescription).toContain('Command: cmd-1')
    expect(wrapper.vm.certificateStatusLabel).toBe('issued')
    expect(wrapper.vm.certificateActivity.last_renew_failure).toBe('runtime token expired')
    expect(wrapper.text()).toContain('1.1.1.1')
    expect(wrapper.text()).toContain('10.0.0.10')
    expect(wrapper.text()).toContain('1.1.1.1:51820')
    expect(wrapper.text()).toContain('Last Renew Failed At')
    expect(wrapper.text()).toContain('runtime token expired')
    expect(wrapper.vm.scrollToFocusSection).toBeTypeOf('function')
    expect(wrapper.vm.commandRowClassName({ row: { id: 'cmd-1' } })).toBe('context-match-row')
    expect(wrapper.vm.policyRowClassName({ row: { policy_ref: 'acl-1', command_id: 'other' } })).toBe('context-match-row')
    expect(wrapper.vm.alertRowClassName({ row: { id: 'alert-1' } })).toBe('context-match-row')
    expect(wrapper.vm.cmdStatusType('sent')).toBe('warning')
    expect(wrapper.vm.cmdStatusType('acknowledged')).toBe('warning')
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

  it('queues a context sync command and preserves alert context from node detail', async () => {
    const wrapper = mountWithStubs(NodeMonitorDetail)
    await flushPromises()

    await wrapper.vm.runContextCommand('sync')

    expect(agentApiMock.sendAgentCommand).toHaveBeenCalledWith('node-1', {
      command: 'sync',
      params: {
        source: 'node_monitor_detail',
        alert_id: 'alert-1',
        event_type: 'policy_failed',
        command_id: 'cmd-1',
        policy_ref: 'acl-1',
        policy_domain: 'acl'
      },
      timeout: 30
    })
    expect(wrapper.vm.node.recent_commands[0].id).toBe('cmd-new')
  })

  it('resolves the focused alert from node detail and reloads node state', async () => {
    const wrapper = mountWithStubs(NodeMonitorDetail)
    await flushPromises()
    const callsBeforeResolve = monitorApiMock.getNodeDetail.mock.calls.length

    await wrapper.vm.resolveContextAlert()

    expect(monitorApiMock.resolveAlert).toHaveBeenCalledWith('alert-1', {
      source: 'node_monitor_detail',
      reason: 'Resolved from node monitoring detail',
      command_id: 'cmd-1'
    })
    expect(monitorApiMock.getNodeDetail.mock.calls.length).toBeGreaterThan(callsBeforeResolve)
  })

  it('routes focused node context to AI diagnostics', async () => {
    const wrapper = mountWithStubs(NodeMonitorDetail)
    await flushPromises()

    wrapper.vm.askAIForContext()

    expect(routerPush).toHaveBeenCalledWith({
      name: 'AiAssistant',
      query: {
        source: 'node_monitor_detail',
        nodeId: 'node-1',
        alertId: 'alert-1',
        eventType: 'policy_failed',
        commandId: 'cmd-1',
        policyRef: 'acl-1',
        policyDomain: 'acl'
      }
    })
  })
})

describe('policy center context handling', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
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

  it('accepts snake_case context and keeps policyRef primary over a stale command id', async () => {
    routeState.query = {
      node_id: 'node-1',
      policy_ref: 'acl-1',
      policy_domain: 'acl',
      command_id: 'stale-cmd'
    }
    routeState.fullPath = '/policy-center?node_id=node-1&policy_ref=acl-1&policy_domain=acl&command_id=stale-cmd'

    const wrapper = mountWithStubs(Policies)
    await flushPromises()

    expect(wrapper.vm.filters.nodeId).toBe('node-1')
    expect(wrapper.vm.filters.keyword).toBe('acl-1')
    expect(wrapper.vm.filters.kind).toBe('acl')
    expect(wrapper.vm.selectedPolicy.policyId).toBe('policy-1')
    expect(wrapper.vm.detailVisible).toBe(true)
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

  it('routes from Policy Center to IP Group management as part of the policy workspace', async () => {
    const wrapper = mountWithStubs(Policies)
    await flushPromises()

    wrapper.vm.goToIpGroups()

    expect(routerPush).toHaveBeenCalledWith({
      name: 'IPGroups',
      query: {
        nodeId: 'node-1',
        policyRef: 'acl-1',
        commandId: 'cmd-1'
      }
    })
  })

  it('retries failed policy delivery from policy center', async () => {
    const wrapper = mountWithStubs(Policies)
    await flushPromises()
    const callsBeforeRetry = policyApiMock.listPolicies.mock.calls.length

    await wrapper.vm.retryPolicyDelivery({
      nodeId: 'node-1',
      policyRef: 'acl-1',
      kind: 'acl',
      name: 'ACL 1',
      status: 'error'
    })

    expect(policyApiMock.retryPolicySync).toHaveBeenCalledWith({
      nodeId: 'node-1',
      kind: 'acl',
      policyRef: 'acl-1',
      policyName: 'ACL 1'
    })
    expect(policyApiMock.listPolicies.mock.calls.length).toBeGreaterThan(callsBeforeRetry)
    expect(routerPush).toHaveBeenCalledWith({
      name: 'NodeMonitorDetail',
      params: { nodeId: 'node-1' },
      query: {
        commandId: 'cmd-retry',
        focus: 'commands',
        policyRef: 'acl-1',
        policyDomain: 'acl'
      }
    })
  })
})

describe('AI assistant monitoring context', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    routeState.params = {}
    routeState.query = {
      source: 'monitoring',
      nodeId: 'node-1',
      alertId: 'alert-1',
      eventType: 'sync_failed',
      commandId: 'cmd-1',
      policyRef: 'acl-1',
      policyDomain: 'acl'
    }
    routeState.fullPath = '/ai-copilot?source=monitoring&nodeId=node-1&alertId=alert-1'
  })

  it('prefills a diagnostic prompt from monitoring query context', () => {
    const wrapper = mountWithStubs(AIAssistant)

    expect(wrapper.vm.inputMessage).toContain('Diagnose Aria operations alert')
    expect(wrapper.vm.inputMessage).toContain('node_id: node-1')
    expect(wrapper.vm.inputMessage).toContain('alert_id: alert-1')
    expect(wrapper.vm.inputMessage).toContain('event_type: sync_failed')
    expect(wrapper.vm.inputMessage).toContain('policy_ref: acl-1')
    expect(wrapper.vm.inputMessage).toContain('command_id: cmd-1')
  })
})
