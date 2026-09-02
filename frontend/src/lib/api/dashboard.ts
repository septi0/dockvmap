import { api } from './client'
import type { Dashboard } from './types/dashboard'

export function getDashboard() {
  return api.get<Dashboard>('/dashboard')
}
