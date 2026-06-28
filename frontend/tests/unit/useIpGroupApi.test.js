import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useIpGroupApi } from '@/composables/useIpGroupApi'

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

describe('useIpGroupApi', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('应该规范化 IP Group 列表响应', async () => {
    api.get.mockResolvedValue({
      data: {
        success: true,
        data: [
          {
            id: 'group-1',
            name: 'office',
            kind: 'custom',
            members: [{ cidr: '10.10.0.0/16' }]
          }
        ]
      }
    })

    const groups = await useIpGroupApi.listIPGroups()

    expect(api.get).toHaveBeenCalledWith('/v2/tenants/tenant-1/ip-groups')
    expect(groups).toEqual([
      expect.objectContaining({
        id: 'group-1',
        name: 'office',
        kind: 'custom',
        members: [expect.objectContaining({ cidr: '10.10.0.0/16' })],
        warnings: []
      })
    ])
  })

  it('应该创建 IP Group 并过滤空 CIDR 成员', async () => {
    api.post.mockResolvedValue({
      data: { success: true, data: { id: 'group-1', name: 'office', kind: 'custom', members: [] } }
    })

    await useIpGroupApi.createIPGroup({
      name: 'office',
      description: 'office networks',
      members: [
        { cidr: '10.10.0.0/16', note: 'office' },
        { cidr: '', note: 'empty' }
      ]
    })

    expect(api.post).toHaveBeenCalledWith('/v2/tenants/tenant-1/ip-groups', {
      name: 'office',
      description: 'office networks',
      kind: 'custom',
      members: [{ cidr: '10.10.0.0/16', note: 'office' }]
    })
  })

  it('应该分页加载 IP 组引用并规范化跳转信息', async () => {
    api.get.mockResolvedValue({
      data: {
        success: true,
        data: {
          items: [
            {
              domain: 'acl',
              rule_id: 'acl-1',
              rule_name: 'office-acl',
              node_id: 'node-1',
              node_name: 'edge-1',
              direction: 'egress',
              enabled: true,
              latest_delivery: {
                status: 'completed',
                command_id: 'cmd-1',
                last_error: '',
                created_at: '2026-06-28T10:12:00Z'
              },
              route: {
                name: 'ACLRules',
                path: '/policy-center/acls',
                query: {
                  node_id: 'node-1',
                  rule_id: 'acl-1'
                }
              }
            }
          ],
          total: 1,
          limit: 20,
          offset: 0,
          has_more: false
        }
      }
    })

    const page = await useIpGroupApi.listIPGroupReferences('group-1', { limit: 20, offset: 0 })

    expect(api.get).toHaveBeenCalledWith('/v2/tenants/tenant-1/ip-groups/group-1/references', {
      params: { limit: 20, offset: 0 }
    })
    expect(page.items[0]).toEqual(expect.objectContaining({
      domain: 'acl',
      rule_id: 'acl-1',
      rule_name: 'office-acl',
      route: expect.objectContaining({
        path: '/policy-center/acls',
        query: { node_id: 'node-1', rule_id: 'acl-1' }
      })
    }))
  })
})
