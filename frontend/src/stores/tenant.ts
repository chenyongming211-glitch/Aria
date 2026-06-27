// src/stores/tenant.ts
import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '@/composables/useApi'
import { API_ENDPOINTS } from '@/config/api'
import useUserStore from '@/stores/user'
import type { Tenant } from '@/types'

interface HttpErrorLike {
  response?: {
    status?: number
  }
}

interface TenantApiResponse {
  data?: {
    data?: Tenant[]
  } | Tenant[]
}

function isSwitchableTenant(tenant: Tenant | null | undefined): boolean {
  const status = String(tenant?.status || 'active').toLowerCase()
  return status === 'active'
}

export default defineStore('tenant', () => {
  const currentTenant = ref<Tenant | null>(null)
  const tenants = ref<Tenant[]>([])
  const loading = ref(false)

  async function loadTenants(): Promise<void> {
    loading.value = true
    try {
      const response = await api.get(API_ENDPOINTS.TENANT.LIST) as TenantApiResponse
      console.log('[Tenant] API response:', response.data)
      const responseBody = response.data
      const allTenants = Array.isArray(responseBody)
        ? responseBody
        : (Array.isArray(responseBody?.data) ? responseBody.data : [])
      tenants.value = allTenants.filter(isSwitchableTenant)

      // Restore from localStorage
      const saved = localStorage.getItem('aria-current-tenant')
      if (saved) {
        try {
          const parsed = JSON.parse(saved)
          const matched = tenants.value.find((tenant) => tenant.id === parsed?.id)
          const nextTenant = matched || (tenants.value[0] || null)
          currentTenant.value = nextTenant
          if (nextTenant) {
            localStorage.setItem('aria-current-tenant', JSON.stringify(nextTenant))
          } else {
            localStorage.removeItem('aria-current-tenant')
          }
        } catch (error) {
          console.warn('Failed to parse aria-current-tenant:', error)
          currentTenant.value = tenants.value[0] || null
          if (currentTenant.value) {
            localStorage.setItem('aria-current-tenant', JSON.stringify(currentTenant.value))
          } else {
            localStorage.removeItem('aria-current-tenant')
          }
        }
      } else if (tenants.value.length > 0) {
        currentTenant.value = tenants.value[0]
        localStorage.setItem('aria-current-tenant', JSON.stringify(currentTenant.value))
      }
    } catch (error) {
      console.error('Failed to load tenants:', error)
      const status = (error as HttpErrorLike)?.response?.status
      if (status === 401) {
        tenants.value = []
        currentTenant.value = null
        localStorage.removeItem('aria-current-tenant')
      }
    } finally {
      loading.value = false
    }
  }

  async function switchTenant(tenant: Tenant | null): Promise<void> {
    if (tenant && !isSwitchableTenant(tenant)) {
      console.warn('Refusing to switch to inactive tenant:', tenant)
      return
    }

    currentTenant.value = tenant
    if (tenant?.id) {
      localStorage.setItem('aria-current-tenant', JSON.stringify(tenant))
    } else {
      localStorage.removeItem('aria-current-tenant')
    }

    const userStore = useUserStore()
    try {
      if (userStore.user?.role) {
        await userStore.loadPermissions(tenant?.id, userStore.user.role)
      }
    } catch (error) {
      console.warn('Failed to reload permissions after tenant switch:', error)
    } finally {
      // Notify other parts of app about tenant change
      window.dispatchEvent(new CustomEvent('tenantChanged', { detail: tenant }))
    }
  }

  return {
    currentTenant,
    tenants,
    loading,
    loadTenants,
    switchTenant
  }
})
