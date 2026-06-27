import { onMounted, onUnmounted } from 'vue'

type TenantChangeHandler = (detail: unknown) => unknown | Promise<unknown>

export function useTenantChangeReload(handler: TenantChangeHandler) {
  const listener = (event: Event) => {
    const detail = event instanceof CustomEvent ? event.detail : undefined
    Promise.resolve(handler(detail)).catch((error: unknown) => {
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
