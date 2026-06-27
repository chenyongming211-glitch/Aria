import { describe, expect, it } from 'vitest'
import { unwrapApiData, unwrapApiList } from '@/composables/apiResponse'

describe('apiResponse helpers', () => {
  it('unwraps unified API envelope data', () => {
    expect(unwrapApiData({ data: { success: true, data: { id: 'n1' } } })).toEqual({ id: 'n1' })
  })

  it('returns raw response data when no envelope exists', () => {
    expect(unwrapApiData({ data: { id: 'raw' } })).toEqual({ id: 'raw' })
  })

  it('unwraps array list responses', () => {
    expect(unwrapApiList({ data: { success: true, data: [{ id: 'a' }] } })).toEqual([{ id: 'a' }])
  })

  it('unwraps items list responses', () => {
    expect(unwrapApiList({ data: { success: true, data: { items: [{ id: 'a' }] } } })).toEqual([{ id: 'a' }])
  })

  it('returns empty list for null list payloads', () => {
    expect(unwrapApiList({ data: { success: true, data: null } })).toEqual([])
  })
})
