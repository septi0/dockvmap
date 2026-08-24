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
  createdAt: string
  updatedAt: string
}

export interface ListImagesResponse {
  images: Image[]
  total: number
}

export interface ImageListParams {
  offset: number
  limit?: number
  search?: string
  updateAvailable?: boolean
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
  tags: Tag[]
}

export interface InspectRepositoryParams {
  registry: string
  repository: string
}

export interface InspectRepositoryResult {
  registry: string
  repository: string
  tagGroups: TagGroup[]
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
