export interface ApiEnvelope<T> {
  success: boolean
  data?: T
  message?: string
  error?: string
  code?: string
}

export interface ListResult<T> {
  items: T[]
  total?: number
  limit?: number
  offset?: number
}

export type ISODateTimeString = string
export type TenantId = string
export type NodeId = string
export type PolicyId = string
export type CommandId = string
export type AlertId = string

