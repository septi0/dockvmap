import { writable } from 'svelte/store'

export const updatesCountStore = writable<number>(0)
