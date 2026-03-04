// src/stores/app.js
import { defineStore } from 'pinia'
import { ref } from 'vue'

export default defineStore('app', () => {
  const lang = ref(localStorage.getItem('aria-lang') || 'zh')
  const version = ref('0.2.27')
  const sidebarCollapsed = ref(false)

  const setLang = (newLang) => {
    lang.value = newLang
    localStorage.setItem('aria-lang', newLang)
  }

  const toggleSidebar = () => {
    sidebarCollapsed.value = !sidebarCollapsed.value
  }

  return {
    lang,
    version,
    sidebarCollapsed,
    setLang,
    toggleSidebar
  }
})