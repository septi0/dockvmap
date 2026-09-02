import { api } from './client'
import type { PullInfo } from './types/metrics'

export function getPullInfo() {
  return api.get<PullInfo>('/proxy/pull-info')
}
