export function createPoller(tick: () => Promise<boolean>, intervalMs: number) {
  let generation = 0
  let timer: ReturnType<typeof setTimeout> | null = null

  function stop() {
    generation += 1
    if (timer !== null) {
      clearTimeout(timer)
      timer = null
    }
  }

  function start() {
    stop()
    const mine = generation

    const run = async () => {
      if (mine !== generation) return

      let again = false
      try {
        again = await tick()
      } catch {
        again = false
      }

      if (mine !== generation) return
      timer = again ? setTimeout(run, intervalMs) : null
    }

    timer = setTimeout(run, intervalMs)
  }

  return {
    start,
    stop,
    get active() {
      return timer !== null
    },
  }
}
