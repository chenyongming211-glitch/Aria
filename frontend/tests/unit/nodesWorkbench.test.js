import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

const {
  routerPush,
  mockNodeStore,
  sendAgentCommandMock,
  getNodeMetricsMock,
  tokenApiMock
} = vi.hoisted(() => ({
  routerPush: vi.fn(),
  mockNodeStore: {
    nodes: [
      {
        id: 'node-1',
        hostname: 'edge-1',
        publicIp: '203.0.113.10',
        vpnIp: '100.64.0.2',
        endpoint: '203.0.113.10:51820',
        region: 'sh',
        status: 'online',
        mode: 'kernel',
        desiredStateVersion: 'desired-1',
        appliedStateVersion: 'applied-1',
        stateConvergence: 'converged',
        routes: ['10.10.0.0/16']
      }
    ],
    loading: false,
    loadNodes: vi.fn(async () => {}),
    loadNodeDetail: vi.fn(async () => ({
      id: 'node-1',
      hostname: 'edge-1',
      publicIp: '203.0.113.10',
      vpnIp: '100.64.0.2',
      endpoint: '203.0.113.10:51820',
      region: 'sh',
      status: 'online',
      uptime: '1 hour',
      desiredStateVersion: 'desired-1',
      appliedStateVersion: 'applied-1',
      observedState: 'healthy',
      observedMessage: 'sync applied successfully',
      stateConvergence: 'converged',
      lastSyncAt: '2026-05-30 18:00:00',
      pendingCmds: 0,
      routes: ['10.10.0.0/16'],
      learnedRoutes: [],
      recentCommands: [],
      recentPolicyDeliveries: [
        {
          id: 'delivery-1',
          policy_domain: 'acl',
          policy_ref: 'acl-1',
          policy_name: 'allow-office',
          command_id: 'cmd-policy-1',
          command_status: 'failed',
          last_error: 'iptables apply failed',
          updated_at: '2026-05-30T10:00:00Z'
        }
      ],
      activeAlerts: [
        {
          id: 'alert-1',
          alert_type: 'certificate_expiring',
          severity: 'warning',
          title: 'Certificate expiring',
          message: 'node certificate expires soon',
          created_at: '2026-05-30T10:00:00Z'
        }
      ],
      certificate: {
        status: 'issued',
        serial_number: 'serial-1',
        issued_at: '2026-05-20T10:00:00Z',
        not_after: '2026-06-20T10:00:00Z'
      },
      certificateActivity: {
        last_renewed_at: '2026-05-29T10:00:00Z',
        last_renewed_serial_number: 'serial-1'
      },
      bandwidth: { upload: 0, download: 0 },
      latency: 0
    })),
    updateNodeRemote: vi.fn(async () => {}),
    deleteNodeRemote: vi.fn(async () => {})
  },
  sendAgentCommandMock: vi.fn(async () => ({
    command_id: 'cmd-queued',
    command: 'sync',
    status: 'pending',
    message: 'Command queued for delivery',
    created_at: '2026-05-30T10:05:00Z'
  })),
  getNodeMetricsMock: vi.fn(async () => ({ upload_mbps: 1.234, download_mbps: 2.345, latency_ms: 12.3 })),
  tokenApiMock: {
    createToken: vi.fn(async () => ({
      id: 'token-1',
      token: 'enroll-secret-123456',
      token_preview: 'enroll-secret...',
      tag: 'node-onboarding',
      max_uses: 1,
      expires_at: '2026-06-26T00:00:00Z'
    }))
  }
}))

vi.mock('/src/stores/node', () => ({
  default: () => mockNodeStore
}))

vi.mock('/src/composables/usePermission', () => ({
  usePermission: () => ({
    hasPermission: () => true
  })
}))

vi.mock('/src/composables/useAgentProxyApi', () => ({
  useAgentProxyApi: {
    sendAgentCommand: sendAgentCommandMock
  }
}))

vi.mock('/src/composables/useMonitorApi', () => ({
  useMonitorApi: {
    getNodeMetrics: getNodeMetricsMock
  }
}))

vi.mock('/src/composables/useTokenApi', () => ({
  useTokenApi: tokenApiMock
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: routerPush
  })
}))

vi.mock('element-plus', () => ({
  ElMessage: {
    success: vi.fn(),
    error: vi.fn(),
    warning: vi.fn()
  },
  ElMessageBox: {
    alert: vi.fn(async () => {})
  }
}))

import Nodes from '@/views/Nodes.vue'

const elementStubs = {
  'el-card': { template: '<div><slot name="header" /><slot /></div>' },
  'el-input': { template: '<div><slot name="prefix" /><slot name="append" /></div>' },
  'el-input-number': { template: '<input />' },
  'el-button': { template: '<button @click="$emit(\'click\')"><slot /></button>' },
  'el-icon': { template: '<i><slot /></i>' },
  'el-tag': { template: '<span><slot /></span>' },
  'el-tooltip': { template: '<div><slot /></div>' },
  'el-alert': { template: '<div><slot />{{ title }}</div>', props: ['title'] },
  'el-empty': { template: '<div></div>' },
  'el-pagination': { template: '<div></div>' },
  'el-popconfirm': { template: '<div><slot name="reference" /></div>' },
  'el-dialog': { template: '<div><slot /><slot name="footer" /></div>' },
  'el-form': { template: '<form><slot /></form>' },
  'el-form-item': { template: '<div><slot /></div>' },
  'el-descriptions': { template: '<div><slot /></div>' },
  'el-descriptions-item': { template: '<div><slot /></div>' },
  'el-table': { template: '<div><slot /></div>' },
  'el-table-column': {
    props: ['prop'],
    template: '<div><slot :row="row" />{{ prop ? row[prop] : "" }}</div>',
    data() {
      return {
        row: {
          id: 'row-1',
          command: 'sync',
          status: 'failed',
          message: 'sync failed',
          command_id: 'cmd-policy-1',
          command_status: 'failed',
          policy_domain: 'acl',
          policy_ref: 'acl-1',
          policy_name: 'allow-office',
          last_error: 'iptables apply failed',
          alert_type: 'certificate_expiring',
          severity: 'warning',
          title: 'Certificate expiring',
          created_at: '2026-05-30T10:00:00Z',
          updated_at: '2026-05-30T10:00:00Z',
          region: 'sh',
          status: 'online'
        }
      }
    }
  }
}

const mountNodes = () => mount(Nodes, {
  global: {
    stubs: elementStubs,
    directives: {
      loading: {}
    }
  }
})

describe('Nodes workbench detail', () => {
  beforeEach(() => {
    routerPush.mockReset()
    sendAgentCommandMock.mockClear()
    getNodeMetricsMock.mockClear()
    tokenApiMock.createToken.mockClear()
    mockNodeStore.loadNodes.mockClear()
    mockNodeStore.loadNodeDetail.mockClear()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('renders certificate and operations context in the node detail dialog', async () => {
    const wrapper = mountNodes()
    await wrapper.vm.viewNodeDetails(mockNodeStore.nodes[0])
    await flushPromises()

    expect(wrapper.text()).toContain('Operations Summary')
    expect(wrapper.text()).toContain('203.0.113.10:51820')
    expect(wrapper.text()).toContain('Certificate Status')
    expect(wrapper.text()).toContain('serial-1')
    expect(wrapper.text()).toContain('certificate_expiring')
    expect(wrapper.text()).toContain('allow-office')
  })

  it('prepends queued quick commands before the next backend refresh catches up', async () => {
    const wrapper = mountNodes()
    await wrapper.vm.viewNodeDetails(mockNodeStore.nodes[0])
    await wrapper.vm.runQuickCommand('sync')
    await flushPromises()

    expect(sendAgentCommandMock).toHaveBeenCalledWith('node-1', {
      command: 'sync',
      params: {},
      timeout: 30
    })
    expect(wrapper.vm.selectedNode.recentCommands[0]).toMatchObject({
      id: 'cmd-queued',
      command: 'sync',
      status: 'pending'
    })
  })

  it('polls command status until the queued quick command reaches a terminal state', async () => {
    vi.useFakeTimers()

    const baseDetail = await mockNodeStore.loadNodeDetail()
    mockNodeStore.loadNodeDetail.mockReset()
    mockNodeStore.loadNodeDetail
      .mockResolvedValueOnce(baseDetail)
      .mockResolvedValueOnce({
        ...baseDetail,
        recentCommands: [
          { id: 'cmd-queued', command: 'sync', status: 'sent', message: 'sent to agent' }
        ]
      })
      .mockResolvedValueOnce({
        ...baseDetail,
        recentCommands: [
          { id: 'cmd-queued', command: 'sync', status: 'acknowledged', message: 'agent acknowledged' }
        ]
      })
      .mockResolvedValueOnce({
        ...baseDetail,
        recentCommands: [
          { id: 'cmd-queued', command: 'sync', status: 'completed', message: 'sync completed' }
        ]
      })

    const wrapper = mountNodes()
    await wrapper.vm.viewNodeDetails(mockNodeStore.nodes[0])
    await wrapper.vm.runQuickCommand('sync')
    await flushPromises()

    expect(wrapper.vm.selectedNode.recentCommands[0].status).toBe('sent')

    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(wrapper.vm.selectedNode.recentCommands[0].status).toBe('acknowledged')

    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(wrapper.vm.selectedNode.recentCommands[0].status).toBe('completed')
  })

  it('preserves node context when opening monitoring and policy center', async () => {
    const wrapper = mountNodes()
    await wrapper.vm.viewNodeDetails(mockNodeStore.nodes[0])

    wrapper.vm.openMonitoringDetail('alerts')
    expect(routerPush).toHaveBeenCalledWith({
      name: 'NodeMonitorDetail',
      params: { nodeId: 'node-1' },
      query: { focus: 'alerts' }
    })

    wrapper.vm.openPolicyCenter({
      policy_ref: 'acl-1',
      policy_domain: 'acl',
      command_id: 'cmd-policy-1'
    })
    expect(routerPush).toHaveBeenCalledWith({
      name: 'Policies',
      query: {
        nodeId: 'node-1',
        policyRef: 'acl-1',
        kind: 'acl',
        commandId: 'cmd-policy-1'
      }
    })
  })

  it('opens onboarding wizard, creates an enrollment token, and builds an init command', async () => {
    const writeText = vi.fn(async () => {})
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true
    })

    const wrapper = mountNodes()
    wrapper.vm.addNode()
    expect(wrapper.vm.onboardingDialogVisible).toBe(true)

    wrapper.vm.onboardingForm.server = 'https://aria.yun:50051'
    wrapper.vm.onboardingForm.controllerApiUrl = 'https://aria.yun'
    wrapper.vm.onboardingForm.region = 'tencent-cloud'
    wrapper.vm.onboardingForm.interface = 'aria0'
    wrapper.vm.onboardingForm.hostname = 'edge-new'
    wrapper.vm.onboardingForm.advertiseRoutes = '10.10.0.0/16,10.20.0.0/16'

    await wrapper.vm.createOnboardingToken()
    await flushPromises()

    expect(tokenApiMock.createToken).toHaveBeenCalledWith({
      tag: 'node-onboarding',
      max_uses: 1,
      ttl: '24h'
    })
    expect(wrapper.vm.onboardingToken.token).toBe('enroll-secret-123456')
    expect(wrapper.vm.onboardingInitCommand).toContain('sudo aria-agent init')
    expect(wrapper.vm.onboardingInitCommand).toContain('--server https://aria.yun:50051')
    expect(wrapper.vm.onboardingInitCommand).toContain('--token enroll-secret-123456')
    expect(wrapper.vm.onboardingInitCommand).toContain('--controller-api-url https://aria.yun')
    expect(wrapper.vm.onboardingInitCommand).toContain('--ca-cert /etc/aria/certs/ca.crt')
    expect(wrapper.vm.onboardingInitCommand).toContain('--tls-server-name aria.yun')
    expect(wrapper.vm.onboardingInitCommand).toContain('--advertise-routes 10.10.0.0/16,10.20.0.0/16')

    await wrapper.vm.copyOnboardingCommand()
    expect(writeText).toHaveBeenCalledWith(wrapper.vm.onboardingInitCommand)
  })
})
