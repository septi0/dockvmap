export interface ProxyToken {
  id: number
  label: string
  createdAt: string
  lastUsedAt?: string
  canPush: boolean
}

export interface CreateProxyTokenParams {
  label: string
  canPush: boolean
}

export interface CreateProxyTokenResult {
  id: number
  label: string
  canPush: boolean
  token: string
}
