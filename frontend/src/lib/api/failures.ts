import { api } from './client'
import type { RecentFailure } from './types/failures'

export function listRecentFailures() {
  return api.get<RecentFailure[]>('/recent-failures')
}
