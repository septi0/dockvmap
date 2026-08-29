import { tagRefreshStatusStore } from '../stores/tagRefreshStatus'
import { getTagRefreshStatus } from '../api/worker'
import { ApiError } from '../api/client'
import { createPoller } from '../utils/poller'

const POLL_INTERVAL_MS = 2000
const TRIGGER_GRACE_MS = 8000

let watchers = 0
let graceUntil = 0

async function load(): Promise<boolean> {
  try {
    const data = await getTagRefreshStatus()
    tagRefreshStatusStore.set({ data, loading: false, error: null })
    return data.running
  } catch (err) {
    tagRefreshStatusStore.update((state) => ({
      ...state,
      loading: false,
      error: err instanceof ApiError ? err.message : 'Failed to load tag check status',
    }))
    return false
  }
}

function keepPolling(running: boolean): boolean {
  return watchers > 0 && (running || Date.now() < graceUntil)
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
  ensurePolling()
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

export const tagRefreshStatus = {
  subscribe: tagRefreshStatusStore.subscribe,
  refresh,
  watch,
  notifyTriggered,
}
