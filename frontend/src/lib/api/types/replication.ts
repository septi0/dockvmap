export type ReplicationRole = 'standalone' | 'primary'

export interface SeenFollower {
  name: string
  address: string
  version: string
  appliedVersion: number
  status: string
  lastSeen: string
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
