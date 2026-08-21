import { writable } from 'svelte/store'
import type { CurrentUser } from '../api/types/auth'

export type AuthStatus = 'loading' | 'setup-required' | 'unauthenticated' | 'authenticated'

export interface AuthState {
  status: AuthStatus
  user: CurrentUser | null
}

export const authStore = writable<AuthState>({ status: 'loading', user: null })
