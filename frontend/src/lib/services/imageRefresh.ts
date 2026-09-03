import { writable } from 'svelte/store'
import { getImage } from '../api/images'
import { createJobPoller } from '../utils/jobPoller'
import type { Image } from '../api/types/images'

const POLL_INTERVAL_MS = 2000
const MAX_CONSECUTIVE_ERRORS = 15

export type ImageRefreshStatus = 'idle' | 'running' | 'stale'

export function applyImageRefreshFields(target: Image, source: Image): void {
  target.refreshStatus = source.refreshStatus
  target.tag = source.tag
  target.lastChecked = source.lastChecked
  target.lastCheckError = source.lastCheckError
  target.updateAvailable = source.updateAvailable
  target.updateAvailableTag = source.updateAvailableTag
  target.updatedAt = source.updatedAt
}

interface ImageRefreshOptions {
  onResult?: (image: Image) => void
}

export function createImageRefresh(getImageId: () => number, options: ImageRefreshOptions = {}) {
  const status = writable<ImageRefreshStatus>('idle')
  const settledListeners = new Set<() => void>()

  const poller = createJobPoller<Image>({
    poll: () => getImage(getImageId()),
    running: (img) => img.refreshStatus === 'running',
    intervalMs: POLL_INTERVAL_MS,
    maxConsecutiveErrors: MAX_CONSECUTIVE_ERRORS,
    onResult: (img) => {
      if (img.refreshStatus === 'running') {
        status.set('running')
        return
      }

      options.onResult?.(img)
    },
    onSettled: () => {
      status.set('idle')
      for (const listener of settledListeners) listener()
    },
    onError: (_err, _consecutive, gaveUp) => {
      if (gaveUp) status.set('stale')
    },
  })

  function sync(image: Image | null) {
    const running = image?.refreshStatus === 'running'

    if (poller.active) return

    if (running) {
      status.set('running')
      poller.start()
    } else {
      status.set('idle')
    }
  }

  function reset() {
    poller.stop()
    status.set('idle')
  }

  function onSettled(listener: () => void): () => void {
    settledListeners.add(listener)

    return () => {
      settledListeners.delete(listener)
    }
  }

  function destroy() {
    poller.stop()
    settledListeners.clear()
  }

  return { subscribe: status.subscribe, sync, reset, onSettled, destroy }
}
