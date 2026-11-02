import type { PullInfo } from '../api/types/metrics'

export function proxyAddress(info: PullInfo): string {
  return info.port === '443' ? info.host : `${info.host}:${info.port}`
}

export function pullCommand(info: PullInfo, name: string): string {
  return `docker pull ${proxyAddress(info)}/${name}:${info.virtualTag}`
}
