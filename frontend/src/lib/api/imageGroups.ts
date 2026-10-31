import { api } from './client'
import { buildQuery } from '../utils/query'
import type { ImageTagsResult } from './types/images'
import type {
  ImageGroup,
  ImageGroupListParams,
  ListImageGroupsResponse,
  SaveImageGroupParams,
} from './types/imageGroups'

export const IMAGE_GROUPS_MAX_LIMIT = 200

export function listImageGroups(params: ImageGroupListParams = {}) {
  return api.get<ListImageGroupsResponse>(`/image-groups${buildQuery(params)}`)
}

export function getImageGroup(id: number) {
  return api.get<ImageGroup>(`/image-groups/${id}`)
}

export function createImageGroup(params: SaveImageGroupParams) {
  return api.post<{ status: string; id: number }>('/image-groups', params)
}

export function updateImageGroup(id: number, params: SaveImageGroupParams) {
  return api.put<{ status: string }>(`/image-groups/${id}`, params)
}

export function deleteImageGroup(id: number) {
  return api.delete<{ status: string }>(`/image-groups/${id}`)
}

export function getCommonImageTags(imageIds: number[]) {
  return api.get<ImageTagsResult>(`/image-groups/common-tags?imageIds=${imageIds.join(',')}`)
}
