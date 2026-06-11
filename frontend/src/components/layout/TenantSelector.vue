<!-- src/components/layout/TenantSelector.vue -->
<template>
  <el-dropdown
    trigger="click"
    @command="switchTenant"
    placement="bottom-end"
    :hide-after="0"
  >
    <div class="tenant-selector-trigger">
      <el-tag type="primary" size="small" effect="dark">
        {{ currentTenant?.name || 'Loading...' }}
      </el-tag>
      <el-icon class="ml-2"><CaretBottom /></el-icon>
    </div>
    <template #dropdown>
      <el-dropdown-menu>
        <el-dropdown-item
          v-for="tenant in tenants"
          :key="tenant.id"
          :command="tenant"
          :disabled="tenant.id === currentTenant?.id"
        >
          <div class="tenant-option">
            <div class="tenant-info">
              <div class="tenant-name">{{ tenant.name }}</div>
              <div class="tenant-code">{{ tenant.code }}</div>
            </div>
            <el-tag :type="getStatusType(tenant.status)" size="small">
              {{ tenant.status }}
            </el-tag>
          </div>
        </el-dropdown-item>
      </el-dropdown-menu>
    </template>
  </el-dropdown>
</template>

<script setup>
import { CaretBottom } from '@element-plus/icons-vue'
import { storeToRefs } from 'pinia'
import { useTenantStore } from '@/stores'

const tenantStore = useTenantStore()
const { currentTenant, tenants } = storeToRefs(tenantStore)

const getStatusType = (status) => {
  switch(status) {
    case 'active': return 'success'
    case 'suspended': return 'warning'
    default: return 'info'
  }
}

const switchTenant = (tenant) => {
  tenantStore.switchTenant(tenant)
}

// Load tenants when component mounts
tenantStore.loadTenants()
</script>

<style scoped>
.tenant-selector-trigger {
  display: flex;
  align-items: center;
  cursor: pointer;
  padding: 4px 8px;
  border-radius: 4px;
  transition: background-color 0.2s;
}

.tenant-selector-trigger:hover {
  background-color: #f5f5f5;
}

.tenant-option {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 200px;
}

.tenant-info {
  flex: 1;
}

.tenant-name {
  font-weight: 600;
  margin-bottom: 2px;
}

.tenant-code {
  font-size: 12px;
  color: #666;
}
</style>