<template>
  <div class="ui-filter-bar">
    <div class="ui-filter-bar__filters">
      <slot name="filters" />
    </div>
    <div v-if="$slots.actions" class="ui-filter-bar__actions">
      <slot name="actions" />
    </div>
  </div>
</template>

<style scoped>
.ui-filter-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-width: 0;
  padding: 12px 14px;
  background: var(--aria-content-bg-secondary);
  border: 1px solid var(--aria-border-primary);
  border-radius: var(--aria-radius);
}

.ui-filter-bar__filters,
.ui-filter-bar__actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  min-width: 0;
}

.ui-filter-bar__actions {
  justify-content: flex-end;
}

@media (max-width: 768px) {
  .ui-filter-bar {
    flex-direction: column;
    align-items: stretch;
  }

  .ui-filter-bar__actions {
    justify-content: flex-start;
  }
}
</style>
