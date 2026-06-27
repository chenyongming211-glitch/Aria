// src/stores/user.ts
import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '@/composables/useApi'
import { API_ENDPOINTS } from '@/config/api'
import { clearSession } from '@/utils/session'
import type { Permission, RoleName, UserProfile } from '@/types'

type BuiltInRole = 'admin' | 'operator' | 'viewer'
type RawUserProfile = Partial<UserProfile> & Record<string, any>

interface JwtClaims {
  uid?: string
  sub?: string
  unm?: string
  rol?: RoleName | string
  tid?: string
  mcp?: boolean
}

interface HttpErrorLike {
  response?: {
    status?: number
    data?: {
      message?: string
    }
  }
  message?: string
}

const BUILT_IN_ROLES: BuiltInRole[] = ['admin', 'operator', 'viewer']

const isBuiltInRole = (role: string): role is BuiltInRole => BUILT_IN_ROLES.includes(role as BuiltInRole)

const asHttpError = (error: unknown): HttpErrorLike => error as HttpErrorLike

export const SYSTEM_ROLE_PERMISSIONS: Readonly<Record<BuiltInRole, readonly Permission[]>> = Object.freeze({
  admin: Object.freeze([
    'nodes:read', 'nodes:write',
    'routes:read', 'routes:write',
    'acls:read', 'acls:write',
    'ip-groups:read', 'ip-groups:write',
    'qos:read', 'qos:write',
    'blacklist:read', 'blacklist:write',
    'monitoring:read',
    'commands:write',
    'tokens:read', 'tokens:write',
    'users:read', 'users:write',
    'roles:read', 'roles:write',
    'ai:use',
    'policies:read',
    'settings:read', 'settings:write'
  ] as Permission[]),
  operator: Object.freeze([
    'nodes:read', 'nodes:write',
    'routes:read', 'routes:write',
    'acls:read', 'acls:write',
    'ip-groups:read', 'ip-groups:write',
    'qos:read', 'qos:write',
    'blacklist:read', 'blacklist:write',
    'monitoring:read',
    'commands:write',
    'ai:use',
    'policies:read'
  ] as Permission[]),
  viewer: Object.freeze([
    'nodes:read',
    'routes:read',
    'acls:read',
    'ip-groups:read',
    'qos:read',
    'blacklist:read',
    'monitoring:read',
    'ai:use',
    'policies:read'
  ] as Permission[])
})

export const normalizeRoleName = (role: unknown): RoleName | string => {
  const roleName = String(role || '').trim()
  const lowerRoleName = roleName.toLowerCase()
  if (lowerRoleName === 'member') return 'operator'
  if (lowerRoleName === 'owner') return 'admin'
  if (['admin', 'operator', 'viewer', 'super_admin'].includes(lowerRoleName)) return lowerRoleName
  return roleName
}

export const permissionsForRole = (role: unknown): Permission[] => {
  const roleName = normalizeRoleName(role)
  if (roleName === 'super_admin') return ['*']
  return isBuiltInRole(roleName) ? [...SYSTEM_ROLE_PERMISSIONS[roleName]] : []
}

const deriveInitials = (value: unknown): string => {
  const text = String(value || '').trim()
  if (!text) return ''
  const parts = text.split(/\s+/).filter(Boolean)
  if (parts.length >= 2) {
    return `${Array.from(parts[0])[0] || ''}${Array.from(parts[1])[0] || ''}`.toUpperCase()
  }
  return Array.from(text).slice(0, 2).join('').toUpperCase()
}

const normalizeUser = (rawUser?: RawUserProfile | null, fallbackUsername = ''): UserProfile => {
  const source = rawUser || {}
  const username = source.username || source.name || fallbackUsername
  const name = source.name || username
  return {
    ...source,
    username,
    name,
    initials: source.initials || deriveInitials(name || username)
  }
}

const decodeJwtPayload = (token: string | null | undefined): JwtClaims | null => {
  const payload = String(token || '').split('.')[1]
  if (!payload) return null

  try {
    const normalized = payload.replace(/-/g, '+').replace(/_/g, '/')
    const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '=')
    const binary = atob(padded)
    const bytes = Uint8Array.from(binary, char => char.charCodeAt(0))
    return JSON.parse(new TextDecoder().decode(bytes))
  } catch (error) {
    console.warn('Failed to decode cached token claims:', error)
    return null
  }
}

const userFromTokenClaims = (token: string | null | undefined): UserProfile | null => {
  const claims = decodeJwtPayload(token)
  if (!claims) return null

  return normalizeUser({
    id: claims.uid || claims.sub,
    username: claims.unm,
    role: claims.rol,
    tenant_id: claims.tid || ''
  })
}

export const tokenRequiresPasswordChange = (token: string | null | undefined): boolean => {
  const claims = decodeJwtPayload(token)
  return Boolean(claims?.mcp)
}

const mergeTokenUserClaims = (cachedUser: RawUserProfile | null | undefined, token: string): UserProfile => {
  const tokenUser = userFromTokenClaims(token)
  if (!tokenUser) return normalizeUser(cachedUser)

  return normalizeUser({
    ...cachedUser,
    id: tokenUser.id || cachedUser?.id,
    username: tokenUser.username || cachedUser?.username,
    role: tokenUser.role || cachedUser?.role,
    tenant_id: tokenUser.tenant_id ?? cachedUser?.tenant_id
  })
}

export default defineStore('user', () => {
  const user = ref<UserProfile | null>(null)
  const isAuthenticated = ref(false)
  const mustChangePassword = ref(false)
  const permissions = ref<Permission[]>([])

  const resetSessionState = () => {
    user.value = null
    isAuthenticated.value = false
    mustChangePassword.value = false
    permissions.value = []
  }

  if (typeof window !== 'undefined') {
    window.addEventListener('aria-session-cleared', resetSessionState)
  }

  const setPermissions = (nextPermissions?: unknown): Permission[] => {
    const list = Array.isArray(nextPermissions) ? nextPermissions : []
    const normalized = Array.from(new Set(list.filter(Boolean))) as Permission[]
    permissions.value = normalized
    localStorage.setItem('aria_permissions', JSON.stringify(permissions.value))
    return permissions.value
  }

  const persistCurrentTenant = (tenantId?: string | null) => {
    if (tenantId) {
      localStorage.setItem('aria-current-tenant', JSON.stringify({ id: tenantId }))
      return
    }
    localStorage.removeItem('aria-current-tenant')
  }

  const readCachedPermissions = (): Permission[] | null | undefined => {
    const cached = localStorage.getItem('aria_permissions')
    if (!cached) return undefined

    try {
      const parsed = JSON.parse(cached)
      if (Array.isArray(parsed)) return parsed as Permission[]
      localStorage.removeItem('aria_permissions')
      return null
    } catch (error) {
      console.warn('Invalid cached permissions, clearing them:', error)
      localStorage.removeItem('aria_permissions')
      return null
    }
  }

  const login = async (credentials: { username?: string; password?: string; [key: string]: unknown }) => {
    try {
      // 调用真实的登录 API
      const response = await api.post(API_ENDPOINTS.AUTH.LOGIN, credentials)

      // 后端返回格式: { success, data: { token, user, require_password_change }, message, code }
      const token = response.data?.data?.token || response.data?.token
      const userData = response.data?.data?.user || response.data?.user
      const requirePasswordChange = response.data?.data?.require_password_change || response.data?.require_password_change || false

      if (token) {
        const tokenUser = userFromTokenClaims(token)
        if (!userData && !tokenUser?.role) {
          throw new Error('Invalid login response: no user found')
        }

        // 存储 token 到 localStorage
        localStorage.setItem('aria_token', token)
        
        // 计算并存储 token 过期时间（默认 2 小时 = 7200 秒）
        const expiresIn = response.data?.data?.expires_in || 7200
        const expireTime = Date.now() + expiresIn * 1000
        localStorage.setItem('aria_token_expire_time', expireTime.toString())
        
        // 初始化最后活动时间
        localStorage.setItem('aria_last_activity', Date.now().toString())

        // 设置用户数据
        user.value = normalizeUser(userData || tokenUser, credentials.username)

        isAuthenticated.value = true
        mustChangePassword.value = requirePasswordChange || tokenRequiresPasswordChange(token)
        if (mustChangePassword.value) {
          localStorage.setItem('aria_must_change_password', 'true')
        } else {
          localStorage.removeItem('aria_must_change_password')
        }

        // 存储用户会话
        localStorage.setItem('aria_user', JSON.stringify(user.value))
        persistCurrentTenant(user.value?.tenant_id)

        // 加载用户权限
        if (user.value?.role === 'super_admin') {
          await loadPermissions(null, user.value.role)
        } else if (user.value?.tenant_id) {
          await loadPermissions(user.value.tenant_id, user.value.role)
        } else {
          setPermissions([])
        }

        return { success: true, token, requirePasswordChange }
      } else {
        throw new Error('Invalid login response: no token found')
      }
    } catch (error) {
      const httpError = asHttpError(error)
      console.error('[Login] Error:', error)
      
      // 如果是认证错误，返回错误消息
      if (httpError.response?.status === 401) {
        return {
          success: false,
          message: httpError.response?.data?.message || 'Invalid username or password'
        }
      }
      
      // 其他错误
      return {
        success: false,
        message: httpError.response?.data?.message || httpError.message || 'Login failed'
      }
    }
  }

  const changePassword = async (oldPassword: string, newPassword: string) => {
    try {
      const response = await api.post(API_ENDPOINTS.AUTH.FORCE_CHANGE_PASSWORD, {
        old_password: oldPassword,
        new_password: newPassword
      })
      
      console.log('[ChangePassword] Response:', response.data)
      
      if (response.data?.success) {
        mustChangePassword.value = false
        localStorage.removeItem('aria_must_change_password')
        return { success: true }
      } else {
        return { success: false, message: response.data?.message || 'Password change failed' }
      }
    } catch (error) {
      const httpError = asHttpError(error)
      console.error('[ChangePassword] Error:', error)
      return { 
        success: false, 
        message: httpError.response?.data?.message || httpError.message || 'Password change failed'
      }
    }
  }

  const logout = () => {
    clearSession()
    resetSessionState()
  }

  const loadSession = () => {
    const token = localStorage.getItem('aria_token')
    const userData = localStorage.getItem('aria_user')

    if (!token && !userData) {
      return false
    }

    if (!token || !userData) {
      logout()
      return false
    }

    try {
      user.value = mergeTokenUserClaims(JSON.parse(userData), token)
    } catch (error) {
      console.warn('Invalid cached user session, clearing it:', error)
      logout()
      return false
    }

    isAuthenticated.value = true
    if (!localStorage.getItem('aria_last_activity')) {
      localStorage.setItem('aria_last_activity', Date.now().toString())
    }
    mustChangePassword.value = localStorage.getItem('aria_must_change_password') === 'true' || tokenRequiresPasswordChange(token)
    if (mustChangePassword.value) {
      localStorage.setItem('aria_must_change_password', 'true')
    } else {
      localStorage.removeItem('aria_must_change_password')
    }
    localStorage.setItem('aria_user', JSON.stringify(user.value))
    if (user.value?.tenant_id) {
      persistCurrentTenant(user.value.tenant_id)
    }

    const cached = readCachedPermissions()
    if (Array.isArray(cached)) {
      permissions.value = cached
    } else {
      permissions.value = []
    }

    if (user.value?.role === 'super_admin' && !permissions.value.includes('*')) {
      setPermissions(['*'])
    }

    if (user.value?.role !== 'super_admin' && user.value?.tenant_id) {
      loadCurrentPermissions().catch((error) => {
        console.warn('Failed to refresh session permissions:', error)
      })
    }

    return true
  }

  // 加载租户列表
  const loadTenants = async (): Promise<unknown[]> => {
    try {
      const response = await api.get(API_ENDPOINTS.TENANT.LIST)
      return response.data?.data || response.data || []
    } catch (error) {
      console.error('Load tenants error:', error)
      return []
    }
  }

  const loadCurrentPermissions = async (): Promise<Permission[]> => {
    const response = await api.get(API_ENDPOINTS.AUTH.PERMISSIONS)
    const data = response.data?.data || response.data || {}
    if (Array.isArray(data.permissions)) {
      return setPermissions(data.permissions)
    }
    return setPermissions([])
  }

  // 加载用户权限：普通租户用户只接受后端当前上下文返回的权限，失败时按空权限处理。
  const loadPermissions = async (_tenantId: string | null | undefined, role: unknown): Promise<Permission[]> => {
    const roleName = normalizeRoleName(role)
    if (roleName === 'super_admin') {
      return setPermissions(['*'])
    }

    try {
      return await loadCurrentPermissions()
    } catch (error) {
      console.error('Load permissions error:', error)
      return setPermissions([])
    }
  }

  // 刷新 token
  const refreshToken = async (): Promise<{ success: boolean }> => {
    try {
      const response = await api.post(API_ENDPOINTS.AUTH.REFRESH)
      const token = response.data?.data?.token || response.data?.token
      if (token) {
        const responseData = response.data?.data || response.data || {}
        const expiresIn = responseData.expires_in || 7200
        const refreshedUser = responseData.user || userFromTokenClaims(token)
        const requirePasswordChange = responseData.require_password_change ?? tokenRequiresPasswordChange(token)

        localStorage.setItem('aria_token', token)
        localStorage.setItem('aria_token_expire_time', (Date.now() + expiresIn * 1000).toString())

        if (refreshedUser) {
          user.value = normalizeUser(refreshedUser, user.value?.username)
          localStorage.setItem('aria_user', JSON.stringify(user.value))
          persistCurrentTenant(user.value?.tenant_id)
        }

        mustChangePassword.value = Boolean(requirePasswordChange)
        if (mustChangePassword.value) {
          localStorage.setItem('aria_must_change_password', 'true')
        } else {
          localStorage.removeItem('aria_must_change_password')
        }
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
    permissions,
    login,
    logout,
    changePassword,
    loadSession,
    loadTenants,
    loadPermissions,
    refreshToken
  }
})
