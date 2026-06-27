const pendingCommandStatuses = new Set([
  'queued',
  'pending',
  'sent',
  'acknowledged',
  'in_progress',
  'running'
])

const failedCommandStatuses = new Set(['failed', 'error', 'timeout', 'timed_out'])

const terminalCommandStatuses = new Set([
  'completed',
  'applied',
  'stale',
  'cancelled',
  'canceled',
  'idle',
  ...failedCommandStatuses
])

const retryablePolicyStatuses = new Set(['error', 'failed', 'stale', 'timeout', 'timed_out'])

const normalizeStatus = (status) => String(status || '').trim().toLowerCase()

export function mapCommandStatusToPolicyStatus(status) {
  const normalized = normalizeStatus(status)
  if (['queued', 'pending'].includes(normalized)) return 'pending'
  if (['sent', 'acknowledged', 'in_progress', 'running'].includes(normalized)) return 'in_progress'
  if (['completed', 'applied'].includes(normalized)) return 'applied'
  if (['failed', 'error', 'timeout', 'timed_out'].includes(normalized)) return 'error'
  if (normalized === 'stale') return 'stale'
  if (['cancelled', 'canceled'].includes(normalized)) return 'cancelled'
  if (normalized === 'idle') return 'idle'
  return normalized ? '' : ''
}

export function isPendingCommandStatus(status) {
  return pendingCommandStatuses.has(normalizeStatus(status))
}

export function isFailedCommandStatus(status) {
  return failedCommandStatuses.has(normalizeStatus(status))
}

export function isTerminalCommandStatus(status) {
  return terminalCommandStatuses.has(normalizeStatus(status))
}

export function pendingCountForCommandStatus(status) {
  return isPendingCommandStatus(status) ? 1 : 0
}

export function isRetryablePolicyStatus(status) {
  return retryablePolicyStatuses.has(normalizeStatus(status))
}

export function policyStatusLabel(status) {
  const labels = {
    accepted: '已接受',
    applied: '已应用',
    canceled: '已取消',
    cancelled: '已取消',
    error: '失败',
    failed: '失败',
    healthy: 'Healthy',
    idle: '空闲',
    in_progress: '下发中',
    pending: '待下发',
    stale: '已过期'
  }
  const normalized = normalizeStatus(status)
  return labels[normalized] || status || '未知'
}

export function policyStatusTagType(status) {
  switch (normalizeStatus(status)) {
    case 'applied':
    case 'healthy':
      return 'success'
    case 'accepted':
    case 'pending':
    case 'in_progress':
      return 'warning'
    case 'stale':
    case 'cancelled':
    case 'idle':
      return 'info'
    case 'error':
    case 'failed':
      return 'danger'
    default:
      return 'info'
  }
}

export function commandStatusLabel(status) {
  const labels = {
    acknowledged: '执行中',
    applied: '已应用',
    canceled: '已取消',
    cancelled: '已取消',
    completed: '已完成',
    error: '失败',
    failed: '失败',
    idle: '空闲',
    in_progress: '执行中',
    pending: '待下发',
    queued: '排队中',
    running: '执行中',
    sent: '已发送',
    stale: '已过期',
    timeout: '超时',
    timed_out: '超时'
  }
  const normalized = normalizeStatus(status)
  return labels[normalized] || status || '未知'
}

export function commandStatusTagType(status) {
  const normalized = normalizeStatus(status)
  if (['completed', 'applied'].includes(normalized)) return 'success'
  if (pendingCommandStatuses.has(normalized)) return 'warning'
  if (['stale', 'cancelled', 'canceled', 'idle'].includes(normalized)) return 'info'
  if (['failed', 'error', 'timeout', 'timed_out'].includes(normalized)) return 'danger'
  return 'info'
}
