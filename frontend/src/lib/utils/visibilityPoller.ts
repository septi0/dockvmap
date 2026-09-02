import { createPoller } from './poller'

interface VisibilityPollerOptions {
  tick: () => Promise<void>
  intervalMs: number
  refetchOnResume?: () => boolean
}

export function createVisibilityPoller({
  tick,
  intervalMs,
  refetchOnResume,
}: VisibilityPollerOptions) {
  let active = false

  function visible(): boolean {
    return typeof document === 'undefined' || document.visibilityState !== 'hidden'
  }

  function running(): boolean {
    return active && visible()
  }

  const poller = createPoller(async () => {
    if (!running()) return false

    await tick()

    return running()
  }, intervalMs)

  function onVisibilityChange() {
    if (!active) return

    if (!visible()) {
      poller.stop()
      return
    }

    if (!refetchOnResume || refetchOnResume()) void tick()

    poller.start()
  }

  function reschedule() {
    if (running()) poller.start()
  }

  function start(): () => void {
    active = true

    void tick()
    poller.start()
    document.addEventListener('visibilitychange', onVisibilityChange)

    return () => {
      active = false
      poller.stop()
      document.removeEventListener('visibilitychange', onVisibilityChange)
    }
  }

  return { start, reschedule }
}
