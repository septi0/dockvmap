import { writable } from 'svelte/store'

export type Theme = 'light' | 'dark' | 'system'

export const themeStore = writable<Theme>('system')
