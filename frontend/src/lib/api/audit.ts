import { api } from './client'
import { buildQuery } from '../utils/query'
import type { AuditLogListParams, AuditLogList } from './types/audit'

export const AUDIT_LOG_PAGE_SIZE = 25

export function listAuditLogs(params: AuditLogListParams) {
  return api.get<AuditLogList>(`/audit-logs${buildQuery(params)}`)
}
