export type AsyncStateKind = 'loading' | 'error' | 'empty' | 'content'

export interface AsyncStateInput {
  loading: boolean
  hasError: boolean
  empty?: boolean
  busy?: boolean
}

export interface ResolvedAsyncState {
  kind: AsyncStateKind
  busy: boolean
}

export function resolveAsyncState({
  loading,
  hasError,
  empty = false,
  busy = false,
}: AsyncStateInput): ResolvedAsyncState {
  if (loading) return { kind: 'loading', busy: false }
  if (hasError) return { kind: 'error', busy: false }
  if (empty) return { kind: 'empty', busy: false }

  return { kind: 'content', busy }
}
