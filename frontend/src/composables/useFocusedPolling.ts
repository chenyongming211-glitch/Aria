import { onMounted, onUnmounted, ref } from 'vue'

interface FocusedPollingOptions {
  poll: () => Promise<void>
  hasActiveItems: () => boolean
  intervalMs?: number
  enabled?: () => boolean
}

export function useFocusedPolling(options: FocusedPollingOptions) {
  const isPolling = ref(false)
  const intervalMs = options.intervalMs ?? 3000
  let timer: ReturnType<typeof setInterval> | null = null
  let inFlight = false

  const isDocumentHidden = () => typeof document !== 'undefined' && document.hidden
  const isEnabled = () => options.enabled?.() !== false
  const canPoll = () => isEnabled() && !isDocumentHidden() && options.hasActiveItems()

  const clearTimer = () => {
    if (timer) {
      clearInterval(timer)
      timer = null
    }
  }

  const stop = () => {
    clearTimer()
    isPolling.value = false
  }

  const runOnce = async () => {
    if (inFlight || !canPoll()) {
      if (!canPoll()) {
        stop()
      }
      return
    }

    inFlight = true
    try {
      await options.poll()
    } catch (error) {
      console.warn('[FocusedPolling] poll failed:', error)
    } finally {
      inFlight = false
      if (!canPoll()) {
        stop()
      }
    }
  }

  const start = () => {
    if (!canPoll()) {
      stop()
      return
    }
    if (!timer) {
      timer = setInterval(() => {
        void runOnce()
      }, intervalMs)
    }
    isPolling.value = true
  }

  const trigger = async () => {
    if (!canPoll()) {
      stop()
      return
    }
    start()
    await runOnce()
  }

  const handleVisibilityChange = () => {
    if (isDocumentHidden()) {
      stop()
      return
    }
    void trigger()
  }

  onMounted(() => {
    if (typeof document !== 'undefined') {
      document.addEventListener('visibilitychange', handleVisibilityChange)
    }
    start()
  })

  onUnmounted(() => {
    if (typeof document !== 'undefined') {
      document.removeEventListener('visibilitychange', handleVisibilityChange)
    }
    stop()
  })

  return {
    start,
    stop,
    trigger,
    isPolling
  }
}
