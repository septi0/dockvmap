import { api } from './client'
import type { ReplicationStatus, ReplicationToken, CreateReplicationTokenResult } from './types/replication'

export function getReplicationStatus() {
  return api.get<ReplicationStatus>('/replication/status')
}

export function listReplicationTokens() {
  return api.get<ReplicationToken[]>('/replication/tokens')
}

export function createReplicationToken(label: string) {
  return api.post<CreateReplicationTokenResult>('/replication/tokens', { label })
}

export function deleteReplicationToken(id: number) {
  return api.delete<{ status: string }>(`/replication/tokens/${id}`)
}

export function removeReplicationFollower(id: number) {
  return api.delete<{ status: string }>(`/replication/followers/${id}`)
}

export function renameReplicationFollower(id: number, name: string) {
  return api.put<{ status: string }>(`/replication/followers/${id}`, { name })
}
