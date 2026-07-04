import { onMounted, onUnmounted, ref } from 'vue'

interface FocusedPollingOptions {
  poll: () => Promise<void>
  hasActiveItems: () => boolean
  intervalMs?: number
  enabled?: () => boolean
  maxConsecutiveFailures?: number
  maxBackoffMs?: number
}

export function useFocusedPolling(options: FocusedPollingOptions) {
  const isPolling = ref(false)
  const intervalMs = options.intervalMs ?? 3000
  const maxConsecutiveFailures = options.maxConsecutiveFailures ?? 3
  const maxBackoffMs = options.maxBackoffMs ?? intervalMs * 8
  let timer: ReturnType<typeof setTimeout> | null = null
  let inFlight = false
  let consecutiveFailures = 0

  const isDocumentHidden = () => typeof document !== 'undefined' && document.hidden
  const isEnabled = () => options.enabled?.() !== false
  const canPoll = () => isEnabled() && !isDocumentHidden() && options.hasActiveItems()

  const clearTimer = () => {
    if (timer) {
      clearTimeout(timer)
      timer = null
    }
  }

  const stop = () => {
    clearTimer()
    isPolling.value = false
  }

  const nextDelay = () => {
    if (consecutiveFailures <= 0) return intervalMs
    return Math.min(intervalMs * (2 ** consecutiveFailures), maxBackoffMs)
  }

  const scheduleNext = () => {
    clearTimer()
    if (!canPoll() || consecutiveFailures >= maxConsecutiveFailures) {
      stop()
      return
    }
    timer = setTimeout(() => {
      timer = null
      void runOnce()
    }, nextDelay())
    isPolling.value = true
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
      consecutiveFailures = 0
    } catch (error) {
      consecutiveFailures += 1
      console.warn('[FocusedPolling] poll failed:', error)
    } finally {
      inFlight = false
      if (!canPoll()) {
        stop()
      } else if (consecutiveFailures >= maxConsecutiveFailures) {
        stop()
      } else {
        scheduleNext()
      }
    }
  }

  const start = () => {
    if (!canPoll()) {
      stop()
      return
    }
    if (!timer) {
      scheduleNext()
    }
    isPolling.value = true
  }

  const trigger = async () => {
    if (!canPoll()) {
      stop()
      return
    }
    clearTimer()
    isPolling.value = true
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
