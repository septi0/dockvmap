import { api } from './client'
import { buildQuery } from '../utils/query'
import type {
  Image,
  ImageListParams,
  ListImagesResponse,
  InspectRepositoryParams,
  InspectRepositoryResult,
  CreateImageParams,
  ImageTagsResult,
} from './types/images'

export const IMAGES_PAGE_SIZE = 25

export function listImages(params: ImageListParams) {
  return api.get<ListImagesResponse>(`/images${buildQuery(params)}`)
}

export function inspectRepository(params: InspectRepositoryParams) {
  return api.post<InspectRepositoryResult>('/images/inspect', params)
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

export function updateImageTag(id: number, tag: string) {
  return api.put<{ status: string }>(`/images/${id}/tag`, { tag })
}

export function renameImage(id: number, name: string) {
  return api.put<{ status: string }>(`/images/${id}/name`, { name })
}

export function refreshImageTags(id: number) {
  return api.post<{ status: string; eventRegistered: boolean }>(`/images/${id}/refresh-tags`)
}

export function markImageTagsAsSeen(id: number) {
  return api.post<{ status: string; tagsMarkedAsSeen: number }>(`/images/${id}/mark-seen`)
}

export function deleteImage(id: number) {
  return api.delete<{ status: string }>(`/images/${id}`)
}
