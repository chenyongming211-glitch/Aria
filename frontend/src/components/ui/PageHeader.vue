<template>
  <header class="ui-page-header">
    <div class="ui-page-header__main">
      <h1 class="ui-page-header__title">{{ resolvedTitle }}</h1>
      <p v-if="resolvedSubtitle" class="ui-page-header__subtitle">{{ resolvedSubtitle }}</p>
    </div>
    <div v-if="$slots.actions" class="ui-page-header__actions">
      <slot name="actions" />
    </div>
  </header>
</template>

<script setup>
import { computed } from 'vue'
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

const resolvedTitle = computed(() => props.titleKey ? t(props.titleKey) : props.title)
const resolvedSubtitle = computed(() => props.subtitleKey ? t(props.subtitleKey) : props.subtitle)
</script>

<style scoped>
.ui-page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  min-width: 0;
  padding: 0 0 4px;
}

.ui-page-header__main {
  min-width: 0;
}

.ui-page-header__title {
  margin: 0;
  color: var(--aria-text-primary);
  font-size: 22px;
  font-weight: 700;
  line-height: 1.25;
  letter-spacing: 0;
}

.ui-page-header__subtitle {
  max-width: 760px;
  margin: 6px 0 0;
  color: var(--aria-text-secondary);
  font-size: 13px;
  line-height: 1.5;
}

.ui-page-header__actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  flex-wrap: wrap;
}

@media (max-width: 768px) {
  .ui-page-header {
    flex-direction: column;
    align-items: stretch;
  }

  .ui-page-header__actions {
    justify-content: flex-start;
  }
}
</style>
