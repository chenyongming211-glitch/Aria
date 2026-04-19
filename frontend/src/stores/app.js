// src/stores/app.js
import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '@/composables/useApi'
import { API_ENDPOINTS } from '@/config/api'

export default defineStore('app', () => {
  const lang = ref(localStorage.getItem('aria-lang') || 'zh')
  const version = ref('0.0.0')
  const sidebarCollapsed = ref(false)

  const setLang = (newLang) => {
    lang.value = newLang
    localStorage.setItem('aria-lang', newLang)
  }

  const toggleSidebar = () => {
    sidebarCollapsed.value = !sidebarCollapsed.value
  }

  const fetchVersion = async () => {
    try {
      const response = await api.get(API_ENDPOINTS.VERSION)
      const data = response.data
      if (data.version) {
        version.value = data.version
      }
    } catch (e) {
      console.error('Failed to fetch version:', e)
    }
  }

  return {
    lang,
    version,
    sidebarCollapsed,
    setLang,
    toggleSidebar,
    fetchVersion
  }
})
