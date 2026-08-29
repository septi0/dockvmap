import { replace } from 'svelte-spa-router'
import { authStore } from '../stores/auth'
import { setUnauthorizedHandler } from '../api/client'
import * as authApi from '../api/auth'

setUnauthorizedHandler(() => {
  authStore.set({ status: 'unauthenticated', user: null })
  replace('/login')
})

async function init() {
  authStore.set({ status: 'loading', user: null })

  const { required } = await authApi.getSetupStatus()

  if (required) {
    authStore.set({ status: 'setup-required', user: null })
    return
  }

  try {
    const user = await authApi.getCurrentUser({ expectUnauthorized: true })
    authStore.set({ status: 'authenticated', user })
  } catch {
    authStore.set({ status: 'unauthenticated', user: null })
  }
}

async function login(username: string, password: string) {
  await authApi.login(username, password)
  const user = await authApi.getCurrentUser()
  authStore.set({ status: 'authenticated', user })
}

async function bootstrap(username: string, email: string, password: string) {
  await authApi.bootstrapUser(username, email, password)
  await login(username, password)
}

async function refresh() {
  const user = await authApi.getCurrentUser()
  authStore.update((state) => ({ ...state, user }))
}

async function logout() {
  try {
    await authApi.logout()
  } catch {
    // best-effort server call; the client is signed out regardless
  } finally {
    authStore.set({ status: 'unauthenticated', user: null })
  }
}

export const auth = {
  subscribe: authStore.subscribe,
  init,
  login,
  bootstrap,
  logout,
  refresh,
}
