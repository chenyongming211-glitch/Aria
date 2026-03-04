// src/composables/useApi.js
import axios from 'axios'
import { API_BASE_URL } from '@/config/api'

// Create axios instance
const api = axios.create({
  baseURL: API_BASE_URL, // Will be proxied to actual backend
  timeout: 10000,
  headers: {
    'Content-Type': 'application/json'
  }
})

// Request interceptor to add auth and tenant headers
api.interceptors.request.use(
  (config) => {
    // Add token to requests if available
    const token = sessionStorage.getItem('aria_token')
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }

    // Add tenant header
    const currentTenant = localStorage.getItem('aria-current-tenant')
    if (currentTenant) {
      const tenant = JSON.parse(currentTenant)
      config.headers['X-Tenant-ID'] = tenant.id
    }

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
      // 但如果当前已经在登录页，不要重复跳转
      const currentPath = window.location.hash
      if (currentPath !== '#/login') {
        console.warn('API returned 401, but staying on current page for debugging')
        // 临时注释掉自动登出功能，方便调试
        // sessionStorage.removeItem('aria_token')
        // sessionStorage.removeItem('aria_user')
        // localStorage.removeItem('aria-current-tenant')
        // window.location.href = '/#/login'
      }
    }
    return Promise.reject(error)
  }
)

export default api