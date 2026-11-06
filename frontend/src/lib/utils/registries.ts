import type { Registry } from '../api/types/registries'

const INTERNAL_REGISTRY_LABEL = 'Internal registry'

export function registryOptions(registry: Registry): string {
  const options = []

  if (registry.internal && registry.options.insecure) options.push('Stack network')
  else if (registry.options.insecure) options.push('Plain HTTP')

  if (registry.options.allow_self_signed_certs) options.push('Self-signed certificate')

  return options.join(' · ') || '-'
}

export function registryLabel(registry: { registry: string; internal: boolean }): string {
  return registry.internal ? `${INTERNAL_REGISTRY_LABEL} (${registry.registry})` : registry.registry
}

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
