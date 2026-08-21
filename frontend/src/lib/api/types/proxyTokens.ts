export interface ProxyToken {
  id: number
  label: string
  createdAt: string
}

export interface CreateProxyTokenParams {
  label: string
}

export interface CreateProxyTokenResult {
  id: number
  label: string
  token: string
}
