export interface AuditLog {
  id: number
  type: string
  data?: Record<string, unknown>
  ip?: string
  userAgent?: string
  userId?: number
  username?: string
  createdAt: string
}

export interface AuditLogList {
  auditLogs: AuditLog[]
  total: number
}

export interface AuditLogListParams {
  offset: number
  limit?: number
  type?: string
  since?: string
  until?: string
}

export const AUDIT_TYPES = [
  'REGISTRY_CREATED',
  'REGISTRY_UPDATED',
  'REGISTRY_DELETED',
  'IMAGE_CREATED',
  'IMAGE_TAG_CHANGED',
  'IMAGE_RENAMED',
  'IMAGE_PIN_CHANGED',
  'IMAGE_DELETED',

  'IMAGE_GROUP_CREATED',
  'IMAGE_GROUP_UPDATED',
  'IMAGE_GROUP_DELETED',

  'USER_BOOTSTRAPPED',
  'USER_CREATED',
  'USER_PASSWORD_CHANGED',
  'USER_PASSWORD_CHANGE_FAILED',
  'USER_PASSWORD_RESET',
  'USER_EMAIL_CHANGED',
  'USER_DELETED',

  'USER_LOGGED_IN',
  'USER_LOGIN_FAILED',
  'USER_LOGGED_OUT',
  'USER_SESSION_REVOKED',

  'PROXY_TOKEN_CREATED',
  'PROXY_TOKEN_DELETED',

  'HOST_CREATED',
  'HOST_UPDATED',
  'HOST_AGENT_TRUSTED',
  'HOST_DELETED',

  'IMAGE_PUSHED',
  'MANIFEST_DELETED',
] as const
