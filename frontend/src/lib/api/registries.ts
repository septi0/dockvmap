import { api } from './client'
import type { Registry, CreateRegistryParams, UpdateRegistryParams } from './types/registries'

export function listRegistries() {
  return api.get<Registry[]>('/registries')
}

export function createRegistry(params: CreateRegistryParams) {
  return api.post<Registry>('/registries', params)
}

export function updateRegistry(id: number, params: UpdateRegistryParams) {
  const body: Record<string, unknown> = {
    registry: params.registry,
    options: params.options,
  }

  if (params.username !== undefined) {
    body.username = params.username
  }

  if (params.credential !== undefined) {
    body.credential = params.credential
  }

  return api.put<Registry>(`/registries/${id}`, body)
}

export function deleteRegistry(id: number) {
  return api.delete<{ status: string }>(`/registries/${id}`)
}
