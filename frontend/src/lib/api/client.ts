export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

const NETWORK_ERROR_STATUS = 0

let onUnauthorized: (() => void) | null = null

export function setUnauthorizedHandler(handler: () => void) {
  onUnauthorized = handler
}

interface RequestOptions {
  expectUnauthorized?: boolean
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  options?: RequestOptions,
): Promise<T> {
  let res: Response

  try {
    res = await fetch(`/api${path}`, {
      method,
      credentials: 'include',
      headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    })
  } catch {
    throw new ApiError(
      NETWORK_ERROR_STATUS,
      'Could not reach the server. Check your connection and try again.',
    )
  }

  const contentType = res.headers.get('content-type') ?? ''
  let data: any

  if (contentType.includes('application/json')) {
    try {
      data = await res.json()
    } catch {
      data = undefined
    }
  }

  if (!res.ok) {
    if (res.status === 401 && !options?.expectUnauthorized) {
      onUnauthorized?.()
    }

    // res.statusText is empty over HTTP/2, so keep a status-code fallback
    const message =
      (data && typeof data.error === 'string' && data.error) ||
      res.statusText ||
      `Request failed (${res.status})`
    throw new ApiError(res.status, message)
  }

  return data as T
}

export const api = {
  get: <T>(path: string, options?: RequestOptions) => request<T>('GET', path, undefined, options),
  post: <T>(path: string, body?: unknown, options?: RequestOptions) =>
    request<T>('POST', path, body, options),
  put: <T>(path: string, body?: unknown, options?: RequestOptions) =>
    request<T>('PUT', path, body, options),
  delete: <T>(path: string, options?: RequestOptions) => request<T>('DELETE', path, undefined, options),
}

export function errorMessage(err: unknown, fallback: string): string {
  return err instanceof ApiError ? err.message : fallback
}
