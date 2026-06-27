import type { ApiEnvelope } from '@/types'

interface ResponseLike<T> {
  data: ApiEnvelope<T> | T
}

function isEnvelope<T>(value: ApiEnvelope<T> | T): value is ApiEnvelope<T> {
  return Boolean(value && typeof value === 'object' && 'success' in value)
}

export function unwrapApiData<T>(response: ResponseLike<T>): T {
  const body = response.data
  if (isEnvelope<T>(body)) {
    return body.data as T
  }
  return body
}

export function unwrapApiList<T>(response: ResponseLike<T[] | { items?: T[] } | null | undefined>): T[] {
  const data = unwrapApiData<T[] | { items?: T[] } | null | undefined>(response)
  if (Array.isArray(data)) return data
  if (Array.isArray(data?.items)) return data.items
  return []
}
