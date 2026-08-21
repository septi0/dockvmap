import { themeStore } from '../stores/theme'
import type { Theme } from '../stores/theme'

const STORAGE_KEY = 'dockvmap-theme'

function apply(value: Theme) {
  if (value === 'system') {
    delete document.documentElement.dataset.theme
  } else {
    document.documentElement.dataset.theme = value
  }
}

function init() {
  const stored = localStorage.getItem(STORAGE_KEY)
  const value: Theme = stored === 'light' || stored === 'dark' ? stored : 'system'

  themeStore.set(value)
  apply(value)
}

function setTheme(value: Theme) {
  themeStore.set(value)
  apply(value)
  localStorage.setItem(STORAGE_KEY, value)
}

export const theme = {
  subscribe: themeStore.subscribe,
  init,
  setTheme,
}
