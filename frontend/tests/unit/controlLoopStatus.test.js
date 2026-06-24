import { describe, expect, it } from 'vitest'
import {
  commandStatusLabel,
  commandStatusTagType,
  isPendingCommandStatus,
  isRetryablePolicyStatus,
  mapCommandStatusToPolicyStatus,
  pendingCountForCommandStatus,
  policyStatusLabel,
  policyStatusTagType
} from '@/utils/controlLoopStatus'

describe('controlLoopStatus', () => {
  it('把 agent command 生命周期归一化为统一策略状态', () => {
    expect(mapCommandStatusToPolicyStatus('queued')).toBe('pending')
    expect(mapCommandStatusToPolicyStatus('pending')).toBe('pending')
    expect(mapCommandStatusToPolicyStatus('sent')).toBe('in_progress')
    expect(mapCommandStatusToPolicyStatus('acknowledged')).toBe('in_progress')
    expect(mapCommandStatusToPolicyStatus('completed')).toBe('applied')
    expect(mapCommandStatusToPolicyStatus('failed')).toBe('error')
    expect(mapCommandStatusToPolicyStatus('timeout')).toBe('error')
    expect(mapCommandStatusToPolicyStatus('stale')).toBe('stale')
    expect(mapCommandStatusToPolicyStatus('cancelled')).toBe('cancelled')
    expect(mapCommandStatusToPolicyStatus('')).toBe('')
  })

  it('只把未完成的 command 状态计入待执行', () => {
    for (const status of ['queued', 'pending', 'sent', 'acknowledged', 'in_progress', 'running']) {
      expect(isPendingCommandStatus(status)).toBe(true)
      expect(pendingCountForCommandStatus(status)).toBe(1)
    }

    for (const status of ['completed', 'failed', 'stale', 'cancelled', 'idle', '']) {
      expect(isPendingCommandStatus(status)).toBe(false)
      expect(pendingCountForCommandStatus(status)).toBe(0)
    }
  })

  it('统一返回策略状态和命令状态的中文展示', () => {
    expect(policyStatusLabel('pending')).toBe('待下发')
    expect(policyStatusLabel('in_progress')).toBe('下发中')
    expect(policyStatusLabel('applied')).toBe('已应用')
    expect(policyStatusLabel('error')).toBe('失败')
    expect(policyStatusLabel('stale')).toBe('已过期')
    expect(policyStatusTagType('error')).toBe('danger')

    expect(commandStatusLabel('acknowledged')).toBe('执行中')
    expect(commandStatusLabel('completed')).toBe('已完成')
    expect(commandStatusTagType('completed')).toBe('success')
  })

  it('只允许失败或过期的策略投递重试', () => {
    expect(isRetryablePolicyStatus('error')).toBe(true)
    expect(isRetryablePolicyStatus('failed')).toBe(true)
    expect(isRetryablePolicyStatus('stale')).toBe(true)
    expect(isRetryablePolicyStatus('pending')).toBe(false)
    expect(isRetryablePolicyStatus('applied')).toBe(false)
  })
})
