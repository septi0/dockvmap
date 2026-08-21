import { api } from './client'
import type { CurrentUser } from './types/auth'

export function getSetupStatus() {
  return api.get<{ required: boolean }>('/setup')
}

export function bootstrapUser(username: string, email: string, password: string) {
  return api.post<{ id: number; status: string }>('/setup', { username, email, password })
}

export function login(username: string, password: string) {
  return api.post<{ status: string }>(
    '/login',
    { username, password },
    { expectUnauthorized: true },
  )
}

export function logout() {
  return api.post<{ status: string }>('/logout')
}

export function getCurrentUser(options?: { expectUnauthorized?: boolean }) {
  return api.get<CurrentUser>('/me', options)
}
