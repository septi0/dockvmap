import { api } from './client'
import type { ProxyToken, CreateProxyTokenParams, CreateProxyTokenResult } from './types/proxyTokens'

export function listProxyTokens() {
  return api.get<ProxyToken[]>('/proxy-tokens')
}

export function createProxyToken(params: CreateProxyTokenParams) {
  return api.post<CreateProxyTokenResult>('/proxy-tokens', params)
}

export function deleteProxyToken(id: number) {
  return api.delete<{ status: string }>(`/proxy-tokens/${id}`)
}
