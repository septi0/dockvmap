import { api } from './client'
import type { Session } from './types/sessions'

export function listSessions() {
  return api.get<{ sessions: Session[] }>('/sessions')
}

export function invalidateSession(id: number) {
  return api.delete<{ status: string }>(`/sessions/${id}`)
}
