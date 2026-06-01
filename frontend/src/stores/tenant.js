// src/stores/tenant.js
import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '@/composables/useApi'
import { API_ENDPOINTS } from '@/config/api'
import useUserStore from '@/stores/user'

export default defineStore('tenant', () => {
  const currentTenant = ref(null)
  const tenants = ref([])
  const loading = ref(false)

  async function loadTenants() {
    loading.value = true
    try {
      const response = await api.get(API_ENDPOINTS.TENANT.LIST)
      console.log('[Tenant] API response:', response.data)
      tenants.value = response.data?.data || response.data || []

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
      const status = error?.response?.status
      if (status === 401) {
        tenants.value = []
        currentTenant.value = null
        localStorage.removeItem('aria-current-tenant')
      }
    } finally {
      loading.value = false
    }
  }

  async function switchTenant(tenant) {
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
