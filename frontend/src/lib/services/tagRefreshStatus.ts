import { tagRefreshStatusStore } from '../stores/tagRefreshStatus'
import { getTagRefreshStatus } from '../api/worker'
import { errorMessage } from '../api/client'
import { createJobPoller } from '../utils/jobPoller'
import type { TagRefreshStatus } from '../api/types/worker'

const POLL_INTERVAL_MS = 2000
const MAX_CONSECUTIVE_POLL_ERRORS = 3

let watchers = 0
let inflight: Promise<TagRefreshStatus> | null = null

const completionListeners = new Set<() => void>()

async function fetchStatus(): Promise<TagRefreshStatus> {
  try {
    const data = await getTagRefreshStatus()
    tagRefreshStatusStore.set({ data, loading: false, error: null, unavailable: false })

    return data
  } catch (err) {
    tagRefreshStatusStore.update((state) => ({
      ...state,
      loading: false,
      error: errorMessage(err, 'Failed to load tag check status'),
    }))

    throw err
  }
}

function load(): Promise<TagRefreshStatus> {
  if (!inflight) {
    inflight = fetchStatus().finally(() => {
      inflight = null
    })
  }

  return inflight
}

const poller = createJobPoller<TagRefreshStatus>({
  poll: load,
  running: (data) => watchers > 0 && data.running,
  intervalMs: POLL_INTERVAL_MS,
  maxConsecutiveErrors: MAX_CONSECUTIVE_POLL_ERRORS,
  onError: (_err, _consecutive, gaveUp) => {
    if (gaveUp) {
      tagRefreshStatusStore.update((state) => ({ ...state, unavailable: true }))
    }
  },
  onSettled: () => {
    for (const listener of completionListeners) listener()
  },
})

async function refresh() {
  try {
    const data = await load()

    if (watchers > 0 && data.running) poller.start()
  } catch {
    // fetchStatus already surfaced the error into the store
  }
}

function notifyTriggered() {
  void refresh()
}

function watch(): () => void {
  watchers += 1
  void refresh()

  return () => {
    watchers = Math.max(0, watchers - 1)
    if (watchers === 0) poller.stop()
  }
}

function onCompleted(listener: () => void): () => void {
  completionListeners.add(listener)

  return () => {
    completionListeners.delete(listener)
  }
}

export const tagRefreshStatus = {
  subscribe: tagRefreshStatusStore.subscribe,
  refresh,
  watch,
  notifyTriggered,
  onCompleted,
}

export type { TagRefreshStatusState } from '../stores/tagRefreshStatus'
