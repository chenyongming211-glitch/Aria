// src/stores/app.js
import { defineStore } from 'pinia'
import { ref } from 'vue'

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
      const response = await fetch('/api/v1/version')
      const data = await response.json()
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
