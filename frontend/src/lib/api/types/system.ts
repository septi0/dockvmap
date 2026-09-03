export interface Version {
  version: string
}

export interface SMTPStatus {
  enabled: boolean
}

export interface ProxyAuthStatus {
  enabled: boolean
}

export interface SystemDatabaseStatus {
  reachable: boolean
  schemaVersion: number
  sizeBytes: number
  path: string
}

export interface SystemStatus {
  version: string
  startedAt: string
  dataPath: string
  database: SystemDatabaseStatus
  configWarnings: string[]
  virtualTag: string
  trustedProxies: string[]
  proxyAuthEnabled: boolean
}

export interface SystemTask {
  name: string
  description: string
  intervalSeconds: number
  enabled: boolean
  disabledReason?: string
  triggerable: boolean
  running: boolean
  lastRun?: string
  nextDue?: string
  lastError?: string
  lastCount?: number
}
