// src/config/api.js

// API Configuration
const API_CONFIG = {
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
const getEnvironment = () => {
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

const readCurrentTenant = () => {
  if (typeof window === 'undefined') {
    return null
  }

  const raw = localStorage.getItem('aria-current-tenant')
  if (!raw) {
    return null
  }

  try {
    return JSON.parse(raw)
  } catch (error) {
    console.warn('Failed to parse current tenant from localStorage:', error)
    return null
  }
}

export const getCurrentTenant = () => readCurrentTenant()

export const getCurrentTenantId = () => readCurrentTenant()?.id || null

export const requireCurrentTenantId = () => {
  const tenantId = getCurrentTenantId()
  if (!tenantId) {
    throw new Error('No tenant selected')
  }
  return tenantId
}

export const buildTenantPath = (tenantId, suffix = '') => {
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
    FORCE_CHANGE_PASSWORD: '/v2/auth/force-change-password'
  },

  // 用户相关 (v2 规划中，目前集成在 tenant-scoped handlers)
  USER: {
    PROFILE: '/v2/user/profile',
    SETTINGS: '/v2/user/settings'
  },

  // 租户管理 API
  TENANT: {
    LIST: '/v2/tenants',
    DETAIL: (tenantId) => buildTenantPath(tenantId),
    USERS: (tenantId) => buildTenantPath(tenantId, '/users'),
    USER_DETAIL: (tenantId, userId) => buildTenantPath(tenantId, `/users/${userId}`),
    TOKENS: (tenantId) => buildTenantPath(tenantId, '/tokens'),
    TOKEN_DETAIL: (tenantId, tokenId) => buildTenantPath(tenantId, `/tokens/${tokenId}`),
    POLICIES: (tenantId) => buildTenantPath(tenantId, '/policies'),
    NODES: (tenantId) => buildTenantPath(tenantId, '/nodes'),
    NODE_DETAIL: (tenantId, nodeId) => buildTenantPath(tenantId, `/nodes/${nodeId}`),
    NODE_ROUTES: (tenantId, nodeId) => buildTenantPath(tenantId, `/nodes/${nodeId}/routes`),
    NODE_ROUTE: (tenantId, nodeId, routeId) => buildTenantPath(tenantId, `/nodes/${nodeId}/routes/${encodeURIComponent(routeId)}`),
    NODE_ACLS: (tenantId, nodeId) => buildTenantPath(tenantId, `/nodes/${nodeId}/security/acls`),
    NODE_ACL: (tenantId, nodeId, ruleId) => buildTenantPath(tenantId, `/nodes/${nodeId}/security/acls/${ruleId}`),
    NODE_QOS: (tenantId, nodeId, category) => buildTenantPath(tenantId, `/nodes/${nodeId}/qos/${category}`),
    NODE_QOS_RULE: (tenantId, nodeId, category, ruleId) => buildTenantPath(tenantId, `/nodes/${nodeId}/qos/${category}/${ruleId}`),
    AI_CHAT: (tenantId) => buildTenantPath(tenantId, '/ai/chat'),
    AI_CONFIRM: (tenantId) => buildTenantPath(tenantId, '/ai/confirm'),
    ROLES: (tenantId) => buildTenantPath(tenantId, '/roles'),
    ROLE_DETAIL: (tenantId, roleId) => buildTenantPath(tenantId, `/roles/${roleId}`)
  },

  // Agent 代理 API
  AGENT: {
    COMMAND: (tenantId, nodeId) => buildTenantPath(tenantId, `/nodes/${nodeId}/agent/command`),
    COMMANDS: (tenantId, nodeId) => buildTenantPath(tenantId, `/nodes/${nodeId}/agent/commands`),
    STATUS: (tenantId, nodeId) => buildTenantPath(tenantId, `/nodes/${nodeId}/agent/status`),
    BATCH_COMMAND: (tenantId) => buildTenantPath(tenantId, '/agents/command')
  },

  // 节点管理 API
  NODES: {
    LIST: (tenantId) => buildTenantPath(tenantId, '/nodes'),
    DETAIL: (tenantId, nodeId) => buildTenantPath(tenantId, `/nodes/${nodeId}`),
    SYNC: (tenantId, nodeId) => buildTenantPath(tenantId, `/nodes/${nodeId}/agent/sync`)
  },

  // Controller 管理 API（Agent 注册）
  CONTROLLER: {
    REGISTER: '/register',
    UNREGISTER: '/unregister',
    CONFIG: '/config'
  },

  // 带宽管理 API
  BANDWIDTH: {
    CATEGORY: (tenantId, nodeId, category) => buildTenantPath(tenantId, `/nodes/${nodeId}/qos/${category}`),
    RULE: (tenantId, nodeId, category, ruleId) => buildTenantPath(tenantId, `/nodes/${nodeId}/qos/${category}/${ruleId}`)
  },

  // 网络策略 API（旧版）
  POLICIES: {
    LIST: (tenantId) => buildTenantPath(tenantId, '/policies')
  },

  // 监控 API
  MONITOR: {
    STATS: (tenantId) => buildTenantPath(tenantId, '/monitoring/stats'),
    NODE_DETAIL: (tenantId, nodeId) => buildTenantPath(tenantId, `/monitoring/nodes/${nodeId}`),
    EVENTS: (tenantId) => buildTenantPath(tenantId, '/monitoring/events'),
    ALERTS: (tenantId) => buildTenantPath(tenantId, '/monitoring/alerts'),
    ALERT_RESOLVE: (tenantId, alertId) => buildTenantPath(tenantId, `/monitoring/alerts/${alertId}/resolve`),
    TRAFFIC: (tenantId) => buildTenantPath(tenantId, '/monitoring/traffic'),
    HEALTH: (tenantId) => buildTenantPath(tenantId, '/monitoring/health'),
    NODE_METRICS: (tenantId, nodeId) => buildTenantPath(tenantId, `/monitoring/nodes/${nodeId}/metrics`),
    TOPOLOGY: (tenantId) => buildTenantPath(tenantId, '/monitoring/topology')
  },

  // AI 聊天 API
  AI: {
    CHAT: (tenantId) => buildTenantPath(tenantId, '/ai/chat'),
    CONFIRM: (tenantId) => buildTenantPath(tenantId, '/ai/confirm')
  },

  // 即时通讯 Webhook
  IM: {
    DINGTALK: '/v2/integrations/dingtalk/webhook',
    FEISHU: '/v2/integrations/feishu/webhook'
  },

  // 健康检查
  HEALTH: '/health',

  // 版本
  VERSION: '/api/version'
};
