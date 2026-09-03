import { api } from './client'
import { buildQuery } from '../utils/query'
import type { FailureList, FailureListParams } from './types/failures'

export const FAILURES_PAGE_SIZE = 25

export function listFailures(params: FailureListParams) {
  return api.get<FailureList>(`/failures${buildQuery(params)}`)
}
