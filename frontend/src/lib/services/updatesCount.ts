import { updatesCountStore } from '../stores/updatesCount'
import { listImages } from '../api/images'
import { listImageGroups } from '../api/imageGroups'
import { createVisibilityPoller } from '../utils/visibilityPoller'

const POLL_INTERVAL_MS = 60_000

async function fetchCount(): Promise<number | null> {
  try {
    const [images, groups] = await Promise.all([
      listImages({ offset: 0, limit: 1, status: 'updateAvailable', ungrouped: true }),
      listImageGroups({ limit: 1, status: 'updateAvailable' }),
    ])

    return images.total + groups.total
  } catch {
    return null
  }
}

async function refresh(): Promise<void> {
  const count = await fetchCount()

  if (count !== null) updatesCountStore.set(count)
}

const poller = createVisibilityPoller({ tick: refresh, intervalMs: POLL_INTERVAL_MS })

export const updatesCount = {
  subscribe: updatesCountStore.subscribe,
  start: poller.start,
  refresh,
}
