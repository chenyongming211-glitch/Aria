import api from './useApi'
import { API_ENDPOINTS, requireCurrentTenantId } from '@/config/api'
import type { IPGroupMember, IPGroupRecord, NormalizedIPGroup } from '@/types/policy'

interface ApiResponseLike {
  data?: unknown
}

type UnknownRecord = Record<string, unknown>

function isRecord(value: unknown): value is UnknownRecord {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

function isIPGroupMember(value: unknown): value is Partial<IPGroupMember> {
  return isRecord(value)
}

function isIPGroupRecord(value: unknown): value is IPGroupRecord {
  return isRecord(value)
}

function normalizeListResponse(response: ApiResponseLike): IPGroupRecord[] {
  const body = response?.data
  if (Array.isArray(body)) return body
  if (!body || typeof body !== 'object') return []

  if ('success' in body) {
    const data = (body as UnknownRecord).data
    if (data == null) return []
    if (Array.isArray(data)) return data
    if (isRecord(data) && Array.isArray(data.items)) return data.items as IPGroupRecord[]
    throw new Error('Invalid IP group list response')
  }

  if (isRecord(body) && Array.isArray(body.items)) return body.items as IPGroupRecord[]
  return []
}

function normalizeMembers(members: unknown = []): IPGroupMember[] {
  if (!Array.isArray(members)) return []
  return members
    .filter(isIPGroupMember)
    .map((member) => ({
      id: member.id || '',
      group_id: member.group_id || '',
      cidr: String(member.cidr || '').trim(),
      note: member.note || ''
    }))
    .filter((member) => member.cidr)
}

function normalizeIPGroup(group: unknown = {}): NormalizedIPGroup {
  const raw = isIPGroupRecord(group) ? group : {}
  return {
    ...raw,
    id: raw.id || '',
    name: raw.name || '',
    description: raw.description || '',
    kind: raw.kind || 'custom',
    members: normalizeMembers(raw.members),
    warnings: Array.isArray(raw.warnings) ? raw.warnings : []
  }
}

function normalizePayload(group: Partial<IPGroupRecord> = {}) {
  return {
    name: String(group.name || '').trim(),
    description: group.description || '',
    kind: group.kind || 'custom',
    members: normalizeMembers(group.members).map((member) => ({
      cidr: member.cidr,
      note: member.note || ''
    }))
  }
}

export const useIpGroupApi = {
  listIPGroups: async () => {
    const tenantId = requireCurrentTenantId()
    const response = await api.get(API_ENDPOINTS.TENANT.IP_GROUPS(tenantId))
    return normalizeListResponse(response).map(normalizeIPGroup)
  },

  getIPGroup: async (groupId: string) => {
    const tenantId = requireCurrentTenantId()
    const response = await api.get(API_ENDPOINTS.TENANT.IP_GROUP(tenantId, groupId))
    return normalizeIPGroup(response.data?.data || response.data)
  },

  createIPGroup: async (group: Partial<IPGroupRecord>) => {
    const tenantId = requireCurrentTenantId()
    const response = await api.post(API_ENDPOINTS.TENANT.IP_GROUPS(tenantId), normalizePayload(group))
    return normalizeIPGroup(response.data?.data || response.data)
  },

  updateIPGroup: async (groupId: string, group: Partial<IPGroupRecord>) => {
    const tenantId = requireCurrentTenantId()
    const response = await api.put(API_ENDPOINTS.TENANT.IP_GROUP(tenantId, groupId), normalizePayload(group))
    return normalizeIPGroup(response.data?.data || response.data)
  },

  deleteIPGroup: async (groupId: string) => {
    const tenantId = requireCurrentTenantId()
    const response = await api.delete(API_ENDPOINTS.TENANT.IP_GROUP(tenantId, groupId))
    return response.data?.data || response.data
  },

  formatGroupLabel: (group?: Partial<IPGroupRecord> | null) => {
    if (!group) return 'any'
    const members = normalizeMembers(group.members)
    const suffix = members.length > 0 ? ` (${members.map((member) => member.cidr).join(', ')})` : ''
    return `${group.name || group.id}${suffix}`
  }
}
