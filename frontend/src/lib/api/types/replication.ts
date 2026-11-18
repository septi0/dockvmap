export type ReplicationRole = 'standalone' | 'primary'

export interface FollowerHealth {
  ready: boolean
  notReady?: string
  pendingRepositories: number
  failedCopies: number
  extraTags?: number
  lastError?: string
  copyError?: string
}

export type FollowerConnection = 'connected' | 'waiting' | 'missing'

export interface SeenFollower {
  id: number
  name: string
  address: string
  token: string
  version: string
  appliedVersion: number
  status: string
  connection: FollowerConnection
  firstSeen: string
  lastSeen: string
  health?: FollowerHealth
}

export interface PrimaryStatus {
  cluster: string
  stateVersion: number
  fingerprint: string
  listen: string
  allowedIps: string[]
  followers: SeenFollower[]
}

export interface ReplicationStatus {
  role: ReplicationRole
  primary?: PrimaryStatus
}

export interface ReplicationToken {
  id: number
  label: string
  createdAt: string
  lastUsedAt?: string
}

export interface CreateReplicationTokenResult {
  id: number
  label: string
  token: string
}
