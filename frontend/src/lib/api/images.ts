import { api } from './client'
import { buildQuery } from '../utils/query'
import type {
  Image,
  ImageListParams,
  ListImagesResponse,
  InspectRepositoryParams,
  DiscoveryResult,
  CreateImageParams,
  ImageTagsResult,
  TagHistoryResult,
  TagHistorySource,
} from './types/images'

export const IMAGES_PAGE_SIZE = 25

export function listImages(params: ImageListParams) {
  return api.get<ListImagesResponse>(`/images${buildQuery(params)}`)
}

export function inspectRepository(params: InspectRepositoryParams) {
  return api.post<DiscoveryResult>('/images/inspect', params)
}

export function getDiscovery(id: number) {
  return api.get<DiscoveryResult>(`/discoveries/${id}`)
}

export function createImage(params: CreateImageParams) {
  return api.post<{ status: string; refreshSuccessful: boolean }>('/images', params)
}

export function getImage(id: number) {
  return api.get<Image>(`/images/${id}`)
}

export function getImageTags(id: number) {
  return api.get<ImageTagsResult>(`/images/${id}/tags`)
}

export function updateImageTag(id: number, tag: string, source?: TagHistorySource) {
  return api.put<{ status: string }>(`/images/${id}/tag`, { tag, source })
}

export function getTagHistory(id: number) {
  return api.get<TagHistoryResult>(`/images/${id}/tag-history`)
}

export function renameImage(id: number, name: string) {
  return api.put<{ status: string }>(`/images/${id}/name`, { name })
}

export function setImagePin(id: number, pinned: boolean) {
  return api.put<{ status: string }>(`/images/${id}/pin`, { pinned })
}

export function refreshImageTags(id: number) {
  return api.post<{ status: 'refreshed' | 'running' | 'error'; error?: string }>(
    `/images/${id}/refresh-tags`,
  )
}

export function markImageTagsAsSeen(id: number) {
  return api.post<{ status: string; tagsMarkedAsSeen: number }>(`/images/${id}/mark-seen`)
}

export function deleteImage(id: number) {
  return api.delete<{ status: string }>(`/images/${id}`)
}
