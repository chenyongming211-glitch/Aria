import type { TenantId } from './api'
import type { Permission, RoleName } from './permission'

export interface UserProfile {
  id?: string
  username: string
  name?: string
  role?: RoleName | string
  tenant_id?: TenantId
  initials?: string
}

export interface LoginResponse {
  token: string
  expires_in?: number
  user?: UserProfile
  permissions?: Permission[]
  must_change_password?: boolean
}
