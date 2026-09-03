export interface Failure {
  occurredAt: string
  source: string
  message: string
}

export interface FailureList {
  failures: Failure[]
  total: number
}

export interface FailureListParams {
  offset: number
  limit?: number
  source?: string
  since?: string
  until?: string
}

export const FAILURE_SOURCES = [
  'webhook',
  'email',
  'refresh',
  'discovery',
  'event_registration',
] as const

export type FailureSource = (typeof FAILURE_SOURCES)[number]

export const FAILURE_SOURCE_LABELS: Record<string, string> = {
  webhook: 'Webhook delivery',
  email: 'Email delivery',
  refresh: 'Tag refresh',
  discovery: 'Tag discovery',
  event_registration: 'Event registration',
}
