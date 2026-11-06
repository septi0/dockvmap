export interface HostInfo {
  hostname?: string
  dockerVersion?: string
  os?: string
  architecture?: string
  cpus?: number
  memoryBytes?: number
}

export type HostPollErrorKind = 'failed' | 'identityChanged' | 'pairedElsewhere'

export interface Host {
  id: number
  name: string
  address: string
  agentFingerprint?: string
  createdAt: string
  lastSeenAt?: string
  pollError?: string
  pollErrorKind?: HostPollErrorKind
  online: boolean
  current: boolean
  agentVersion?: string
  agentUpdateNeeded: boolean
  dockerError?: string
  info: HostInfo
  runningContainers: number
  stoppedContainers: number
  stacks: number
  unhealthyContainers: number
  crashedContainers: number
  proxyContainers: number
  rollout: Rollout
  lastStartedAt?: string
}

export interface Rollout {
  upToDate: number
  total: number
}

export type ContainerVersionStatus = 'upToDate' | 'outdated' | 'pulledNotRecreated' | 'unknown' | 'unknownImage' | 'internalRegistry'

export interface ContainerPort {
  ip?: string
  privatePort: number
  publicPort: number
  type: string
}

export interface ContainerTracking {
  imageId?: number
  imageName: string
  tag?: string
  status: ContainerVersionStatus
  pinnedTag?: string
  needsUpdate: boolean
  internalRegistry: boolean
}

export interface HostContainer {
  id: string
  name: string
  image: string
  state: string
  health?: 'healthy' | 'unhealthy' | 'starting'
  exitCode: number
  crashed: boolean
  recentlyCrashed: boolean
  composeProject?: string
  composeService?: string
  composeWorkingDir?: string
  createdAt?: string
  startedAt?: string
  finishedAt?: string
  ports: ContainerPort[]
  tracking?: ContainerTracking
}

export interface HostDetails {
  host: Host
  containers: HostContainer[]
}

export interface ImageRollout extends Rollout {
  imageId: number
  hostsOffline: number
  hostsDockerError: number
}

export interface ImageContainers {
  hosts: HostDetails[]
  rollout: Rollout
  configuredHosts: number
}

export interface HostParams {
  name: string
  address: string
}

export interface HostConnection extends HostParams {
  id: number
  lastSeenAt?: string
}
