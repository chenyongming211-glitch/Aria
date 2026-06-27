// src/composables/useApi.ts
import axios, { AxiosHeaders } from 'axios'
import type { InternalAxiosRequestConfig } from 'axios'
import { API_BASE_URL, API_ENDPOINTS } from '@/config/api'
import {
  clearSession,
  isIdleSessionExpired,
  recordUserActivity,
  startIdleSessionMonitor
} from '@/utils/session'

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
let refreshSubscribers: Array<(newToken: string | null) => void> = []

type HeaderMap = Record<string, string>

function setHeader(config: InternalAxiosRequestConfig, name: string, value: string) {
  if (!config.headers) {
    config.headers = new AxiosHeaders()
  }

  if (typeof config.headers.set === 'function') {
    config.headers.set(name, value)
    return
  }

  ;(config.headers as unknown as HeaderMap)[name] = value
}

function deleteHeader(config: InternalAxiosRequestConfig, name: string) {
  if (!config.headers) return

  if (typeof config.headers.delete === 'function') {
    config.headers.delete(name)
    return
  }

  delete (config.headers as unknown as HeaderMap)[name]
}

// 订阅 token 刷新
function subscribeTokenRefresh(callback: (newToken: string | null) => void) {
  refreshSubscribers.push(callback)
}

// 通知所有订阅者 token 已刷新
function onTokenRefreshed(newToken: string) {
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
  const tokenExpireTime = localStorage.getItem('aria_token_expire_time')
  if (!tokenExpireTime) return false
  
  const expireTime = parseInt(tokenExpireTime, 10)
  if (Number.isNaN(expireTime)) return true
  const now = Date.now()
  const tenMinutes = 10 * 60 * 1000
  
  return expireTime - now < tenMinutes
}

function applyAuthHeaders(config: InternalAxiosRequestConfig, tokenOverride: string | null = null): InternalAxiosRequestConfig {
  const token = tokenOverride || localStorage.getItem('aria_token')
  if (token) {
    setHeader(config, 'Authorization', `Bearer ${token}`)
  }

  const currentTenant = localStorage.getItem('aria-current-tenant')
  if (currentTenant) {
    try {
      const tenant = JSON.parse(currentTenant) as { id?: unknown }
      if (tenant?.id && typeof tenant.id === 'string') {
        setHeader(config, 'X-Tenant-ID', tenant.id)
      } else {
        localStorage.removeItem('aria-current-tenant')
        deleteHeader(config, 'X-Tenant-ID')
      }
    } catch (error) {
      console.warn('Invalid aria-current-tenant in localStorage, clearing it:', error)
      localStorage.removeItem('aria-current-tenant')
      deleteHeader(config, 'X-Tenant-ID')
    }
  }

  return config
}

// 刷新 token
async function refreshToken() {
  const token = localStorage.getItem('aria_token')
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
      localStorage.setItem('aria_token', newToken)
      // 从响应中获取新的过期时间（如果没有则默认 2 小时）
      const expiresIn = response.data.data.expires_in || 7200
      const expireTime = Date.now() + expiresIn * 1000
      localStorage.setItem('aria_token_expire_time', expireTime.toString())
      
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
  if (isIdleSessionExpired()) {
    console.warn('Max idle time exceeded, redirecting to login')
    return false
  }

  return true
}

// Request interceptor to add auth and tenant headers
api.interceptors.request.use(
  async (config) => {
    // 检查最大不活动时间
    if (!checkMaxIdleTime()) {
      redirectToLogin()
      return Promise.reject(new Error('Session expired'))
    }
    
    // 检查 token 是否快过期，如果是则先刷新
    const token = localStorage.getItem('aria_token')
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
          return Promise.reject(new Error('Token refresh failed'))
        }
      } else {
        // 正在刷新，等待刷新完成后更新 token
        return new Promise((resolve, reject) => {
          subscribeTokenRefresh((newToken) => {
            if (newToken) {
              resolve(applyAuthHeaders(config, newToken))
            } else {
              // 刷新失败，跳转登录并 reject Promise
              redirectToLogin()
              reject(new Error('Token refresh failed'))
            }
          })
        })
      }
    }
    
    return applyAuthHeaders(config)
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
  clearSession()
  window.location.href = '/#/login'
}

function handleUserActivity() {
  if (!localStorage.getItem('aria_token')) return

  if (!checkMaxIdleTime()) {
    redirectToLogin()
    return
  }

  recordUserActivity()
}

// 监听用户活动，更新最后活动时间
if (typeof window !== 'undefined') {
  const events = ['mousedown', 'keydown', 'scroll', 'touchstart']
  events.forEach(event => {
    document.addEventListener(event, handleUserActivity, { passive: true })
  })
  startIdleSessionMonitor(redirectToLogin)
}

export default api
