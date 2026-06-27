<template>
  <div class="ui-metric-strip" role="list">
    <div
      v-for="metric in metrics"
      :key="metric.key || metric.label"
      class="ui-metric-strip__item"
      :class="statusClass(metric.status)"
      role="listitem"
    >
      <div class="ui-metric-strip__label-row">
        <span v-if="metric.status" class="ui-metric-strip__dot" aria-hidden="true" />
        <span class="ui-metric-strip__label">{{ metric.label }}</span>
      </div>
      <div class="ui-metric-strip__value">{{ metric.value }}</div>
      <div v-if="metric.meta" class="ui-metric-strip__meta">{{ metric.meta }}</div>
    </div>
  </div>
</template>

<script setup>
defineProps({
  metrics: {
    type: Array,
    default: () => []
  }
})

const allowedStatuses = new Set(['success', 'warning', 'danger', 'info', 'muted'])

const statusClass = (status) => {
  const normalized = String(status || 'muted').toLowerCase()
  const safeStatus = allowedStatuses.has(normalized) ? normalized : 'muted'
  return `ui-metric-strip__item--${safeStatus}`
}
</script>

<style scoped>
.ui-metric-strip {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 10px;
}

.ui-metric-strip__item {
  min-width: 0;
  min-height: 76px;
  padding: 12px 14px;
  background: var(--aria-content-bg-secondary);
  border: 1px solid var(--aria-border-primary);
  border-radius: var(--aria-radius);
}

.ui-metric-strip__label-row {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.ui-metric-strip__dot {
  width: 7px;
  height: 7px;
  border-radius: var(--radius-full);
  background: var(--aria-text-muted);
  flex: 0 0 auto;
}

.ui-metric-strip__label {
  min-width: 0;
  color: var(--aria-text-muted);
  font-size: 12px;
  font-weight: 600;
}

.ui-metric-strip__value {
  margin-top: 8px;
  color: var(--aria-text-primary);
  font-size: 22px;
  font-weight: 700;
  line-height: 1;
}

.ui-metric-strip__meta {
  margin-top: 6px;
  color: var(--aria-text-muted);
  font-size: 12px;
}

.ui-metric-strip__item--success .ui-metric-strip__dot {
  background: var(--aria-success);
}

.ui-metric-strip__item--warning .ui-metric-strip__dot {
  background: var(--aria-warning);
}

.ui-metric-strip__item--danger .ui-metric-strip__dot {
  background: var(--aria-danger);
}

.ui-metric-strip__item--info .ui-metric-strip__dot {
  background: var(--aria-primary);
}
</style>
