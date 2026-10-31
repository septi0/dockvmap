import type { ImageGroup } from '../api/types/imageGroups'

export interface GroupPullStatus {
  pulled: number
  total: number
  missing: string[]
}

export function groupPullStatus(group: ImageGroup): GroupPullStatus | null {
  if (!group.images.some((image) => image.lastPulledAt)) return null

  const missing = group.images
    .filter((image) => image.lastPulledTag !== group.tag)
    .map((image) => image.name)

  if (missing.length === 0) return null

  return { pulled: group.images.length - missing.length, total: group.images.length, missing }
}
