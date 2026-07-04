const toText = (value: unknown): string => String(value ?? '').trim()

export const isValidIPv4 = (value: unknown): boolean => {
  const text = toText(value)
  const parts = text.split('.')
  if (parts.length !== 4) return false

  return parts.every((part) => {
    if (!/^\d+$/.test(part)) return false
    if (part.length > 1 && part.startsWith('0')) return false
    const octet = Number(part)
    return Number.isInteger(octet) && octet >= 0 && octet <= 255
  })
}

export const isValidIPv6 = (value: unknown): boolean => {
  const text = toText(value)
  if (!text.includes(':') || text.includes('[') || text.includes(']')) {
    return false
  }
  try {
    new URL(`http://[${text}]/`)
    return true
  } catch {
    return false
  }
}

export const isValidIp = (value: unknown): boolean => {
  const text = toText(value)
  return text.includes(':') ? isValidIPv6(text) : isValidIPv4(text)
}

const isValidPrefix = (value: string, max: number): boolean => {
  if (!/^\d+$/.test(value)) return false
  const prefix = Number(value)
  return Number.isInteger(prefix) && prefix >= 0 && prefix <= max
}

export const isValidCidrOrIp = (value: unknown): boolean => {
  const text = toText(value)
  if (!text) return false

  const firstSlash = text.indexOf('/')
  if (firstSlash === -1) {
    return isValidIp(text)
  }
  if (firstSlash === 0 || firstSlash !== text.lastIndexOf('/') || firstSlash === text.length - 1) {
    return false
  }

  const ip = text.slice(0, firstSlash)
  const prefix = text.slice(firstSlash + 1)
  if (!isValidIp(ip)) return false

  return ip.includes(':')
    ? isValidPrefix(prefix, 128)
    : isValidPrefix(prefix, 32)
}
