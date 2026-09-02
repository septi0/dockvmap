import { dashboardRefreshStore } from '../stores/dashboardRefresh'

/**
 * Coordinates a single "refresh everything" action across the dashboard cards
 * without unmounting them. Cards subscribe to `nonce` and refetch in place when
 * it changes; they bracket every fetch with `begin`/`end` so the header can show
 * a real progress state, an accurate "updated" timestamp, and whether any card
 * failed to load.
 */

function requestRefresh() {
  dashboardRefreshStore.update((state) => ({ ...state, nonce: state.nonce + 1 }))
}

function begin(id: string) {
  dashboardRefreshStore.update((state) =>
    state.pending.includes(id)
      ? state
      : { ...state, pending: [...state.pending, id] },
  )
}

function end(id: string, ok = true) {
  dashboardRefreshStore.update((state) => {
    const pending = state.pending.filter((entry) => entry !== id)

    const errored = ok
      ? state.errored.filter((entry) => entry !== id)
      : state.errored.includes(id)
        ? state.errored
        : [...state.errored, id]

    return {
      ...state,
      pending,
      errored,
      settledAt: pending.length === 0 ? Date.now() : state.settledAt,
    }
  })
}

/** Called when the dashboard route unmounts, so a later visit starts clean. */
function reset() {
  dashboardRefreshStore.set({
    nonce: 0,
    pending: [],
    errored: [],
    settledAt: null,
  })
}

export const dashboardRefresh = {
  subscribe: dashboardRefreshStore.subscribe,
  requestRefresh,
  begin,
  end,
  reset,
}
