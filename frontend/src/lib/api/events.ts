import { api } from './client'
import { buildQuery } from '../utils/query'
import type { ListEventsResponse } from './types/events'

export function listEvents(offset: number) {
  return api.get<ListEventsResponse>(`/events${buildQuery({ offset })}`)
}
