// src/config/api.ts

// API Configuration
type ApiEnvironment = 'development' | 'production'

interface ApiEnvironmentConfig {
  baseURL: string
  authEnabled: boolean
}

type PathParam = string | number

export interface CurrentTenant {
  id?: string
  [key: string]: unknown
}

const API_CONFIG: Record<ApiEnvironment, ApiEnvironmentConfig> = {
  // Development configuration
  development: {
    baseURL: '/api',
    authEnabled: false  // Temporarily disable auth until backend provides login endpoint
  },

  // Production configuration
  production: {
    baseURL: '/api',
    authEnabled: true
  }
};

// Determine environment
const getEnvironment = (): ApiEnvironment => {
  // In browser environment
  if (typeof window !== 'undefined') {
    const hostname = window.location.hostname;

    // If on the production domain, use production config
    if (hostname === 'aria.yun' || hostname.includes('.aria.yun')) {
      return 'production';
    }
  }

  // Default to development for local testing
  return 'development';
};

const ENV = getEnvironment();
export const API_BASE_URL = API_CONFIG[ENV].baseURL;
export const AUTH_ENABLED = API_CONFIG[ENV].authEnabled;

const readCurrentTenant = (): CurrentTenant | null => {
  if (typeof window === 'undefined') {
    return null
  }

  const raw = localStorage.getItem('aria-current-tenant')
  if (!raw) {
    return null
  }

  try {
    const parsed: unknown = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object') {
      return null
    }
    return parsed as CurrentTenant
  } catch (error) {
    console.warn('Failed to parse current tenant from localStorage:', error)
    return null
  }
}

export const getCurrentTenant = (): CurrentTenant | null => readCurrentTenant()

export const getCurrentTenantId = (): string | null => readCurrentTenant()?.id || null

export const requireCurrentTenantId = (): string => {
  const tenantId = getCurrentTenantId()
  if (!tenantId) {
    throw new Error('No tenant selected')
  }
  return tenantId
}

export const buildTenantPath = (tenantId: string, suffix = ''): string => {
  const normalizedSuffix = suffix.startsWith('/') || suffix === '' ? suffix : `/${suffix}`
  return `/v2/tenants/${tenantId}${normalizedSuffix}`
}

// API endpoints - 与后端 v2 API 对接
export const API_ENDPOINTS = {
  // 认证相关
  AUTH: {
    LOGIN: '/v2/auth/login',
    REFRESH: '/v2/auth/refresh',
    LOGOUT: '/v2/auth/logout',
    FORCE_CHANGE_PASSWORD: '/v2/auth/force-change-password',
    PERMISSIONS: '/v2/auth/permissions'
  },

  // 系统设置 API
  SETTINGS: {
    BACKUPS: '/v2/settings/backups',
    BACKUP_UPLOAD: '/v2/settings/backups/upload',
    BACKUP_DETAIL: (id: string) => `/v2/settings/backups/${id}`,
    BACKUP_DOWNLOAD: (id: string) => `/v2/settings/backups/${id}/download`,
    BACKUP_RESTORE: (id: string) => `/v2/settings/backups/${id}/restore`,
    BACKUP_RESTORE_DRY_RUN: (id: string) => `/v2/settings/backups/${id}/restore`
  },

  // 租户管理 API
  TENANT: {
    LIST: '/v2/tenants',
    DETAIL: (tenantId: string) => buildTenantPath(tenantId),
    USERS: (tenantId: string) => buildTenantPath(tenantId, '/users'),
    USER_DETAIL: (tenantId: string, userId: string) => buildTenantPath(tenantId, `/users/${userId}`),
    TOKENS: (tenantId: string) => buildTenantPath(tenantId, '/tokens'),
    TOKEN_DETAIL: (tenantId: string, tokenId: string) => buildTenantPath(tenantId, `/tokens/${tokenId}`),
    POLICIES: (tenantId: string) => buildTenantPath(tenantId, '/policies'),
    POLICY_RETRY: (tenantId: string) => buildTenantPath(tenantId, '/policies/retry'),
    NODES: (tenantId: string) => buildTenantPath(tenantId, '/nodes'),
    NODE_DETAIL: (tenantId: string, nodeId: string) => buildTenantPath(tenantId, `/nodes/${nodeId}`),
    NODE_ROUTES: (tenantId: string, nodeId: string) => buildTenantPath(tenantId, `/nodes/${nodeId}/routes`),
    NODE_ROUTE: (tenantId: string, nodeId: string, routeId: PathParam) => buildTenantPath(tenantId, `/nodes/${nodeId}/routes/${encodeURIComponent(String(routeId))}`),
    NODE_ACLS: (tenantId: string, nodeId: string) => buildTenantPath(tenantId, `/nodes/${nodeId}/security/acls`),
    NODE_ACL: (tenantId: string, nodeId: string, ruleId: PathParam) => buildTenantPath(tenantId, `/nodes/${nodeId}/security/acls/${ruleId}`),
    NODE_QOS: (tenantId: string, nodeId: string) => buildTenantPath(tenantId, `/nodes/${nodeId}/qos`),
    NODE_QOS_RULE: (tenantId: string, nodeId: string, ruleId: PathParam) => buildTenantPath(tenantId, `/nodes/${nodeId}/qos/${ruleId}`),
    IP_GROUPS: (tenantId: string) => buildTenantPath(tenantId, '/ip-groups'),
    IP_GROUP: (tenantId: string, groupId: string) => buildTenantPath(tenantId, `/ip-groups/${groupId}`),
    AI_CHAT: (tenantId: string) => buildTenantPath(tenantId, '/ai/chat'),
    AI_CONFIRM: (tenantId: string) => buildTenantPath(tenantId, '/ai/confirm'),
    ROLES: (tenantId: string) => buildTenantPath(tenantId, '/roles'),
    ROLE_DETAIL: (tenantId: string, roleId: string) => buildTenantPath(tenantId, `/roles/${roleId}`)
  },

  // Agent 代理 API
  AGENT: {
    COMMAND: (tenantId: string, nodeId: string) => buildTenantPath(tenantId, `/nodes/${nodeId}/agent/command`),
    COMMANDS: (tenantId: string, nodeId: string) => buildTenantPath(tenantId, `/nodes/${nodeId}/agent/commands`),
    STATUS: (tenantId: string, nodeId: string) => buildTenantPath(tenantId, `/nodes/${nodeId}/agent/status`),
    BATCH_COMMAND: (tenantId: string) => buildTenantPath(tenantId, '/agents/command')
  },

  // 节点管理 API
  NODES: {
    LIST: (tenantId: string) => buildTenantPath(tenantId, '/nodes'),
    DETAIL: (tenantId: string, nodeId: string) => buildTenantPath(tenantId, `/nodes/${nodeId}`),
    SYNC: (tenantId: string, nodeId: string) => buildTenantPath(tenantId, `/nodes/${nodeId}/agent/sync`)
  },

  // 带宽管理 API
  BANDWIDTH: {
    LIST: (tenantId: string, nodeId: string) => buildTenantPath(tenantId, `/nodes/${nodeId}/qos`),
    RULE: (tenantId: string, nodeId: string, ruleId: PathParam) => buildTenantPath(tenantId, `/nodes/${nodeId}/qos/${ruleId}`)
  },

  // 监控 API
  MONITOR: {
    STATS: (tenantId: string) => buildTenantPath(tenantId, '/monitoring/stats'),
    NODE_DETAIL: (tenantId: string, nodeId: string) => buildTenantPath(tenantId, `/monitoring/nodes/${nodeId}`),
    EVENTS: (tenantId: string) => buildTenantPath(tenantId, '/monitoring/events'),
    ALERTS: (tenantId: string) => buildTenantPath(tenantId, '/monitoring/alerts'),
    ALERT_RESOLVE: (tenantId: string, alertId: string) => buildTenantPath(tenantId, `/monitoring/alerts/${alertId}/resolve`),
    TRAFFIC: (tenantId: string) => buildTenantPath(tenantId, '/monitoring/traffic'),
    HEALTH: (tenantId: string) => buildTenantPath(tenantId, '/monitoring/health'),
    NODE_METRICS: (tenantId: string, nodeId: string) => buildTenantPath(tenantId, `/monitoring/nodes/${nodeId}/metrics`),
    TOPOLOGY: (tenantId: string) => buildTenantPath(tenantId, '/monitoring/topology')
  },

  // AI 聊天 API
  AI: {
    CHAT: (tenantId: string) => buildTenantPath(tenantId, '/ai/chat'),
    CONFIRM: (tenantId: string) => buildTenantPath(tenantId, '/ai/confirm')
  },

  // 即时通讯 Webhook
  IM: {
    DINGTALK: '/v2/integrations/dingtalk/webhook',
    FEISHU: '/v2/integrations/feishu/webhook'
  },

  // 健康检查
  HEALTH: '/health',

  // 版本
  VERSION: '/version'
};
