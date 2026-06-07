import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import useNodeStore from '@/stores/node'

vi.mock('@/composables/useApi', () => ({
  default: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn()
  }
}))

vi.mock('@/config/api', async () => {
  const actual = await vi.importActual('@/config/api')
  return {
    ...actual,
    requireCurrentTenantId: vi.fn(() => 'tenant-1')
  }
})

import api from '@/composables/useApi'

describe('node store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('deletes nodes through the tenant-scoped backend API', async () => {
    api.delete.mockResolvedValue({ data: { success: true } })
    const store = useNodeStore()
    store.nodes = [{ id: 'node-1' }, { id: 'node-2' }]

    await store.deleteNodeRemote('node-1')

    expect(api.delete).toHaveBeenCalledWith('/v2/tenants/tenant-1/nodes/node-1')
    expect(store.nodes).toEqual([{ id: 'node-2' }])
  })

  it('updates cached nodes from the server response instead of submitted form data', async () => {
    api.put.mockResolvedValue({
      data: {
        success: true,
        data: {
          id: 'node-1',
          hostname: 'server-hostname',
          region: 'server-region',
          advertised_routes: ['10.20.0.0/16'],
          status: 'offline'
        }
      }
    })

    const store = useNodeStore()
    store.nodes = [
      {
        id: 'node-1',
        hostname: 'old-hostname',
        region: 'old-region',
        routes: ['10.10.0.0/16'],
        status: 'online'
      }
    ]

    await store.updateNodeRemote('node-1', {
      hostname: 'submitted-hostname',
      region: 'submitted-region',
      advertised_routes: ['10.30.0.0/16']
    })

    expect(api.put).toHaveBeenCalledWith('/v2/tenants/tenant-1/nodes/node-1', {
      hostname: 'submitted-hostname',
      region: 'submitted-region',
      advertised_routes: ['10.30.0.0/16']
    })
    expect(store.nodes[0].hostname).toBe('server-hostname')
    expect(store.nodes[0].region).toBe('server-region')
    expect(store.nodes[0].routes).toEqual(['10.20.0.0/16'])
    expect(store.nodes[0].status).toBe('offline')
  })

  it('normalizes monitoring detail fields for the node workbench', async () => {
    api.get.mockImplementation(async (url) => {
      if (url === '/v2/tenants/tenant-1/nodes/node-1') {
        return {
          data: {
            success: true,
            data: {
              id: 'node-1',
              hostname: 'edge-1',
              assigned_ip: '100.64.0.2',
              public_ip: '203.0.113.10',
              endpoint: '203.0.113.10:51820',
              region: 'sh',
              status: 'online',
              advertised_routes: ['10.10.0.0/16']
            }
          }
        }
      }

      if (url === '/v2/tenants/tenant-1/monitoring/nodes/node-1') {
        return {
          data: {
            success: true,
            data: {
              hostname: 'edge-1',
              availability_status: 'online',
              desired_state_version: 'desired-1',
              applied_state_version: 'applied-1',
              observed_state: 'healthy',
              observed_message: 'sync applied successfully',
              state_convergence: 'converged',
              certificate: {
                status: 'issued',
                serial_number: 'serial-1',
                issued_at: '2026-05-30T10:00:00Z',
                not_after: '2026-06-30T10:00:00Z'
              },
              certificate_activity: {
                last_renewed_at: '2026-05-29T10:00:00Z',
                last_renewed_serial_number: 'serial-1'
              },
              recent_commands: [
                { id: 'cmd-monitor', command: 'sync', status: 'completed' }
              ],
              recent_policy_deliveries: [
                {
                  id: 'delivery-1',
                  policy_domain: 'acl',
                  policy_ref: 'acl-1',
                  command_id: 'cmd-monitor',
                  command_status: 'completed'
                }
              ],
              active_alerts: [
                { id: 'alert-1', alert_type: 'certificate_expiring', severity: 'warning' }
              ]
            }
          }
        }
      }

      if (url === '/v2/tenants/tenant-1/nodes/node-1/agent/status') {
        return {
          data: {
            success: true,
            data: {
              pending_cmds: 0,
              configuration_status: 'applied'
            }
          }
        }
      }

      if (url === '/v2/tenants/tenant-1/nodes/node-1/agent/commands') {
        return {
          data: {
            success: true,
            data: {
              items: [
                { id: 'cmd-api', command: 'health_check', status: 'completed' }
              ]
            }
          }
        }
      }

      throw new Error(`unexpected url ${url}`)
    })

    const store = useNodeStore()
    const node = await store.loadNodeDetail('node-1')

    expect(node.desiredStateVersion).toBe('desired-1')
    expect(node.endpoint).toBe('203.0.113.10:51820')
    expect(node.appliedStateVersion).toBe('applied-1')
    expect(node.observedState).toBe('healthy')
    expect(node.certificate.status).toBe('issued')
    expect(node.certificate.serial_number).toBe('serial-1')
    expect(node.certificateActivity.last_renewed_serial_number).toBe('serial-1')
    expect(node.recentCommands[0].id).toBe('cmd-api')
    expect(node.recentPolicyDeliveries[0].policy_ref).toBe('acl-1')
    expect(node.activeAlerts[0].alert_type).toBe('certificate_expiring')
  })

  it('keeps node detail usable when auxiliary agent endpoints fail', async () => {
    api.get.mockImplementation(async (url) => {
      if (url === '/v2/tenants/tenant-1/nodes/node-1') {
        return {
          data: {
            success: true,
            data: {
              id: 'node-1',
              hostname: 'edge-1',
              assigned_ip: '100.64.0.2',
              public_ip: '203.0.113.10',
              region: 'sh',
              status: 'online'
            }
          }
        }
      }

      if (url === '/v2/tenants/tenant-1/monitoring/nodes/node-1') {
        return {
          data: {
            success: true,
            data: {
              hostname: 'edge-1',
              availability_status: 'online',
              recent_commands: [
                { id: 'cmd-monitor', command: 'sync', status: 'completed' }
              ],
              certificate: {
                status: 'issued',
                serial_number: 'serial-1'
              },
              active_alerts: [
                { id: 'alert-1', alert_type: 'sync_failed', severity: 'warning' }
              ]
            }
          }
        }
      }

      if (url === '/v2/tenants/tenant-1/nodes/node-1/agent/status') {
        throw new Error('status unavailable')
      }

      if (url === '/v2/tenants/tenant-1/nodes/node-1/agent/commands') {
        throw new Error('commands unavailable')
      }

      throw new Error(`unexpected url ${url}`)
    })

    const store = useNodeStore()
    const node = await store.loadNodeDetail('node-1')

    expect(node.hostname).toBe('edge-1')
    expect(node.status).toBe('online')
    expect(node.certificate.status).toBe('issued')
    expect(node.recentCommands[0].id).toBe('cmd-monitor')
    expect(node.activeAlerts[0].alert_type).toBe('sync_failed')
  })
})
