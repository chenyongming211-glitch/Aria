import { mount } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useFocusedPolling } from '@/composables/useFocusedPolling'

function deferred() {
  let resolve!: () => void
  const promise = new Promise<void>((res) => {
    resolve = res
  })
  return { promise, resolve }
}

function mountPolling(options: {
  active?: boolean
  hidden?: boolean
  poll?: () => Promise<void>
  enabled?: () => boolean
} = {}) {
  const active = ref(options.active ?? true)
  const poll = vi.fn(options.poll || (() => Promise.resolve()))
  let controls!: ReturnType<typeof useFocusedPolling>
  let hidden = options.hidden ?? false

  vi.spyOn(document, 'hidden', 'get').mockImplementation(() => hidden)

  const wrapper = mount(defineComponent({
    setup() {
      controls = useFocusedPolling({
        poll,
        hasActiveItems: () => active.value,
        intervalMs: 1000,
        enabled: options.enabled
      })
      return () => null
    }
  }))

  return {
    wrapper,
    active,
    poll,
    controls: () => controls,
    setHidden(value: boolean) {
      hidden = value
      document.dispatchEvent(new Event('visibilitychange'))
    }
  }
}

describe('useFocusedPolling', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('starts when active items exist', async () => {
    const { poll, controls, wrapper } = mountPolling({ active: true })

    expect(controls().isPolling.value).toBe(true)

    await vi.advanceTimersByTimeAsync(1000)
    expect(poll).toHaveBeenCalledTimes(1)

    wrapper.unmount()
  })

  it('does not start when no active items exist', async () => {
    const { poll, controls, wrapper } = mountPolling({ active: false })

    expect(controls().isPolling.value).toBe(false)

    await vi.advanceTimersByTimeAsync(3000)
    expect(poll).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('prevents overlapping requests', async () => {
    const pending = deferred()
    const { poll, wrapper } = mountPolling({ poll: () => pending.promise })

    await vi.advanceTimersByTimeAsync(1000)
    await vi.advanceTimersByTimeAsync(1000)

    expect(poll).toHaveBeenCalledTimes(1)

    pending.resolve()
    await vi.runOnlyPendingTimersAsync()

    wrapper.unmount()
  })

  it('pauses when document is hidden', async () => {
    const { poll, controls, setHidden, wrapper } = mountPolling()

    setHidden(true)
    expect(controls().isPolling.value).toBe(false)

    await vi.advanceTimersByTimeAsync(2000)
    expect(poll).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('resumes and immediately polls when document becomes visible', async () => {
    const { poll, controls, setHidden, wrapper } = mountPolling({ hidden: true })

    expect(controls().isPolling.value).toBe(false)

    setHidden(false)
    await Promise.resolve()

    expect(controls().isPolling.value).toBe(true)
    expect(poll).toHaveBeenCalledTimes(1)

    wrapper.unmount()
  })

  it('stops when active items become terminal', async () => {
    const { active, poll, controls, wrapper } = mountPolling()

    poll.mockImplementation(async () => {
      active.value = false
    })

    await vi.advanceTimersByTimeAsync(1000)

    expect(poll).toHaveBeenCalledTimes(1)
    expect(controls().isPolling.value).toBe(false)

    wrapper.unmount()
  })

  it('backs off and stops after repeated poll failures', async () => {
    const { poll, controls, wrapper } = mountPolling({
      poll: async () => {
        throw Object.assign(new Error('server unavailable'), { response: { status: 500 } })
      }
    })

    await vi.advanceTimersByTimeAsync(1000)
    expect(poll).toHaveBeenCalledTimes(1)
    expect(controls().isPolling.value).toBe(true)

    await vi.advanceTimersByTimeAsync(1999)
    expect(poll).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(1)
    expect(poll).toHaveBeenCalledTimes(2)

    await vi.advanceTimersByTimeAsync(3999)
    expect(poll).toHaveBeenCalledTimes(2)

    await vi.advanceTimersByTimeAsync(1)
    expect(poll).toHaveBeenCalledTimes(3)
    expect(controls().isPolling.value).toBe(false)

    await vi.advanceTimersByTimeAsync(10000)
    expect(poll).toHaveBeenCalledTimes(3)

    wrapper.unmount()
  })
})
