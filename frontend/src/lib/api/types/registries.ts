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
