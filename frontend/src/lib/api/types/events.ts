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

export interface ListEventsResponse {
  events: ImageEvent[]
  hasMore: boolean
  nextOffset: number
}
