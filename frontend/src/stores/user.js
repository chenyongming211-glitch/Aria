// src/stores/user.js
import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '@/composables/useApi'
import { API_ENDPOINTS } from '@/config/api'

export default defineStore('user', () => {
  const user = ref(null)
  const isAuthenticated = ref(false)
  const mustChangePassword = ref(false)

  const login = async (credentials) => {
    try {
      // 调用真实的登录 API
      const response = await api.post(API_ENDPOINTS.AUTH.LOGIN, credentials)
      
      console.log('[Login] Response:', response.data)

      // 后端返回格式: { success, data: { token, user, require_password_change }, message, code }
      const token = response.data?.data?.token || response.data?.token
      const userData = response.data?.data?.user || response.data?.user
      const requirePasswordChange = response.data?.data?.require_password_change || response.data?.require_password_change || false

      if (token) {
        // 存储 token 到 localStorage
        localStorage.setItem('aria_token', token)
        
        // 计算并存储 token 过期时间（默认 2 小时 = 7200 秒）
        const expiresIn = response.data?.data?.expires_in || 7200
        const expireTime = Date.now() + expiresIn * 1000
        localStorage.setItem('aria_token_expire_time', expireTime.toString())
        
        // 初始化最后活动时间
        localStorage.setItem('aria_last_activity', Date.now().toString())

        // 设置用户数据
        user.value = userData || {
          id: 'user-1',
          name: credentials.username,
          initials: credentials.username.substring(0, 2).toUpperCase(),
          role: 'admin'
        }

        isAuthenticated.value = true
        mustChangePassword.value = requirePasswordChange

        // 存储用户会话
        localStorage.setItem('aria_user', JSON.stringify(user.value))

        // 存储租户 ID 到 localStorage（用于 API 请求）
        if (userData?.tenant_id) {
          localStorage.setItem('aria-current-tenant', JSON.stringify({
            id: userData.tenant_id
          }))
        }

        return { success: true, token, requirePasswordChange }
      } else {
        throw new Error('Invalid login response: no token found')
      }
    } catch (error) {
      console.error('[Login] Error:', error)
      
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

  const changePassword = async (oldPassword, newPassword) => {
    try {
      const response = await api.post(API_ENDPOINTS.AUTH.FORCE_CHANGE_PASSWORD, {
        old_password: oldPassword,
        new_password: newPassword
      })
      
      console.log('[ChangePassword] Response:', response.data)
      
      if (response.data?.success) {
        mustChangePassword.value = false
        return { success: true }
      } else {
        return { success: false, message: response.data?.message || 'Password change failed' }
      }
    } catch (error) {
      console.error('[ChangePassword] Error:', error)
      return { 
        success: false, 
        message: error.response?.data?.message || error.message || 'Password change failed' 
      }
    }
  }

  const logout = () => {
    user.value = null
    isAuthenticated.value = false
    localStorage.removeItem('aria_token')
    localStorage.removeItem('aria_token_expire_time')
    localStorage.removeItem('aria_last_activity')
    localStorage.removeItem('aria_user')
    localStorage.removeItem('aria-current-tenant')
  }

  const loadSession = () => {
    const token = localStorage.getItem('aria_token')
    const userData = localStorage.getItem('aria_user')

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
      const token = response.data?.data?.token || response.data?.token
      if (token) {
        localStorage.setItem('aria_token', token)
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
    mustChangePassword,
    login,
    logout,
    changePassword,
    loadSession,
    loadTenants,
    refreshToken
  }
})
