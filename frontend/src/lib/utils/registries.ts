export const INTERNAL_REGISTRY_LABEL = 'Internal registry'

type ImageSourceFields = { registry: string; repository: string; registryInternal: boolean }

export function imageSourceParts(image: ImageSourceFields): { registry: string; separator: string } {
  return image.registryInternal
    ? { registry: INTERNAL_REGISTRY_LABEL, separator: ' · ' }
    : { registry: image.registry, separator: '/' }
}

export function imageSource(image: ImageSourceFields): string {
  const { registry, separator } = imageSourceParts(image)

  return `${registry}${separator}${image.repository}`
}
