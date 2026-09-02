import { tagRefreshStatusStore } from '../stores/tagRefreshStatus'
import { getTagRefreshStatus } from '../api/worker'
import { errorMessage } from '../api/client'
import { createPoller } from '../utils/poller'

const POLL_INTERVAL_MS = 2000
const TRIGGER_GRACE_MS = 8000
const MAX_CONSECUTIVE_POLL_ERRORS = 3

let watchers = 0
let graceUntil = 0
let consecutiveErrors = 0
let wasRunning = false
let inflight: Promise<LoadResult> | null = null

const completionListeners = new Set<() => void>()

interface LoadResult {
  running: boolean
  failed: boolean
}

function announceCompletion(running: boolean) {
  if (wasRunning && !running) {
    for (const listener of completionListeners) listener()
  }

  wasRunning = running
}

async function fetchStatus(): Promise<LoadResult> {
  try {
    const data = await getTagRefreshStatus()

    consecutiveErrors = 0
    tagRefreshStatusStore.set({ data, loading: false, error: null, unavailable: false })
    announceCompletion(data.running)

    return { running: data.running, failed: false }
  } catch (err) {
    consecutiveErrors += 1

    tagRefreshStatusStore.update((state) => ({
      ...state,
      loading: false,
      error: errorMessage(err, 'Failed to load tag check status'),
      unavailable: consecutiveErrors >= MAX_CONSECUTIVE_POLL_ERRORS,
    }))

    return { running: false, failed: true }
  }
}

function load(): Promise<LoadResult> {
  if (inflight) return inflight

  inflight = fetchStatus().finally(() => {
    inflight = null
  })

  return inflight
}

function keepPolling(result: LoadResult): boolean {
  if (watchers === 0) return false
  if (result.failed) return consecutiveErrors < MAX_CONSECUTIVE_POLL_ERRORS

  return result.running || Date.now() < graceUntil
}

const poller = createPoller(async () => keepPolling(await load()), POLL_INTERVAL_MS)

function ensurePolling() {
  if (watchers > 0 && !poller.active) poller.start()
}

async function refresh() {
  if (keepPolling(await load())) ensurePolling()
}

// grace window: the worker may not have set its running flag by the time this fires
function notifyTriggered() {
  graceUntil = Date.now() + TRIGGER_GRACE_MS
  consecutiveErrors = 0
  ensurePolling()
  void refresh()
}

function watch(): () => void {
  watchers += 1

  if (watchers === 1) consecutiveErrors = 0

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
