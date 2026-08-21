import { api } from './client'
import type { SMTPStatus, ProxyAuthStatus } from './types/system'

export function getSMTPStatus() {
  return api.get<SMTPStatus>('/smtp-status')
}

export function getProxyAuthStatus() {
  return api.get<ProxyAuthStatus>('/proxy-auth-status')
}
