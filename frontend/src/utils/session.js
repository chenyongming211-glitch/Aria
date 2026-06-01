export const SESSION_IDLE_TIMEOUT_MS = 60 * 60 * 1000
export const SESSION_IDLE_CHECK_INTERVAL_MS = 60 * 1000

const SESSION_STORAGE_KEYS = [
  'aria_token',
  'aria_token_expire_time',
  'aria_user',
  'aria_last_activity',
  'aria-current-tenant',
  'aria_permissions'
]

export function clearSession() {
  SESSION_STORAGE_KEYS.forEach((key) => localStorage.removeItem(key))
}

export function readLastActivity() {
  const raw = localStorage.getItem('aria_last_activity')
  if (!raw) return null

  const value = parseInt(raw, 10)
  if (Number.isNaN(value)) return null

  return value
}

export function hasActiveSession() {
  return Boolean(localStorage.getItem('aria_token'))
}

export function isIdleSessionExpired(now = Date.now()) {
  if (!hasActiveSession()) return false

  const lastActivity = readLastActivity()
  if (!lastActivity) return true

  return now - lastActivity >= SESSION_IDLE_TIMEOUT_MS
}

export function recordUserActivity(now = Date.now()) {
  if (!hasActiveSession()) return
  localStorage.setItem('aria_last_activity', now.toString())
}

export function startIdleSessionMonitor(onExpired, intervalMs = SESSION_IDLE_CHECK_INTERVAL_MS) {
  if (typeof window === 'undefined') return null

  const timer = window.setInterval(() => {
    if (isIdleSessionExpired()) {
      onExpired()
    }
  }, intervalMs)

  if (typeof timer?.unref === 'function') {
    timer.unref()
  }

  return timer
}
