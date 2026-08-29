export interface Image {
  id: number
  name: string
  registryId: number
  registry: string
  repository: string
  tag: string
  lastChecked?: string
  lastCheckError?: string
  updateAvailable: boolean
  updateAvailableTag?: string
  refreshStatus: 'idle' | 'running'
  createdAt: string
  updatedAt: string
}

export interface ListImagesResponse {
  images: Image[]
  total: number
}

export const IMAGE_STATUS_FILTERS = ['updateAvailable', 'failedCheck'] as const

export type ImageStatusFilter = (typeof IMAGE_STATUS_FILTERS)[number]

export interface ImageListParams {
  offset: number
  limit?: number
  search?: string
  status?: ImageStatusFilter
}

export interface Tag {
  tag: string
  firstSeen?: string
  lastSeen?: string
  new?: boolean
  prerelease?: boolean
}

export interface TagGroup {
  familyType: string
  familyId: number
  hasOrder: boolean
  tags: Tag[]
}

export interface InspectRepositoryParams {
  registry: string
  repository: string
}

export type DiscoveryStatus = 'running' | 'completed' | 'failed'

export interface DiscoveryResult {
  id: number
  status: DiscoveryStatus
  tagGroups?: TagGroup[]
  tagCount?: number
  ignoredCount?: number
  tagsSeen?: number
  error?: string
}

export interface CreateImageParams {
  name: string
  registryId: number
  repository: string
  tag: string
}

export interface ImageTagsResult {
  tagGroups: TagGroup[]
}

export type TagHistorySource = 'created' | 'manual' | 'restore'

export interface TagHistoryEntry {
  id: number
  tag: string
  previousTag?: string
  source: TagHistorySource
  appliedAt: string
}

export interface TagHistoryResult {
  history: TagHistoryEntry[]
}
