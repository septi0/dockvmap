export interface ImageEvent {
  id: number
  imageId: number
  imageName: string
  type: string
  data: { tags: string[] }
  createdAt: string
  notify: boolean
  notifSentAt?: string
}

export const TAG_EVENT_TYPES = ['TAG_ADDED', 'TAG_REMOVED', 'UPGRADE_AVAILABLE'] as const

export type TagEventType = (typeof TAG_EVENT_TYPES)[number]

export interface ImageEventList {
  events: ImageEvent[]
  total: number
}

export interface ImageEventListParams {
  offset: number
  limit?: number
  type?: string
  imageId?: number
  since?: string
  until?: string
}
