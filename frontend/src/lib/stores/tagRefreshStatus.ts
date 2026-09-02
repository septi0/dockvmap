import { writable } from 'svelte/store'
import type { TagRefreshStatus } from '../api/types/worker'

export interface TagRefreshStatusState {
  data: TagRefreshStatus | null
  loading: boolean
  error: string | null
  unavailable: boolean
}

export const tagRefreshStatusStore = writable<TagRefreshStatusState>({
  data: null,
  loading: true,
  error: null,
  unavailable: false,
})
