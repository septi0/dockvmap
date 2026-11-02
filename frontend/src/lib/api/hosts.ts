import { api } from './client'
import type { Host, HostDetails, HostParams, ImageContainers, ImageRollout } from './types/hosts'

export function listHosts() {
  return api.get<Host[]>('/hosts')
}

export function getHost(id: number) {
  return api.get<HostDetails>(`/hosts/${id}`)
}

export function createHost(params: HostParams) {
  return api.post<{ id: number }>('/hosts', params)
}

export function updateHost(id: number, params: HostParams) {
  return api.put<{ status: string }>(`/hosts/${id}`, params)
}

export function trustHostAgent(id: number) {
  return api.post<{ status: string }>(`/hosts/${id}/trust-agent`)
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
