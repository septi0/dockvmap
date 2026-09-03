import { api } from './client'
import type {
  Version,
  SMTPStatus,
  ProxyAuthStatus,
  SystemStatus,
  SystemTask,
} from './types/system'

export function getVersion() {
  return api.get<Version>('/version')
}

export function getSMTPStatus() {
  return api.get<SMTPStatus>('/smtp-status')
}

export function getProxyAuthStatus() {
  return api.get<ProxyAuthStatus>('/proxy-auth-status')
}

export function getSystemStatus() {
  return api.get<SystemStatus>('/system/status')
}

export function getSystemTasks() {
  return api.get<{ tasks: SystemTask[] }>('/system/tasks')
}

export function runSystemTask(name: string) {
  return api.post<{ status: string }>(`/system/tasks/${encodeURIComponent(name)}/run`)
}
