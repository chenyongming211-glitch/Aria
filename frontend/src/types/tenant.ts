import type { ISODateTimeString, TenantId } from './api'

export type TenantStatus = 'active' | 'disabled' | 'deleted' | string

export interface Tenant {
  id: TenantId
  name?: string
  code?: string
  status?: TenantStatus
  created_at?: ISODateTimeString
  updated_at?: ISODateTimeString
}

