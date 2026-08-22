import { api } from './client'
import type { Version, SMTPStatus, ProxyAuthStatus } from './types/system'

export function getVersion() {
  return api.get<Version>('/version')
}

export function getSMTPStatus() {
  return api.get<SMTPStatus>('/smtp-status')
}

export function getProxyAuthStatus() {
  return api.get<ProxyAuthStatus>('/proxy-auth-status')
}
