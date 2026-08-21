import { updatesCountStore } from '../stores/updatesCount'
import { listImages } from '../api/images'

async function refresh() {
  try {
    const result = await listImages({ offset: 0, limit: 1, updateAvailable: true })
    updatesCountStore.set(result.total)
  } catch {}
}

export const updatesCount = {
  subscribe: updatesCountStore.subscribe,
  refresh,
}
