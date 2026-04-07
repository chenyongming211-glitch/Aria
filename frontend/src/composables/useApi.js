// src/composables/useApi.js
import axios from 'axios'
import { API_BASE_URL, API_ENDPOINTS } from '@/config/api'

// Create axios instance
const api = axios.create({
  baseURL: API_BASE_URL, // Will be proxied to actual backend
  timeout: 10000,
  headers: {
    'Content-Type': 'application/json'
  }
})

// Token 刷新状态，防止多个请求同时刷新
let isRefreshing = false
let refreshSubscribers = []

// 订阅 token 刷新
function subscribeTokenRefresh(callback) {
  refreshSubscribers.push(callback)
}

// 通知所有订阅者 token 已刷新
function onTokenRefreshed(newToken) {
  refreshSubscribers.forEach(callback => callback(newToken))
  refreshSubscribers = []
}

// 通知所有订阅者 token 刷新失败
function onTokenRefreshFailed() {
  refreshSubscribers.forEach(callback => callback(null))
  refreshSubscribers = []
}

// 检查 token 是否快过期（剩余 < 10 分钟）
function isTokenExpiringSoon() {
  const tokenExpireTime = sessionStorage.getItem('aria_token_expire_time')
  if (!tokenExpireTime) return true
  
  const expireTime = parseInt(tokenExpireTime, 10)
  const now = Date.now()
  const tenMinutes = 10 * 60 * 1000
  
  return expireTime - now < tenMinutes
}

// 刷新 token
async function refreshToken() {
  const token = sessionStorage.getItem('aria_token')
  if (!token) return null
  
  try {
    const response = await axios.post(
      API_BASE_URL + API_ENDPOINTS.AUTH.REFRESH,
      {},
      {
        headers: {
          Authorization: `Bearer ${token}`,
          'Content-Type': 'application/json'
        }
      }
    )
    
    if (response.data && response.data.success && response.data.data && response.data.data.token) {
      const newToken = response.data.data.token
      // 保存新 token
      sessionStorage.setItem('aria_token', newToken)
      // 从响应中获取新的过期时间（如果没有则默认 2 小时）
      const expiresIn = response.data.data.expires_in || 7200
      const expireTime = Date.now() + expiresIn * 1000
      sessionStorage.setItem('aria_token_expire_time', expireTime.toString())
      
      return newToken
    }
    return null
  } catch (error) {
    console.error('Token refresh failed:', error)
    return null
  }
}

// 检查最大不活动时间（1 小时）
function checkMaxIdleTime() {
  const lastActivity = sessionStorage.getItem('aria_last_activity')
  if (!lastActivity) {
    updateLastActivity()
    return true
  }
  
  const maxIdleTime = 60 * 60 * 1000 // 1 小时
  const now = Date.now()
  
  if (now - parseInt(lastActivity, 10) > maxIdleTime) {
    console.warn('Max idle time exceeded, redirecting to login')
    return false
  }
  
  return true
}

// 更新最后活动时间
function updateLastActivity() {
  sessionStorage.setItem('aria_last_activity', Date.now().toString())
}

// Request interceptor to add auth and tenant headers
api.interceptors.request.use(
  async (config) => {
    // 检查最大不活动时间
    if (!checkMaxIdleTime()) {
      redirectToLogin()
      return config
    }
    
    // 检查 token 是否快过期，如果是则先刷新
    const token = sessionStorage.getItem('aria_token')
    if (token && isTokenExpiringSoon()) {
      if (!isRefreshing) {
        isRefreshing = true
        const newToken = await refreshToken()
        isRefreshing = false
        
        if (newToken) {
          onTokenRefreshed(newToken)
        } else {
          // 刷新失败，通知所有等待的请求，然后跳转登录
          onTokenRefreshFailed()
          redirectToLogin()
          return config
        }
      } else {
        // 正在刷新，等待刷新完成后更新 token
        return new Promise((resolve, reject) => {
          subscribeTokenRefresh((newToken) => {
            if (newToken) {
              // 刷新成功，使用新 token 继续请求
              config.headers.Authorization = `Bearer ${newToken}`
              resolve(config)
            } else {
              // 刷新失败，跳转登录并 reject Promise
              redirectToLogin()
              reject(new Error('Token refresh failed'))
            }
          })
        })
      }
    }
    
    // Add token to requests if available
    const currentToken = sessionStorage.getItem('aria_token')
    if (currentToken) {
      config.headers.Authorization = `Bearer ${currentToken}`
    }

    // Add tenant header
    const currentTenant = localStorage.getItem('aria-current-tenant')
    if (currentTenant) {
      const tenant = JSON.parse(currentTenant)
      config.headers['X-Tenant-ID'] = tenant.id
    }

    // 更新最后活动时间
    updateLastActivity()
    
    return config
  },
  (error) => Promise.reject(error)
)

// Response interceptor
api.interceptors.response.use(
  (response) => {
    // 后端返回统一格式: { success, data, message, error, code }
    // 如果响应是统一的 API 格式，只返回 data 部分
    const responseData = response.data
    if (responseData && typeof responseData === 'object' && 'success' in responseData) {
      // 如果成功，提取 data 字段；如果失败，保持原样让调用方处理
      if (responseData.success) {
        // 直接返回整个响应对象，包含 data 和元数据
        // 这样可以访问 i18n 等额外信息
        response.data = responseData
      }
    }
    return response
  },
  (error) => {
    if (error.response?.status === 401) {
      // Handle unauthorized - clear session and redirect to login
      const currentPath = window.location.hash
      if (currentPath !== '#/login') {
        console.warn('API returned 401, redirecting to login')
        redirectToLogin()
      }
    }
    return Promise.reject(error)
  }
)

// 跳转到登录页并清除会话
function redirectToLogin() {
  sessionStorage.removeItem('aria_token')
  sessionStorage.removeItem('aria_token_expire_time')
  sessionStorage.removeItem('aria_user')
  sessionStorage.removeItem('aria_last_activity')
  localStorage.removeItem('aria-current-tenant')
  window.location.href = '/#/login'
}

// 监听用户活动，更新最后活动时间
if (typeof window !== 'undefined') {
  const events = ['mousedown', 'keydown', 'scroll', 'touchstart']
  events.forEach(event => {
    document.addEventListener(event, updateLastActivity, { passive: true })
  })
}

export default api
