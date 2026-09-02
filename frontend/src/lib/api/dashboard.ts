import { api } from './client'
import type { DashboardSummary } from './types/dashboard'

export function getDashboardSummary() {
  return api.get<DashboardSummary>('/dashboard/summary')
}
