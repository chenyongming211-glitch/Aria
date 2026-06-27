<template>
  <button
    type="button"
    class="ui-action-icon-button"
    :class="toneClass"
    :aria-label="label"
    :title="label"
    :disabled="disabled"
    @click="handleClick"
  >
    <slot />
    <span class="ui-action-icon-button__label">{{ label }}</span>
  </button>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  label: {
    type: String,
    required: true
  },
  tone: {
    type: String,
    default: 'neutral',
    validator: (value) => ['neutral', 'primary', 'danger'].includes(value)
  },
  disabled: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits(['click'])
const toneClass = computed(() => `ui-action-icon-button--${props.tone}`)

const handleClick = (event) => {
  if (props.disabled) return
  emit('click', event)
}
</script>

<style scoped>
.ui-action-icon-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  padding: 0;
  border: 1px solid var(--aria-border-primary);
  border-radius: var(--aria-radius-sm);
  background: var(--aria-content-bg-tertiary);
  color: var(--aria-text-secondary);
  cursor: pointer;
  transition: background-color var(--aria-transition-base), border-color var(--aria-transition-base), color var(--aria-transition-base);
}

.ui-action-icon-button:hover,
.ui-action-icon-button:focus-visible {
  background: var(--aria-content-bg-secondary);
  border-color: var(--aria-border-hover);
  color: var(--aria-text-primary);
}

.ui-action-icon-button--primary {
  color: var(--aria-primary);
  background: rgba(37, 99, 235, 0.08);
  border-color: rgba(37, 99, 235, 0.18);
}

.ui-action-icon-button--danger {
  color: var(--aria-danger);
  background: rgba(239, 68, 68, 0.08);
  border-color: rgba(239, 68, 68, 0.18);
}

.ui-action-icon-button:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.ui-action-icon-button__label {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}
</style>
