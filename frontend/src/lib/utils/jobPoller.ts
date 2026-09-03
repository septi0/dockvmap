import { createPoller } from './poller'

interface JobPollerOptions<T> {
  poll: () => Promise<T>
  running: (result: T) => boolean
  intervalMs: number
  maxConsecutiveErrors?: number
  onResult?: (result: T) => void
  onSettled?: (result: T) => void
  onError?: (err: unknown, consecutive: number, gaveUp: boolean) => void
}

export function createJobPoller<T>(options: JobPollerOptions<T>) {
  const maxErrors = options.maxConsecutiveErrors ?? 3

  let consecutiveErrors = 0
  let wasRunning = false

  const poller = createPoller(async () => {
    let result: T

    try {
      result = await options.poll()
    } catch (err) {
      consecutiveErrors += 1
      const gaveUp = consecutiveErrors >= maxErrors
      options.onError?.(err, consecutiveErrors, gaveUp)

      return !gaveUp
    }

    consecutiveErrors = 0
    options.onResult?.(result)

    const running = options.running(result)

    if (wasRunning && !running) options.onSettled?.(result)
    wasRunning = running

    return running
  }, options.intervalMs)

  return {
    start() {
      if (poller.active) return

      consecutiveErrors = 0
      wasRunning = false
      poller.start()
    },
    stop: poller.stop,
    get active() {
      return poller.active
    },
  }
}
