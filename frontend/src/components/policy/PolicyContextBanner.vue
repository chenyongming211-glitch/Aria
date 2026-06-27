<template>
  <div v-if="hasContext" class="policy-context-banner" data-testid="policy-context-banner">
    <div class="policy-context-banner__main">
      <span class="policy-context-banner__label">当前上下文</span>
      <el-tag v-if="domain" size="small" effect="plain">{{ domain }}</el-tag>
      <el-tag v-if="nodeLabel" size="small" effect="plain">{{ nodeLabel }}</el-tag>
      <el-tag v-if="policyRef" size="small" type="warning" effect="plain">Policy {{ policyRef }}</el-tag>
      <el-tooltip v-if="commandId" :content="commandId" placement="top">
        <el-tag size="small" type="info" effect="plain">Command {{ shortCommandId }}</el-tag>
      </el-tooltip>
    </div>
    <div class="policy-context-banner__actions">
      <el-button
        v-if="nodeId"
        size="small"
        text
        data-testid="open-context-node"
        @click="$emit('open-node-detail')"
      >
        查看节点详情
      </el-button>
      <el-button
        v-if="nodeId || policyRef || commandId"
        size="small"
        text
        data-testid="open-context-policy-center"
        @click="$emit('open-policy-center')"
      >
        返回 Policy Center
      </el-button>
      <el-button size="small" text data-testid="clear-policy-context" @click="$emit('clear')">
        清除上下文
      </el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  domain?: string
  nodeId?: string
  nodeName?: string
  policyRef?: string
  commandId?: string
}>()

defineEmits<{
  clear: []
  'open-node-detail': []
  'open-policy-center': []
}>()

const hasContext = computed(() => Boolean(props.nodeId || props.policyRef || props.commandId))
const nodeLabel = computed(() => props.nodeName || props.nodeId || '')
const shortCommandId = computed(() => {
  if (!props.commandId) return ''
  return props.commandId.length > 8 ? props.commandId.slice(0, 8) : props.commandId
})
</script>

<style scoped>
.policy-context-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid var(--aria-border-primary, #dbe3ef);
  border-radius: 8px;
  background: var(--aria-info-soft, #ecf5ff);
}

.policy-context-banner__main,
.policy-context-banner__actions {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.policy-context-banner__main {
  flex-wrap: wrap;
}

.policy-context-banner__label {
  color: var(--aria-text-secondary, #5d6b82);
  font-size: 13px;
  font-weight: 600;
}

@media (max-width: 720px) {
  .policy-context-banner {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
