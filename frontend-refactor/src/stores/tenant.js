// src/stores/tenant.js
import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '@/composables/useApi'
import { API_ENDPOINTS } from '@/config/api'

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
        currentTenant.value = JSON.parse(saved)
      } else if (tenants.value.length > 0) {
        currentTenant.value = tenants.value[0]
      }
    } catch (error) {
      console.error('Failed to load tenants:', error)
      // Fallback to mock data for demo
      tenants.value = [
        { id: 'tenant-1', name: 'Acme Corp', code: 'acme', status: 'active' },
        { id: 'tenant-2', name: 'Beta Inc', code: 'beta', status: 'active' },
        { id: 'tenant-3', name: 'Gamma LLC', code: 'gamma', status: 'suspended' }
      ]

      const saved = localStorage.getItem('aria-current-tenant')
      if (saved) {
        currentTenant.value = JSON.parse(saved)
      } else if (tenants.value.length > 0) {
        currentTenant.value = tenants.value[0]
      }
    } finally {
      loading.value = false
    }
  }

  function switchTenant(tenant) {
    currentTenant.value = tenant
    localStorage.setItem('aria-current-tenant', JSON.stringify(tenant))

    // Notify other parts of app about tenant change
    window.dispatchEvent(new CustomEvent('tenantChanged', { detail: tenant }))
  }

  return {
    currentTenant,
    tenants,
    loading,
    loadTenants,
    switchTenant
  }
})