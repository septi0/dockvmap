import type { StateTone } from './hosts'
import type { SeenFollower } from '../api/types/replication'
import { formatCount, formatRelativeTime } from './format'

type FollowerState = { label: string; tone: StateTone; fix?: string }

const RESYNC = 'Needs dockvmap -resync'

// fix is what the primary's followers table tells the operator to do on that follower
const SYNC_STATES: Record<string, FollowerState> = {
  diverged: { label: "Stopped: history doesn't match the primary's", tone: 'failed', fix: RESYNC },
  'other-cluster': { label: 'Stopped: the primary belongs to another cluster', tone: 'failed', fix: RESYNC },
  'existing-data': { label: 'Stopped: this follower has its own data', tone: 'failed', fix: RESYNC },
  'setup-mismatch': { label: "Stopped: setup differs from the primary's", tone: 'failed', fix: 'Match its config, then restart it' },
  'api-version': { label: 'The primary runs a newer DockVMap', tone: 'warning', fix: 'Update it to this version' },
}

// a follower shows the name given on this primary, or its address until it has one
export function followerName(follower: SeenFollower): string {
  return follower.name || follower.address
}

// the worst thing about a follower first: gone, stopped, failing copies, not ready, then how far behind it is
export function followerState(follower: SeenFollower, stateVersion: number): FollowerState {
  if (follower.connection === 'missing') {
    return { label: `Not connected, last seen ${formatRelativeTime(follower.lastSeen)}`, tone: 'failed', fix: 'Check that it runs and can reach this primary' }
  }

  if (follower.connection === 'waiting') {
    return { label: 'Waiting for it to connect', tone: 'stopped' }
  }

  if (follower.status && follower.status !== 'running') {
    return SYNC_STATES[follower.status] ?? { label: follower.status, tone: 'warning' }
  }

  const health = follower.health

  if (health && health.failedCopies > 0) {
    return { label: `${formatCount(health.failedCopies, 'repository', 'repositories')} failing to copy`, tone: 'failed' }
  }

  if (health && !health.ready) {
    return { label: `Not ready: ${health.notReady}`, tone: 'warning' }
  }

  if (health && health.pendingRepositories > 0) {
    return { label: `Copying, ${formatCount(health.pendingRepositories, 'repository', 'repositories')} left`, tone: 'running' }
  }

  const behind = stateVersion - follower.appliedVersion

  return { label: behind <= 0 ? 'In sync' : `${formatCount(behind, 'change')} behind`, tone: 'running' }
}

// the primary's own address: the listen host when it names one, else the address this page was opened on
export function followerConfig(listen: string, token: string): string {
  const separator = listen.lastIndexOf(':')
  const host = listen.slice(0, separator)
  const port = listen.slice(separator + 1)
  const address = host && host !== '0.0.0.0' && host !== '[::]' ? host : window.location.hostname

  return `follower:\n  enabled: true\n  primary: ${address}:${port}\n  token: ${token}\n`
}
