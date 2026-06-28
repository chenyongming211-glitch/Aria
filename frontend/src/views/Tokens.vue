<!-- src/views/Tokens.vue -->
<template>
  <div class="tokens">
    <el-card>
      <template #header>
        <div class="card-header">
          <h3>{{ t('nav.tokenManagement') }}</h3>
          <div class="header-actions">
            <el-input
              v-model="searchQuery"
              :placeholder="t('common.search')"
              style="width: 200px; margin-right: 10px;"
              clearable
            >
              <template #prefix>
                <el-icon><Search /></el-icon>
              </template>
            </el-input>
            <el-button v-if="hasPermission('tokens:write')" type="primary" @click="createToken">
              <el-icon><Plus /></el-icon>
              {{ t('common.add') }}
            </el-button>
            <el-button @click="fetchTokens">
              <el-icon><Refresh /></el-icon>
              {{ t('common.refresh') }}
            </el-button>
          </div>
        </div>
      </template>

      <el-table
        :data="paginatedTokens"
        stripe
        style="width: 100%"
        v-loading="loading"
      >
        <el-table-column prop="id" :label="t('common.id')" width="200" />
        <el-table-column :label="t('common.token')" width="220">
          <template #default="{ row }">
            {{ tokenDisplay(row) }}
          </template>
        </el-table-column>
        <el-table-column prop="tenant_id" :label="t('common.tenantId')" width="200" />
        <el-table-column prop="tag" :label="t('common.tag')" width="150" />
        <el-table-column prop="used_count" :label="t('common.usage')" width="100">
          <template #default="{ row }">
            {{ row.used_count }} / {{ row.max_uses }}
          </template>
        </el-table-column>
        <el-table-column prop="status" :label="t('common.status')" width="120">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)">
              {{ t(`common.${row.status}`) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" :label="t('common.createdAt')" width="180" />
        <el-table-column prop="expires_at" :label="t('common.expiresAt')" width="180" />
        <el-table-column :label="t('common.actions')" width="250">
          <template #default="{ row }">
            <el-button size="small" @click="viewToken(row)">{{ t('common.view') }}</el-button>
            <el-popconfirm
              v-if="hasPermission('tokens:write')"
              :title="t('tokens.revokeConfirm')"
              @confirm="revokeToken(row.id)"
            >
              <template #reference>
                <el-button size="small" type="danger">{{ t('common.revoke') }}</el-button>
              </template>
            </el-popconfirm>
            <el-button size="small" :disabled="!row.token" @click="copyToken(row.token)" type="info">{{ t('common.copy') }}</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination-container">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :page-sizes="[10, 20, 50, 100]"
          :total="filteredTokens.length"
          layout="sizes, prev, pager, next, jumper, ->, total"
          @size-change="handleSizeChange"
          @current-change="handleCurrentChange"
        />
      </div>
    </el-card>

    <!-- Token Creation Dialog -->
    <el-dialog
      v-model="dialogVisible"
      :title="isCreating ? t('tokens.createTitle') : t('tokens.tokenDetails')"
      width="50%"
    >
      <el-form
        v-if="currentToken"
        :model="currentToken"
        label-width="120px"
      >
        <el-form-item :label="t('common.tag')">
          <el-input v-model="currentToken.tag" :disabled="!isCreating" />
        </el-form-item>
        <el-form-item :label="t('common.maxUses')" v-if="isCreating">
          <el-input-number v-model="currentToken.max_uses" :min="1" :max="1000" />
        </el-form-item>
        <el-form-item :label="t('common.ttlHours')" v-if="isCreating">
          <el-input-number v-model="currentToken.ttl_hours" :min="1" :max="8760" /> <!-- Up to 1 year -->
        </el-form-item>
        <el-form-item :label="t('common.status')" v-if="!isCreating">
          <el-select v-model="currentToken.status" disabled>
            <el-option :label="t('common.active')" value="active" />
            <el-option :label="t('common.expired')" value="expired" />
            <el-option :label="t('common.exhausted')" value="exhausted" />
            <el-option :label="t('common.revoked')" value="revoked" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('common.token')" v-if="!isCreating">
          <el-input :value="currentToken.token" readonly>
            <template #append>
              <el-button :disabled="!currentToken.token" @click="copyToken(currentToken.token)">{{ t('common.copy') }}</el-button>
            </template>
          </el-input>
          <div v-if="!currentToken.token && currentToken.token_preview" class="token-preview">
            {{ currentToken.token_preview }}
          </div>
        </el-form-item>
        <el-form-item :label="t('common.tenantId')" v-if="currentToken.tenant_id">
          <el-input :value="currentToken.tenant_id" readonly />
        </el-form-item>
        <el-form-item :label="t('common.usage')">
          <div>{{ currentToken.used_count || 0 }} / {{ currentToken.max_uses || '∞' }}</div>
        </el-form-item>
        <el-form-item :label="t('common.createdAt')">
          <el-input :value="currentToken.created_at" readonly />
        </el-form-item>
        <el-form-item :label="t('common.expiresAt')">
          <el-input :value="currentToken.expires_at" readonly />
        </el-form-item>
      </el-form>

      <template #footer v-if="isCreating">
        <span class="dialog-footer">
          <el-button @click="dialogVisible = false">{{ t('common.cancel') }}</el-button>
          <el-button v-if="hasPermission('tokens:write')" type="primary" @click="saveToken">{{ t('common.create') }}</el-button>
        </span>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { Search, Plus, Refresh } from '@element-plus/icons-vue'
import { ElMessage, ElNotification } from 'element-plus'
import { t } from '@/i18n'
import { useTokenApi } from '@/composables/useTokenApi'
import { usePermission } from '@/composables/usePermission'
import { useTenantChangeReload } from '@/composables/useTenantChangeReload'

const { hasPermission } = usePermission()

const tokens = ref([])
const loading = ref(false)
const searchQuery = ref('')
const currentPage = ref(1)
const pageSize = ref(10)
const dialogVisible = ref(false)
const currentToken = ref(null)
const isCreating = ref(false)

// Get status type for tag coloring
const getStatusType = (status) => {
  switch(status) {
    case 'active': return 'success'
    case 'revoked': return 'danger'
    case 'inactive': return 'info'
    case 'expired': return 'warning'
    case 'exhausted': return 'info'
    default: return 'info'
  }
}

// Computed property for filtered tokens
const safeLower = (value) => String(value ?? '').toLowerCase()

const tokenDisplay = (token) => token?.token_preview || token?.token || ''

const filteredTokens = computed(() => {
  if (!searchQuery.value) {
    return tokens.value
  }

  const query = safeLower(searchQuery.value)
  return tokens.value.filter(token =>
    safeLower(token.id).includes(query) ||
    safeLower(token.token).includes(query) ||
    safeLower(token.token_preview).includes(query) ||
    safeLower(token.tag).includes(query)
  )
})

const paginatedTokens = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  return filteredTokens.value.slice(start, start + pageSize.value)
})

// Fetch tokens from backend
const fetchTokens = async () => {
  try {
    loading.value = true
    const response = await useTokenApi.getAllTokens()

    // 处理不同格式的响应
    if (Array.isArray(response)) {
      tokens.value = response
    } else if (response && response.data && Array.isArray(response.data)) {
      tokens.value = response.data
    } else {
      tokens.value = []
    }

    if (tokens.value.length === 0) {
      // No tokens found - silently show empty state
    }
  } catch (error) {
    console.error('Failed to fetch tokens:', error)

    // 不显示错误消息，避免401错误导致重复提示
    // ElMessage.error(`Failed to load tokens: ${error.message || error}`)

    // Fallback to empty array
    tokens.value = []
  } finally {
    loading.value = false
  }
}

// Create token
const createToken = () => {
  currentToken.value = {
    tag: '',
    max_uses: 1,
    ttl_hours: 24
  }
  isCreating.value = true
  dialogVisible.value = true
}

// View token details
const viewToken = async (token) => {
  try {
    // Get detailed information about token
    const tokenDetails = await useTokenApi.getTokenDetail(token.id)
    currentToken.value = { ...token, ...tokenDetails }
  } catch (error) {
    // If detailed info fails, show basic token info
    currentToken.value = { ...token }
    console.warn('Could not get detailed token info:', error)
  }
  isCreating.value = false
  dialogVisible.value = true
}

// Copy token to clipboard
const copyToken = async (tokenValue) => {
  if (!tokenValue) {
    ElMessage.warning(t('tokens.firstCreatedOnly'))
    return
  }

  try {
    await navigator.clipboard.writeText(tokenValue)
    ElNotification({
      title: t('tokens.copySuccessTitle'),
      message: t('tokens.copySuccess'),
      type: 'success'
    })
  } catch (err) {
    console.error('Failed to copy token: ', err)
    ElMessage.error(t('tokens.copyFailed'))
  }
}

// Save token (create)
const saveToken = async () => {
  if (!currentToken.value.tag.trim()) {
    ElMessage.error(t('tokens.tagRequired'))
    return
  }

  try {
    const params = {
      tag: currentToken.value.tag,
      max_uses: currentToken.value.max_uses,
      ttl: `${currentToken.value.ttl_hours}h`  // Format as duration string
    }

    const response = await useTokenApi.createToken(params)

    // Add new token to list
    tokens.value.unshift(response)
    ElMessage.success(t('tokens.created'))
    ElNotification({
      title: t('tokens.createdTitle'),
      message: t('tokens.createdMessage').replace('{token}', tokenDisplay(response)),
      type: 'success',
      duration: 0 // Don't auto-close
    })

    dialogVisible.value = false
  } catch (error) {
    console.error('Failed to create token:', error)
    ElMessage.error(`${t('tokens.createFailed')}: ${error.message || error}`)
  }
}

// Revoke token
const revokeToken = async (tokenId) => {
  try {
    await useTokenApi.revokeToken(tokenId)

    // Update token status in local list
    const index = tokens.value.findIndex(t => t.id === tokenId)
    if (index !== -1) {
      tokens.value[index].status = 'revoked'
    }

    ElMessage.success(t('tokens.revoked'))
  } catch (error) {
    console.error('Failed to revoke token:', error)
    ElMessage.error(`${t('tokens.revokeFailed')}: ${error.message || error}`)
  }
}

// Pagination handlers
const handleSizeChange = (size) => {
  pageSize.value = size
  currentPage.value = 1
}

const handleCurrentChange = (page) => {
  currentPage.value = page
}

// Load tokens on component mount
onMounted(() => {
  fetchTokens()
})

useTenantChangeReload(() => {
  currentPage.value = 1
  return fetchTokens()
})
</script>

<style scoped>
.tokens {
  padding: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-actions {
  display: flex;
  gap: 10px;
}

.pagination-container {
  margin-top: 20px;
  display: flex;
  justify-content: flex-end;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}

.token-preview {
  margin-top: 6px;
  color: #909399;
  font-size: 12px;
}
</style>
