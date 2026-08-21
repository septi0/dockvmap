export function debounce<Args extends unknown[]>(fn: (...args: Args) => void, delayMs: number) {
  let timeout: ReturnType<typeof setTimeout> | undefined

  return (...args: Args) => {
    clearTimeout(timeout)
    timeout = setTimeout(() => fn(...args), delayMs)
  }
}
