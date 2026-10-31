import type { PullInfo } from '../api/types/metrics'

export function imageReference(info: PullInfo, name: string): string {
  return `${info.host}:${info.port}/${name}:${info.virtualTag}`
}

export function pullCommand(info: PullInfo, name: string): string {
  return `docker pull ${imageReference(info, name)}`
}
