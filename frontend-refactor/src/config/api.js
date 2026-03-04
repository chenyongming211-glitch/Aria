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

// API endpoints - 与后端 v1 API 对接
export const API_ENDPOINTS = {
  // 认证相关
  AUTH: {
    LOGIN: '/v1/auth/login',
    REFRESH: '/v1/auth/refresh',
    LOGOUT: '/v1/auth/logout'
  },

  // 用户相关
  USER: {
    PROFILE: '/user/profile',
    SETTINGS: '/user/settings'
  },

  // 租户管理 API
  TENANT: {
    LIST: '/v1/tenants',
    CURRENT: '/v1/tenant-management',
    TENANTS: '/v1/system/tenants',
    NODES: '/v1/tenant-management/nodes',
    ACL_RULES: '/v1/tenant-management/acl-rules'
  },

  // ACL 规则管理 API
  ACL: {
    LIST: '/v1/tenant-management/acl-rules',
    CREATE: '/v1/tenant-management/acl-rules',
    GET: (id) => `/v1/tenant-management/acl-rules/${id}`,
    UPDATE: (id) => `/v1/tenant-management/acl-rules/${id}`,
    DELETE: (id) => `/v1/tenant-management/acl-rules/${id}`
  },

  // Agent 代理 API
  AGENT: {
    COMMAND: (nodeId) => `/v1/agent/${nodeId}/command`,
    STATUS: (nodeId) => `/v1/agent/${nodeId}/status`,
    BATCH_COMMAND: '/v1/agents/command'
  },

  // 节点管理 API
  NODES: {
    LIST: '/nodes',
    DETAIL: (id) => `/nodes/${id}`,
    SYNC: '/sync'
  },

  // Controller 管理 API（Agent 注册）
  CONTROLLER: {
    REGISTER: '/register',
    UNREGISTER: '/unregister',
    CONFIG: '/config',
    TOKENS: '/tokens'
  },

  // Token 管理 API（后端实际路径是 /tokens）
  TOKENS: {
    LIST: '/tokens',
    CREATE: '/tokens',
    DELETE: (id) => `/tokens/${id}`,
    DETAIL: '/tokens/detail',
    REVOKE: '/tokens/revoke'
  },

  // 带宽管理 API
  BANDWIDTH: {
    // 带宽限制
    LIMITS: {
      LIST: '/v1/bandwidth/limits',
      CREATE: '/v1/bandwidth/limits',
      DELETE: (id) => `/v1/bandwidth/limits/${id}`
    },
    // 策略管理
    POLICIES: {
      LIST: '/v1/bandwidth/policies',
      CREATE: '/v1/bandwidth/policies',
      GET: (id) => `/v1/bandwidth/policies/${id}`,
      UPDATE: (id) => `/v1/bandwidth/policies/${id}`,
      DELETE: (id) => `/v1/bandwidth/policies/${id}`
    }
  },

  // 网络策略 API（旧版）
  POLICIES: {
    LIST: '/policies',
    CREATE: '/policies',
    GET: (id) => `/policies/${id}`,
    UPDATE: (id) => `/policies/${id}`,
    DELETE: (id) => `/policies/${id}`
  },

  // 监控 API
  MONITOR: {
    STATS: '/v1/monitor/stats',
    NODE_DETAIL: (id) => `/v1/monitor/node/${id}`
  },

  // AI 聊天 API
  AI: {
    CHAT: '/v1/ai/chat',
    CONFIRM: '/v1/ai/confirm'
  },

  // 即时通讯 Webhook
  IM: {
    DINGTALK: '/v1/im/dingtalk',
    FEISHU: '/v1/im/feishu'
  },

  // 健康检查
  HEALTH: '/health',

  // 版本
  VERSION: '/version'
};