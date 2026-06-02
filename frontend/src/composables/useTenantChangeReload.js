import { onMounted, onUnmounted } from 'vue'

export function useTenantChangeReload(handler) {
  const listener = (event) => {
    Promise.resolve(handler(event?.detail)).catch((error) => {
      console.error('Failed to reload tenant-scoped view:', error)
    })
  }

  onMounted(() => {
    if (typeof window !== 'undefined') {
      window.addEventListener('tenantChanged', listener)
    }
  })

  onUnmounted(() => {
    if (typeof window !== 'undefined') {
      window.removeEventListener('tenantChanged', listener)
    }
  })
}
