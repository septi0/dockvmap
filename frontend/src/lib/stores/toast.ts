import { writable } from 'svelte/store'

export interface Toast {
  id: number
  message: string
}

export const toastStore = writable<Toast[]>([])
