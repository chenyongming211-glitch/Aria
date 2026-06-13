import api from './useApi'
import { API_ENDPOINTS, requireCurrentTenantId } from '@/config/api'

function normalizeListResponse(response) {
  const body = response?.data
  if (Array.isArray(body)) return body
  if (!body || typeof body !== 'object') return []

  if ('success' in body) {
    const data = body.data
    if (data == null) return []
    if (Array.isArray(data)) return data
    if (Array.isArray(data.items)) return data.items
    throw new Error('Invalid IP group list response')
  }

  if (Array.isArray(body.items)) return body.items
  return []
}

function normalizeMembers(members = []) {
  if (!Array.isArray(members)) return []
  return members
    .map((member) => ({
      id: member.id || '',
      group_id: member.group_id || '',
      cidr: String(member.cidr || '').trim(),
      note: member.note || ''
    }))
    .filter((member) => member.cidr)
}

function normalizeIPGroup(group = {}) {
  return {
    ...group,
    id: group.id || '',
    name: group.name || '',
    description: group.description || '',
    kind: group.kind || 'custom',
    members: normalizeMembers(group.members),
    warnings: Array.isArray(group.warnings) ? group.warnings : []
  }
}

function normalizePayload(group = {}) {
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

  getIPGroup: async (groupId) => {
    const tenantId = requireCurrentTenantId()
    const response = await api.get(API_ENDPOINTS.TENANT.IP_GROUP(tenantId, groupId))
    return normalizeIPGroup(response.data?.data || response.data)
  },

  createIPGroup: async (group) => {
    const tenantId = requireCurrentTenantId()
    const response = await api.post(API_ENDPOINTS.TENANT.IP_GROUPS(tenantId), normalizePayload(group))
    return normalizeIPGroup(response.data?.data || response.data)
  },

  updateIPGroup: async (groupId, group) => {
    const tenantId = requireCurrentTenantId()
    const response = await api.put(API_ENDPOINTS.TENANT.IP_GROUP(tenantId, groupId), normalizePayload(group))
    return normalizeIPGroup(response.data?.data || response.data)
  },

  deleteIPGroup: async (groupId) => {
    const tenantId = requireCurrentTenantId()
    const response = await api.delete(API_ENDPOINTS.TENANT.IP_GROUP(tenantId, groupId))
    return response.data?.data || response.data
  },

  formatGroupLabel: (group) => {
    if (!group) return 'any'
    const members = normalizeMembers(group.members)
    const suffix = members.length > 0 ? ` (${members.map((member) => member.cidr).join(', ')})` : ''
    return `${group.name || group.id}${suffix}`
  }
}
