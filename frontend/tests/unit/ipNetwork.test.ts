import { describe, expect, it } from 'vitest'

import { isValidCidrOrIp } from '@/utils/ipNetwork'

describe('ipNetwork utilities', () => {
  it('accepts valid IPv4 and IPv6 CIDR or bare IP values', () => {
    expect(isValidCidrOrIp('10.0.0.0/24')).toBe(true)
    expect(isValidCidrOrIp('0.0.0.0/0')).toBe(true)
    expect(isValidCidrOrIp('203.0.113.10')).toBe(true)
    expect(isValidCidrOrIp('2001:db8::/64')).toBe(true)
    expect(isValidCidrOrIp('2001:db8::1')).toBe(true)
  })

  it('rejects invalid octets, prefixes, and malformed networks', () => {
    expect(isValidCidrOrIp('999.999.999.999/99')).toBe(false)
    expect(isValidCidrOrIp('10.0.0.0/33')).toBe(false)
    expect(isValidCidrOrIp('2001:db8::/129')).toBe(false)
    expect(isValidCidrOrIp('10.0.0/24')).toBe(false)
    expect(isValidCidrOrIp('')).toBe(false)
  })
})
