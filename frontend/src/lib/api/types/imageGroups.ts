import type { Image } from './images'

export interface ImageGroupPartialTag {
  tag: string
  missing: string[]
}

export interface ImageGroup {
  id: number
  name: string
  tag: string
  pinned: boolean
  updateAvailable: boolean
  updateAvailableTag?: string
  partialTag?: ImageGroupPartialTag
  images: Image[]
  createdAt: string
  updatedAt: string
}

export interface ListImageGroupsResponse {
  groups: ImageGroup[]
  total: number
}

export interface ImageGroupListParams {
  offset?: number
  limit?: number
  search?: string
  status?: 'updateAvailable' | 'pinned'
}

export interface SaveImageGroupParams {
  name: string
  imageIds: number[]
  tag: string
}
