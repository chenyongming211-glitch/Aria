<!-- src/views/AIAssistant.vue - 现代化 AI 助手页面 -->
<template>
  <div class="ai-assistant-page">
    <el-card class="chat-card glass-card" shadow="never">
      <!-- 聊天头部 -->
      <template #header>
        <div class="chat-header">
          <div class="header-left">
            <div class="ai-avatar">
              <el-icon><ChatLineRound /></el-icon>
              <div class="avatar-glow"></div>
            </div>
            <div class="header-info">
              <h3 class="assistant-name">Aria AI Assistant</h3>
              <p class="assistant-status">
                <span class="status-dot"></span>
                Online and ready to help
              </p>
            </div>
          </div>
          <div class="header-actions">
            <el-button
              link
              :icon="Delete"
              @click="clearChat"
              class="action-btn"
            >
              Clear Chat
            </el-button>
          </div>
        </div>
      </template>

      <!-- 聊天消息区域 -->
      <div class="chat-messages" ref="messagesContainer">
        <!-- 欢迎消息 -->
        <div class="welcome-section" v-if="messages.length === 1">
          <div class="welcome-icon">
            <el-icon><StarFilled /></el-icon>
          </div>
          <h4 class="welcome-title">Hello! I'm Aria AI Assistant</h4>
          <p class="welcome-subtitle">I can help you with various tasks</p>
          <div class="capabilities-grid">
            <div v-for="(capability, index) in capabilities" :key="index" class="capability-item">
              <div class="capability-icon" :style="{ background: capability.bgColor }">
                <el-icon :color="capability.iconColor">
                  <component :is="capability.icon" />
                </el-icon>
              </div>
              <span class="capability-label">{{ capability.label }}</span>
            </div>
          </div>
        </div>

        <!-- 消息列表 -->
        <div
          v-for="(message, index) in messages"
          :key="index"
          class="message-wrapper"
          :class="message.role"
        >
          <div class="message">
            <div class="message-avatar">
              <el-avatar v-if="message.role === 'user'" :size="40">
                {{ currentUser?.initials || 'U' }}
              </el-avatar>
              <div v-else class="ai-avatar-small">
                <el-icon><ChatLineRound /></el-icon>
              </div>
            </div>
            <div class="message-content">
              <div class="message-bubble">
                <div class="message-text" v-html="formatMessage(message.content)"></div>
              </div>

              <!-- 卡片数据 -->
              <div v-if="message.cardData" class="message-card">
                <div class="card-header">
                  <el-icon class="card-icon"><DataBoard /></el-icon>
                  <span class="card-title">{{ message.cardData.title }}</span>
                </div>
                <div class="card-body">
                  <div
                    v-for="(value, key) in message.cardData.data"
                    :key="key"
                    class="card-row"
                  >
                    <span class="card-label">{{ key }}:</span>
                    <span class="card-value">{{ value }}</span>
                  </div>
                </div>
              </div>

              <!-- 工具调用标签 -->
              <div v-if="message.toolCalls && message.toolCalls.length > 0" class="tool-calls">
                <el-tag
                  v-for="(tool, i) in message.toolCalls"
                  :key="i"
                  type="info"
                  size="small"
                  effect="plain"
                  class="tool-tag"
                >
                  <el-icon><Tools /></el-icon>
                  {{ tool.name }}
                </el-tag>
              </div>

              <!-- 消息时间 -->
              <div class="message-time">{{ message.time || 'Just now' }}</div>
            </div>
          </div>
        </div>

        <!-- 加载状态 -->
        <div v-if="loading" class="message-wrapper assistant">
          <div class="message">
            <div class="message-avatar">
              <div class="ai-avatar-small">
                <el-icon><ChatLineRound /></el-icon>
              </div>
            </div>
            <div class="message-content">
              <div class="message-bubble typing-indicator">
                <div class="typing-dots">
                  <span></span>
                  <span></span>
                  <span></span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 输入区域 -->
      <div class="chat-input-area">
        <div class="input-wrapper">
          <el-input
            v-model="inputMessage"
            type="textarea"
            :rows="2"
            placeholder="Type your message here..."
            class="chat-input"
            @keydown.enter.ctrl="sendMessage"
            :disabled="loading"
            resize="none"
          />
          <el-button
            type="primary"
            :icon="Promotion"
            @click="sendMessage"
            :loading="loading"
            class="send-button"
            :disabled="!inputMessage.trim()"
          >
            <span v-if="!loading">Send</span>
            <span v-else>Sending...</span>
          </el-button>
        </div>
        <div class="input-footer">
          <span class="input-hint">Press Ctrl + Enter to send</span>
        </div>
      </div>
    </el-card>

    <!-- 工具确认对话框 -->
    <el-dialog
      v-model="showConfirmDialog"
      title="Confirm Action"
      width="500px"
      :close-on-click-modal="false"
      class="confirm-dialog"
    >
      <div v-if="pendingTool" class="confirm-content">
        <div class="confirm-header">
          <el-icon class="confirm-icon"><Warning /></el-icon>
          <h4>AI Requested to Execute:</h4>
        </div>
        <div class="confirm-details">
          <div class="detail-item">
            <span class="detail-label">Tool:</span>
            <el-tag type="primary">{{ pendingTool.name }}</el-tag>
          </div>
          <div
            v-for="(value, key) in pendingTool.params"
            :key="key"
            class="detail-item"
          >
            <span class="detail-label">{{ key }}:</span>
            <span class="detail-value">{{ value }}</span>
          </div>
        </div>
      </div>
      <template #footer>
        <div class="dialog-footer">
          <el-button @click="showConfirmDialog = false">Cancel</el-button>
          <el-button type="primary" @click="confirmTool" :loading="loading">
            Confirm & Execute
          </el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, nextTick, onMounted } from 'vue'
import {
  ChatLineRound,
  Delete,
  Promotion,
  StarFilled,
  Monitor,
  Position,
  DataAnalysis,
  Tools,
  DataBoard,
  Warning
} from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { useAiApi } from '@/composables/useAiApi'
import { useUserStore } from '@/stores'

const userStore = useUserStore()
const currentUser = computed(() => userStore.currentUser)

// 聊天状态
const messages = ref([
  {
    role: 'assistant',
    content: 'Hello! I\'m Aria AI Assistant. I can help you with:\n\n• Query node status\n• Manage network configuration\n• Analyze monitoring data\n• Answer system questions\n\nWhat can I help you with today?',
    time: 'Just now'
  }
])

const inputMessage = ref('')
const loading = ref(false)
const sessionId = ref('')
const messagesContainer = ref(null)

// 工具确认状态
const showConfirmDialog = ref(false)
const pendingTool = ref(null)

// 功能能力
const capabilities = [
  {
    icon: Monitor,
    label: 'Query Status',
    bgColor: 'rgba(59, 130, 246, 0.15)',
    iconColor: '#3B82F6'
  },
  {
    icon: Position,
    label: 'Manage Config',
    bgColor: 'rgba(34, 197, 94, 0.15)',
    iconColor: '#22C55E'
  },
  {
    icon: DataAnalysis,
    label: 'Analyze Data',
    bgColor: 'rgba(245, 158, 11, 0.15)',
    iconColor: '#F59E0B'
  },
  {
    icon: Tools,
    label: 'System Ops',
    bgColor: 'rgba(139, 92, 246, 0.15)',
    iconColor: '#8B5CF6'
  }
]

// 发送消息
const sendMessage = async () => {
  const message = inputMessage.value.trim()
  if (!message || loading.value) return

  messages.value.push({
    role: 'user',
    content: message,
    time: new Date().toLocaleTimeString()
  })

  inputMessage.value = ''
  await scrollToBottom()

  loading.value = true
  try {
    const response = await useAiApi.chat({
      message,
      session_id: sessionId.value,
      tools: true
    })

    if (response.session_id) {
      sessionId.value = response.session_id
    }

    handleAIResponse(response)
  } catch (error) {
    console.error('AI 调用失败:', error)
    ElMessage.error('AI service temporarily unavailable, please try again later')
    messages.value.push({
      role: 'assistant',
      content: 'Sorry, I encountered an error. Please try again later.',
      time: new Date().toLocaleTimeString()
    })
  } finally {
    loading.value = false
    await scrollToBottom()
  }
}

// 处理 AI 响应
const handleAIResponse = (response) => {
  const { reply, card_data, tool_calls, needs_confirm } = response

  if (needs_confirm && tool_calls && tool_calls.length > 0) {
    pendingTool.value = tool_calls[0]
    showConfirmDialog.value = true
  }

  let displayCardData = null
  let displayContent = reply || ''

  if (card_data) {
    if (Array.isArray(card_data) && card_data.length > 0) {
      displayCardData = {
        title: 'Node List',
        data: {}
      }
      card_data.forEach((node, index) => {
        displayCardData.data[`Node ${index + 1}`] = `${node.name} (${node.region}) - ${node.status}`
      })
    } else if (typeof card_data === 'object' && card_data.title) {
      displayCardData = {
        title: card_data.title,
        data: {}
      }

      const fieldsToShow = ['total_nodes', 'online_nodes', 'offline_nodes', 'message']
      fieldsToShow.forEach(field => {
        if (card_data[field] !== undefined) {
          const label = {
            total_nodes: 'Total Nodes',
            online_nodes: 'Online Nodes',
            offline_nodes: 'Offline Nodes',
            message: 'Message'
          }[field]
          displayCardData.data[label] = card_data[field]
        }
      })

      if (card_data.by_region && Object.keys(card_data.by_region).length > 0) {
        Object.entries(card_data.by_region).forEach(([region, count]) => {
          displayCardData.data[`Region ${region}`] = `${count} nodes`
        })
      }
    } else if (Array.isArray(card_data) && card_data.length === 0) {
      displayContent = 'Currently no registered nodes. Please use Token to register Agent to Controller.'
      displayCardData = {
        title: 'Node Status',
        data: {
          'Total Nodes': 0,
          'Online Nodes': 0,
          'Offline Nodes': 0
        }
      }
    }
  }

  messages.value.push({
    role: 'assistant',
    content: displayContent,
    cardData: displayCardData,
    toolCalls: tool_calls,
    time: new Date().toLocaleTimeString()
  })
}

// 确认工具执行
const confirmTool = async () => {
  if (!pendingTool.value) return

  showConfirmDialog.value = false
  loading.value = true

  try {
    const result = await useAiApi.confirm({
      session_id: sessionId.value,
      tool_name: pendingTool.value.name,
      params: pendingTool.value.params || {},
      confirmed: true
    })

    messages.value.push({
      role: 'assistant',
      content: `✅ Action completed successfully.\n\nResult: ${JSON.stringify(result, null, 2)}`,
      time: new Date().toLocaleTimeString()
    })

    ElMessage.success('Action executed successfully')
  } catch (error) {
    console.error('工具执行失败:', error)
    ElMessage.error('Action execution failed')
    messages.value.push({
      role: 'assistant',
      content: 'Sorry, the action failed. Please try again later.',
      time: new Date().toLocaleTimeString()
    })
  } finally {
    loading.value = false
    pendingTool.value = null
    await scrollToBottom()
  }
}

// 清空对话
const clearChat = () => {
  messages.value = [
    {
      role: 'assistant',
      content: 'Conversation cleared. How can I help you today?',
      time: new Date().toLocaleTimeString()
    }
  ]
  sessionId.value = ''
}

// 滚动到底部
const scrollToBottom = async () => {
  await nextTick()
  if (messagesContainer.value) {
    messagesContainer.value.scrollTop = messagesContainer.value.scrollHeight
  }
}

// 格式化消息
const formatMessage = (content) => {
  if (!content) return ''
  return content
    .replace(/\n/g, '<br>')
    .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
    .replace(/`([^`]+)`/g, '<code>$1</code>')
}

onMounted(() => {
  scrollToBottom()
})
</script>

<style scoped>
/* ============================================
   AI Assistant Page Styles
   ============================================ */
.ai-assistant-page {
  height: calc(100vh - 120px);
  display: flex;
  flex-direction: column;
}

/* ============================================
   Chat Card
   ============================================ */
.chat-card {
  flex: 1;
  display: flex;
  flex-direction: column;
  background: var(--aria-bg-secondary);
  border: 1px solid var(--aria-border-primary);
  border-radius: var(--aria-radius-lg);
  overflow: hidden;
}

/* Chat Header */
.chat-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0;
  border: none;
  background: transparent;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 16px;
}

.ai-avatar {
  position: relative;
  width: 48px;
  height: 48px;
  border-radius: var(--radius-full);
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  color: white;
  font-size: 24px;
}

.avatar-glow {
  position: absolute;
  inset: -4px;
  border-radius: var(--radius-full);
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  filter: blur(8px);
  opacity: 0.5;
  animation: avatar-glow 3s ease-in-out infinite;
}

@keyframes avatar-glow {
  0%, 100% { opacity: 0.5; transform: scale(1); }
  50% { opacity: 0.8; transform: scale(1.05); }
}

.header-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.assistant-name {
  font-size: 16px;
  font-weight: 600;
  color: var(--aria-text-primary);
  margin: 0;
}

.assistant-status {
  font-size: 12px;
  color: var(--aria-text-muted);
  margin: 0;
  display: flex;
  align-items: center;
  gap: 6px;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--aria-success);
  box-shadow: 0 0 8px rgba(34, 197, 94, 0.5);
  animation: pulse-dot 2s ease-in-out infinite;
}

@keyframes pulse-dot {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.6; }
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.action-btn {
  color: var(--aria-text-secondary);
  font-weight: 500;
  transition: all var(--aria-transition-fast);
}

.action-btn:hover {
  color: var(--aria-primary);
}

/* ============================================
   Chat Messages
   ============================================ */
.chat-messages {
  flex: 1;
  overflow-y: auto;
  padding: 24px;
  min-height: 0;
}

/* Welcome Section */
.welcome-section {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 40px 20px;
  text-align: center;
}

.welcome-icon {
  width: 80px;
  height: 80px;
  border-radius: var(--radius-full);
  background: linear-gradient(135deg, rgba(102, 126, 234, 0.2) 0%, rgba(118, 75, 162, 0.2) 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 40px;
  color: #667eea;
  margin-bottom: 24px;
}

.welcome-title {
  font-size: 24px;
  font-weight: 700;
  color: var(--aria-text-primary);
  margin: 0 0 8px 0;
}

.welcome-subtitle {
  font-size: 14px;
  color: var(--aria-text-secondary);
  margin: 0 0 32px 0;
}

.capabilities-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 16px;
  width: 100%;
  max-width: 600px;
}

.capability-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 20px;
  background: var(--aria-bg-tertiary);
  border: 1px solid var(--aria-border-primary);
  border-radius: var(--aria-radius-md);
  transition: all var(--aria-transition-base);
  cursor: default;
}

.capability-item:hover {
  border-color: var(--aria-border-hover);
  transform: translateY(-2px);
  box-shadow: var(--aria-shadow);
}

.capability-icon {
  width: 48px;
  height: 48px;
  border-radius: var(--aria-radius-md);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 24px;
}

.capability-label {
  font-size: 13px;
  font-weight: 500;
  color: var(--aria-text-secondary);
}

/* Message Wrapper */
.message-wrapper {
  margin-bottom: 24px;
}

.message-wrapper.assistant .message {
  flex-direction: row;
}

.message-wrapper.user .message {
  flex-direction: row-reverse;
}

.message {
  display: flex;
  gap: 12px;
  max-width: 100%;
}

.message-avatar {
  flex-shrink: 0;
}

.ai-avatar-small {
  width: 40px;
  height: 40px;
  border-radius: var(--radius-full);
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  color: white;
  font-size: 20px;
}

.message-content {
  max-width: 70%;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.message-wrapper.assistant .message-content {
  align-items: flex-start;
}

.message-wrapper.user .message-content {
  align-items: flex-end;
}

/* Message Bubble */
.message-bubble {
  padding: 16px 20px;
  border-radius: var(--aria-radius-lg);
  line-height: 1.6;
  word-break: break-word;
}

.message-wrapper.assistant .message-bubble {
  background: var(--aria-bg-tertiary);
  border: 1px solid var(--aria-border-primary);
  color: var(--aria-text-primary);
}

.message-wrapper.user .message-bubble {
  background: linear-gradient(135deg, var(--aria-primary) 0%, var(--aria-primary-dark) 100%);
  color: white;
}

.message-text {
  color: inherit;
}

.message-text :deep(code) {
  background: rgba(0, 0, 0, 0.1);
  padding: 2px 8px;
  border-radius: 4px;
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 0.9em;
  color: inherit;
}

.message-wrapper.user .message-text :deep(code) {
  background: rgba(255, 255, 255, 0.2);
}

.message-text :deep(strong) {
  font-weight: 600;
}

/* Message Card */
.message-card {
  padding: 16px;
  background: var(--aria-bg-secondary);
  border: 1px solid var(--aria-border-primary);
  border-radius: var(--aria-radius-md);
}

.card-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}

.card-icon {
  font-size: 18px;
  color: var(--aria-primary);
}

.card-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--aria-text-primary);
}

.card-body {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.card-row {
  display: flex;
  gap: 8px;
}

.card-label {
  color: var(--aria-text-muted);
  font-size: 13px;
  min-width: 100px;
}

.card-value {
  color: var(--aria-text-primary);
  font-size: 13px;
  flex: 1;
}

/* Tool Calls */
.tool-calls {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.tool-tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin: 0;
}

/* Message Time */
.message-time {
  font-size: 11px;
  color: var(--aria-text-disabled);
  margin-top: 4px;
}

/* Typing Indicator */
.typing-indicator {
  padding: 16px 20px;
}

.typing-dots {
  display: flex;
  gap: 4px;
  align-items: center;
}

.typing-dots span {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: currentColor;
  animation: typing-bounce 1.4s ease-in-out infinite;
}

.typing-dots span:nth-child(1) {
  animation-delay: 0s;
}

.typing-dots span:nth-child(2) {
  animation-delay: 0.2s;
}

.typing-dots span:nth-child(3) {
  animation-delay: 0.4s;
}

@keyframes typing-bounce {
  0%, 60%, 100% { transform: translateY(0); }
  30% { transform: translateY(-4px); }
}

/* ============================================
   Chat Input Area
   ============================================ */
.chat-input-area {
  padding: 20px;
  border-top: 1px solid var(--aria-border-primary);
  background: var(--aria-bg-secondary);
}

.input-wrapper {
  display: flex;
  gap: 12px;
  align-items: flex-end;
}

.chat-input {
  flex: 1;
}

:deep(.chat-input .el-textarea__inner) {
  background: var(--aria-bg-tertiary);
  border: 1px solid var(--aria-border-primary);
  border-radius: var(--aria-radius-md);
  color: var(--aria-text-primary);
  font-size: 14px;
  resize: none;
  transition: all var(--aria-transition-base);
}

:deep(.chat-input .el-textarea__inner:focus) {
  border-color: var(--aria-primary);
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
}

:deep(.chat-input .el-textarea__inner::placeholder) {
  color: var(--aria-text-muted);
}

.send-button {
  height: 80px;
  min-width: 100px;
  font-weight: 600;
  border-radius: var(--aria-radius-md);
}

.input-footer {
  display: flex;
  justify-content: center;
  margin-top: 12px;
}

.input-hint {
  font-size: 12px;
  color: var(--aria-text-disabled);
}

/* ============================================
   Confirm Dialog
   ============================================ */
:deep(.confirm-dialog) {
  background: var(--aria-bg-secondary);
}

.confirm-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.confirm-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding-bottom: 16px;
  border-bottom: 1px solid var(--aria-border-primary);
}

.confirm-icon {
  font-size: 32px;
  color: var(--aria-warning);
}

.confirm-header h4 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: var(--aria-text-primary);
}

.confirm-details {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.detail-item {
  display: flex;
  align-items: center;
  gap: 12px;
}

.detail-label {
  min-width: 60px;
  color: var(--aria-text-secondary);
  font-weight: 500;
}

.detail-value {
  color: var(--aria-text-primary);
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 13px;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}

/* ============================================
   Responsive
   ============================================ */
@media (max-width: 768px) {
  .ai-assistant-page {
    height: calc(100vh - 100px);
  }

  .message-content {
    max-width: 85%;
  }

  .capabilities-grid {
    grid-template-columns: repeat(2, 1fr);
  }

  .input-wrapper {
    flex-direction: column;
  }

  .send-button {
    width: 100%;
    height: 48px;
  }
}

@media (max-width: 480px) {
  .capabilities-grid {
    grid-template-columns: 1fr;
  }

  .header-info {
    display: none;
  }
}
</style>
