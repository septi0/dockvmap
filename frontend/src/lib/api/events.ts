import { api } from './client'
import { buildQuery } from '../utils/query'
import type { ImageEventListParams, ImageEventList } from './types/events'

export const TAG_ACTIVITY_PAGE_SIZE = 25

export function listTagEvents(params: ImageEventListParams) {
  return api.get<ImageEventList>(`/events${buildQuery(params)}`)
}
