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
})
