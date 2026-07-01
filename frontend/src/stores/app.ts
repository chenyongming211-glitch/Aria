// src/stores/app.ts
import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '@/composables/useApi'
import { API_ENDPOINTS } from '@/config/api'

interface FetchVersionOptions {
  reloadOnChange?: boolean
  reload?: () => void
}

export default defineStore('app', () => {
  const lang = ref(localStorage.getItem('aria-lang') || 'zh')
  const version = ref('0.0.0')
  const sidebarCollapsed = ref(false)
  let versionWatcher: number | null = null

  const setLang = (newLang: string) => {
    lang.value = newLang
    localStorage.setItem('aria-lang', newLang)
  }

  const toggleSidebar = () => {
    sidebarCollapsed.value = !sidebarCollapsed.value
  }

  const fetchVersion = async (options: FetchVersionOptions = {}) => {
    try {
      const response = await api.get(API_ENDPOINTS.VERSION)
      const data = response.data
      if (data.version) {
        const nextVersion = String(data.version)
        const previousVersion = version.value
        version.value = nextVersion
        if (
          options.reloadOnChange &&
          previousVersion &&
          previousVersion !== '0.0.0' &&
          previousVersion !== nextVersion
        ) {
          const reload = options.reload || (() => window.location.reload())
          reload()
        }
      }
    } catch (e) {
      console.error('Failed to fetch version:', e)
    }
  }

  const startVersionWatcher = (intervalMs = 60_000) => {
    if (typeof window === 'undefined' || versionWatcher !== null) {
      return
    }
    versionWatcher = window.setInterval(() => {
      fetchVersion({ reloadOnChange: true })
    }, intervalMs)
  }

  const stopVersionWatcher = () => {
    if (versionWatcher !== null) {
      window.clearInterval(versionWatcher)
      versionWatcher = null
    }
  }

  return {
    lang,
    version,
    sidebarCollapsed,
    setLang,
    toggleSidebar,
    fetchVersion,
    startVersionWatcher,
    stopVersionWatcher
  }
})
