import type { Image } from '../api/types/images'

const MIN_SUGGESTED_IMAGES = 2
const FLOATING_TAGS = new Set(['latest', 'stable', 'edge', 'main', 'master', 'nightly'])

export interface ImageGroupSuggestion {
  key: string
  name: string
  tag: string
  images: Image[]
}

function namespaceOf(repository: string): string | null {
  const slash = repository.lastIndexOf('/')

  return slash === -1 ? null : repository.slice(0, slash)
}

export function suggestImageGroups(images: Image[]): ImageGroupSuggestion[] {
  const buckets = new Map<string, ImageGroupSuggestion>()

  for (const image of images) {
    const namespace = namespaceOf(image.repository)

    if (image.group || !namespace || FLOATING_TAGS.has(image.tag)) continue

    const key = `${image.registry}/${namespace}@${image.tag}`
    const bucket = buckets.get(key)

    if (bucket) bucket.images.push(image)
    else buckets.set(key, { key, name: namespace, tag: image.tag, images: [image] })
  }

  return [...buckets.values()].filter((bucket) => bucket.images.length >= MIN_SUGGESTED_IMAGES)
}
