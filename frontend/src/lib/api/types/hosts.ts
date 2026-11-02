export interface HostInfo {
  hostname?: string
  dockerVersion?: string
  os?: string
  architecture?: string
  cpus?: number
  memoryBytes?: number
}

export interface Host {
  id: number
  name: string
  createdAt: string
  lastSeenAt?: string
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

export type ContainerVersionStatus = 'upToDate' | 'outdated' | 'pulledNotRecreated' | 'unknown' | 'unknownImage'

export interface ContainerPort {
  ip?: string
  privatePort: number
  publicPort: number
  type: string
}

export interface ContainerTracking {
  imageId?: number
  imageName: string
  status: ContainerVersionStatus
  pinnedTag?: string
  needsUpdate: boolean
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
  createdAt?: string
  startedAt?: string
  finishedAt?: string
  ports: ContainerPort[]
  tracking?: ContainerTracking
}

export interface HostDetails {
  host: Host
  containers: HostContainer[] | null
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

export interface CreateHostParams {
  name: string
}

export interface HostToken {
  id: number
  name: string
  token: string
}

export interface HostConnection extends HostToken {
  lastSeenAt?: string
}
