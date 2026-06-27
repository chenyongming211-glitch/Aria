<template>
  <section class="ui-data-panel">
    <header v-if="hasHeader" class="ui-data-panel__header">
      <div class="ui-data-panel__heading">
        <h2 v-if="resolvedTitle" class="ui-data-panel__title">{{ resolvedTitle }}</h2>
        <p v-if="resolvedSubtitle" class="ui-data-panel__subtitle">{{ resolvedSubtitle }}</p>
      </div>
      <div v-if="$slots.actions" class="ui-data-panel__actions">
        <slot name="actions" />
      </div>
    </header>
    <div class="ui-data-panel__body">
      <slot />
    </div>
  </section>
</template>

<script setup>
import { computed, useSlots } from 'vue'
import { t } from '@/i18n'

const props = defineProps({
  title: {
    type: String,
    default: ''
  },
  titleKey: {
    type: String,
    default: ''
  },
  subtitle: {
    type: String,
    default: ''
  },
  subtitleKey: {
    type: String,
    default: ''
  }
})

const slots = useSlots()
const resolvedTitle = computed(() => props.titleKey ? t(props.titleKey) : props.title)
const resolvedSubtitle = computed(() => props.subtitleKey ? t(props.subtitleKey) : props.subtitle)
const hasHeader = computed(() => Boolean(resolvedTitle.value || resolvedSubtitle.value || slots.actions))
</script>

<style scoped>
.ui-data-panel {
  background: var(--aria-content-bg-secondary);
  border: 1px solid var(--aria-border-primary);
  border-radius: var(--aria-radius);
  overflow: hidden;
}

.ui-data-panel__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 16px;
  border-bottom: 1px solid var(--aria-border-primary);
}

.ui-data-panel__heading {
  min-width: 0;
}

.ui-data-panel__title {
  margin: 0;
  color: var(--aria-text-primary);
  font-size: 15px;
  font-weight: 700;
}

.ui-data-panel__subtitle {
  margin: 4px 0 0;
  color: var(--aria-text-muted);
  font-size: 12px;
}

.ui-data-panel__actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  flex-wrap: wrap;
}

.ui-data-panel__body {
  min-width: 0;
}
</style>
