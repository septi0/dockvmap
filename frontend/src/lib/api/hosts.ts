import { api } from './client'
import type { CreateHostParams, Host, HostDetails, HostToken, ImageContainers, ImageRollout } from './types/hosts'

export function listHosts() {
  return api.get<Host[]>('/hosts')
}

export function getHost(id: number) {
  return api.get<HostDetails>(`/hosts/${id}`)
}

export function createHost(params: CreateHostParams) {
  return api.post<HostToken>('/hosts', params)
}

export function regenerateHostToken(id: number) {
  return api.post<{ token: string }>(`/hosts/${id}/token`)
}

export function deleteHost(id: number) {
  return api.delete<{ status: string }>(`/hosts/${id}`)
}

export function getImageHosts(imageId: number) {
  return api.get<ImageContainers>(`/images/${imageId}/hosts`)
}

export function listImageRollouts() {
  return api.get<ImageRollout[]>('/images/rollouts')
}
