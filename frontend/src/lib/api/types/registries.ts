export interface RegistryOptions {
  insecure: boolean
  allow_self_signed_certs: boolean
}

export interface Registry {
  id: number
  registry: string
  username?: string
  authenticationConfigured: boolean
  options: RegistryOptions
  internal: boolean
  imageCount: number
  failedChecks: number
}

export interface RegistryRepository {
  name: string
  images: { id: number; name: string }[]
}

export interface RegistryRepositories {
  repositories: RegistryRepository[]
  more: boolean
}

export interface CreateRegistryParams {
  registry: string
  username: string
  credential: string
  options: RegistryOptions
}

export interface UpdateRegistryParams {
  registry: string
  username?: string
  credential?: string
  options: RegistryOptions
}

export interface RegistryTestResult {
  ok: boolean
  error?: string
}
