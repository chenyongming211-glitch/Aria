// src/stores/user.js
import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '@/composables/useApi'
import { API_ENDPOINTS } from '@/config/api'

export default defineStore('user', () => {
  const user = ref(null)
  const isAuthenticated = ref(false)

  const login = async (credentials) => {
    try {
      // 调用真实的登录 API
      const response = await api.post(API_ENDPOINTS.AUTH.LOGIN, credentials)

      if (response.data && response.data.token) {
        // 存储 token 到 sessionStorage
        sessionStorage.setItem('aria_token', response.data.token)

        // 设置用户数据
        user.value = response.data.user || {
          id: 'user-1',
          name: credentials.username,
          initials: credentials.username.substring(0, 2).toUpperCase(),
          role: 'admin'
        }

        isAuthenticated.value = true

        // 存储用户会话
        sessionStorage.setItem('aria_user', JSON.stringify(user.value))

        // 存储租户 ID 到 localStorage（用于 API 请求）
        if (response.data.user?.tenant_id) {
          localStorage.setItem('aria-current-tenant', JSON.stringify({
            id: response.data.user.tenant_id
          }))
        }

        return { success: true, token: response.data.token }
      } else {
        throw new Error('Invalid login response')
      }
    } catch (error) {
      console.error('Login error:', error)
      
      // 如果是认证错误，返回错误消息
      if (error.response?.status === 401) {
        return {
          success: false,
          message: error.response?.data?.message || 'Invalid username or password'
        }
      }
      
      // 其他错误
      return {
        success: false,
        message: error.response?.data?.message || error.message || 'Login failed'
      }
    }
  }

  const logout = () => {
    user.value = null
    isAuthenticated.value = false
    sessionStorage.removeItem('aria_token')
    sessionStorage.removeItem('aria_user')
    localStorage.removeItem('aria-current-tenant')
  }

  const loadSession = () => {
    const token = sessionStorage.getItem('aria_token')
    const userData = sessionStorage.getItem('aria_user')

    if (token && userData) {
      user.value = JSON.parse(userData)
      isAuthenticated.value = true
    }
  }

  // 加载租户列表
  const loadTenants = async () => {
    try {
      const response = await api.get(API_ENDPOINTS.TENANT.LIST)
      return response.data?.data || response.data || []
    } catch (error) {
      console.error('Load tenants error:', error)
      return []
    }
  }

  // 刷新 token
  const refreshToken = async () => {
    try {
      const response = await api.post(API_ENDPOINTS.AUTH.REFRESH)
      if (response.data && response.data.token) {
        sessionStorage.setItem('aria_token', response.data.token)
        return { success: true }
      }
      return { success: false }
    } catch (error) {
      console.error('Token refresh error:', error)
      return { success: false }
    }
  }

  return {
    user,
    isAuthenticated,
    login,
    logout,
    loadSession,
    loadTenants,
    refreshToken
  }
})
