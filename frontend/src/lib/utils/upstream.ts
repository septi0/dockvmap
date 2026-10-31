const DOCKER_HUB_HOSTS = ['docker.io', 'index.docker.io', 'registry-1.docker.io']

export function upstreamUrl(registry: string, repository: string): string | null {
  if (DOCKER_HUB_HOSTS.includes(registry)) {
    const name = repository.replace(/^library\//, '')

    return name.includes('/')
      ? `https://hub.docker.com/r/${name}/tags`
      : `https://hub.docker.com/_/${name}`
  }

  if (registry === 'ghcr.io') return `https://ghcr.io/${repository}`
  if (registry === 'quay.io') return `https://quay.io/repository/${repository}`

  return null
}
