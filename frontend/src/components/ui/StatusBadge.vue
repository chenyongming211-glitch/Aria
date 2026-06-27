<template>
  <span class="ui-status-badge" :class="badgeClass">
    <span class="ui-status-badge__dot" aria-hidden="true" />
    <span class="ui-status-badge__label">{{ resolvedLabel }}</span>
  </span>
</template>

<script setup>
import { computed } from 'vue'
import { t } from '@/i18n'

const props = defineProps({
  domain: {
    type: String,
    default: ''
  },
  status: {
    type: String,
    default: ''
  },
  label: {
    type: String,
    default: ''
  },
  labelKey: {
    type: String,
    default: ''
  },
  tone: {
    type: String,
    default: ''
  }
})

const statusTone = {
  accepted: 'info',
  acknowledged: 'warning',
  applied: 'success',
  completed: 'success',
  healthy: 'success',
  online: 'success',
  pending: 'info',
  queued: 'info',
  running: 'warning',
  sent: 'warning',
  in_progress: 'warning',
  stale: 'warning',
  warning: 'warning',
  canceled: 'muted',
  cancelled: 'muted',
  idle: 'muted',
  offline: 'muted',
  failed: 'danger',
  error: 'danger',
  timeout: 'danger',
  timed_out: 'danger',
  critical: 'danger'
}

const normalizedStatus = computed(() => String(props.status || 'unknown').toLowerCase())
const resolvedTone = computed(() => props.tone || statusTone[normalizedStatus.value] || 'muted')
const badgeClass = computed(() => `ui-status-badge--${resolvedTone.value}`)

const resolvedLabel = computed(() => {
  if (props.labelKey) return t(props.labelKey)
  if (props.label) return props.label
  if (props.domain && props.status) {
    return t(`status.${props.domain}.${normalizedStatus.value}`)
  }
  if (props.status) return t(`status.${normalizedStatus.value}`)
  return t('status.unknown')
})
</script>

<style scoped>
.ui-status-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-height: 24px;
  padding: 3px 9px;
  border: 1px solid var(--aria-border-primary);
  border-radius: var(--radius-full);
  background: var(--aria-content-bg-tertiary);
  color: var(--aria-text-secondary);
  font-size: 12px;
  font-weight: 600;
  line-height: 1;
}

.ui-status-badge__dot {
  width: 6px;
  height: 6px;
  border-radius: var(--radius-full);
  background: currentColor;
}

.ui-status-badge--success {
  color: var(--aria-success-dark);
  background: rgba(34, 197, 94, 0.1);
  border-color: rgba(34, 197, 94, 0.18);
}

.ui-status-badge--warning {
  color: var(--aria-warning-dark);
  background: rgba(245, 158, 11, 0.1);
  border-color: rgba(245, 158, 11, 0.2);
}

.ui-status-badge--danger {
  color: var(--aria-danger-dark);
  background: rgba(239, 68, 68, 0.1);
  border-color: rgba(239, 68, 68, 0.18);
}

.ui-status-badge--info {
  color: var(--aria-primary-dark);
  background: rgba(37, 99, 235, 0.08);
  border-color: rgba(37, 99, 235, 0.18);
}
</style>
