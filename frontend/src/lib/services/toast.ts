import { toastStore } from '../stores/toast'

const DISMISS_AFTER_MS = 4000
const MAX_VISIBLE = 3

let nextId = 1

function dismiss(id: number) {
  toastStore.update((toasts) => toasts.filter((toast) => toast.id !== id))
}

function success(message: string) {
  const id = nextId++
  toastStore.update((toasts) => [...toasts, { id, message }].slice(-MAX_VISIBLE))
  setTimeout(() => dismiss(id), DISMISS_AFTER_MS)
}

export const toast = {
  subscribe: toastStore.subscribe,
  success,
  dismiss,
}
