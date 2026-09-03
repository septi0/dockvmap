import { writable } from 'svelte/store'
import { inspectRepository, getDiscovery } from '../api/images'
import { errorMessage } from '../api/client'
import { createJobPoller } from '../utils/jobPoller'
import type { DiscoveryResult } from '../api/types/images'

const POLL_INTERVAL_MS = 1000
const MAX_CONSECUTIVE_POLL_ERRORS = 3

export type TagDiscoveryPhase = 'idle' | 'inspecting' | 'discovering'

export interface TagDiscoveryState {
  phase: TagDiscoveryPhase
  elapsedSeconds: number
  tagsSeen: number
  error: string | null
}

const initialState: TagDiscoveryState = {
  phase: 'idle',
  elapsedSeconds: 0,
  tagsSeen: 0,
  error: null,
}

export interface TagDiscoveryParams {
  registry: string
  repository: string
}

export function createTagDiscovery() {
  const store = writable<TagDiscoveryState>({ ...initialState })

  const resolvedListeners = new Set<(result: DiscoveryResult) => void>()

  let discoveryId: number | null = null
  let elapsedTimer: ReturnType<typeof setInterval> | null = null

  function startElapsedTimer() {
    stopElapsedTimer()
    store.update((state) => ({ ...state, elapsedSeconds: 0 }))
    elapsedTimer = setInterval(() => {
      store.update((state) => ({ ...state, elapsedSeconds: state.elapsedSeconds + 1 }))
    }, 1000)
  }

  function stopElapsedTimer() {
    if (elapsedTimer === null) return

    clearInterval(elapsedTimer)
    elapsedTimer = null
  }

  function settle(result: DiscoveryResult) {
    stopElapsedTimer()
    discoveryId = null

    if (result.status === 'failed') {
      store.update((state) => ({
        ...state,
        phase: 'idle',
        error: result.error || 'Tag discovery failed',
      }))

      return
    }

    store.update((state) => ({ ...state, phase: 'idle' }))

    for (const listener of resolvedListeners) listener(result)
  }

  const poll = createJobPoller<DiscoveryResult>({
    poll: () => getDiscovery(discoveryId as number),
    running: (result) => result.status === 'running',
    intervalMs: POLL_INTERVAL_MS,
    maxConsecutiveErrors: MAX_CONSECUTIVE_POLL_ERRORS,
    onResult: (result) => {
      if (result.status === 'running') {
        store.update((state) => ({ ...state, tagsSeen: result.tagsSeen ?? state.tagsSeen }))
      } else {
        settle(result)
      }
    },
    onError: (err, _consecutive, gaveUp) => {
      if (!gaveUp) return

      stopElapsedTimer()
      discoveryId = null
      store.update((state) => ({
        ...state,
        phase: 'idle',
        error: errorMessage(err, 'Failed to check discovery status'),
      }))
    },
  })

  async function start({ registry, repository }: TagDiscoveryParams) {
    poll.stop()
    stopElapsedTimer()
    discoveryId = null

    store.set({ ...initialState, phase: 'inspecting' })

    try {
      const result = await inspectRepository({ registry, repository })

      if (result.status !== 'running') {
        settle(result)

        return
      }

      discoveryId = result.id
      store.update((state) => ({
        ...state,
        phase: 'discovering',
        tagsSeen: result.tagsSeen ?? 0,
      }))
      startElapsedTimer()
      poll.start()
    } catch (err) {
      store.update((state) => ({
        ...state,
        phase: 'idle',
        error: errorMessage(err, 'Failed to inspect repository'),
      }))
    }
  }

  function cancel() {
    poll.stop()
    stopElapsedTimer()
    discoveryId = null
    store.update((state) => ({ ...state, phase: 'idle' }))
  }

  function destroy() {
    cancel()
    resolvedListeners.clear()
    store.set({ ...initialState })
  }

  function onResolved(listener: (result: DiscoveryResult) => void): () => void {
    resolvedListeners.add(listener)

    return () => {
      resolvedListeners.delete(listener)
    }
  }

  return {
    subscribe: store.subscribe,
    start,
    cancel,
    destroy,
    onResolved,
  }
}
