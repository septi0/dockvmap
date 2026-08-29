import { api } from './client'
import type { TagRefreshStatus } from './types/worker'

export function getTagRefreshStatus() {
  return api.get<TagRefreshStatus>('/tag-refresh-status')
}

export function triggerTagRefresh() {
  return api.post<{ status: string }>('/tag-refresh')
}
