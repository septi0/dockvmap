import { api } from './client'

export function updatePassword(currentPassword: string, newPassword: string) {
  return api.put<{ status: string }>(
    '/users/password',
    { currentPassword, newPassword },
  )
}

export function updateEmail(email: string) {
  return api.put<{ status: string }>('/users/email', { email })
}

export interface UpdateUserPreferences {
  notifyNewTags?: boolean
}

export function updatePreferences(preferences: UpdateUserPreferences) {
  return api.put<{ status: string }>('/users/preferences', preferences)
}
