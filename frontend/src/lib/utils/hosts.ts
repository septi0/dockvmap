import { capitalize, formatDuration, formatRelativeTime } from './format'
import type { ContainerPort, HostContainer } from '../api/types/hosts'

const AGENT_IMAGE = 'ghcr.io/septi0/dockvmap-agent'
const PUBLISHED_VERSION = /^v?(\d+\.\d+\.\d+(?:-rc\.\d+)?)$/
const ANY_ADDRESS = ['', '0.0.0.0', '::']
const SHELL_SAFE = /^[\w./-]+$/

export type StateTone = 'running' | 'stopped' | 'warning' | 'failed'

export interface ContainerStateView {
  label: string
  detail: string
  tone: StateTone
}

export function containerState(container: HostContainer, now = Date.now()): ContainerStateView {
  const since = container.finishedAt ? formatRelativeTime(container.finishedAt, '', now) : ''

  switch (container.state) {
    case 'running':
      return runningState(container, now)
    case 'exited':
      return {
        label: `Exited (${container.exitCode})`,
        detail: since,
        tone: container.crashed ? 'failed' : 'stopped',
      }
    case 'restarting':
      return { label: 'Restarting', detail: '', tone: 'warning' }
    case 'dead':
      return { label: 'Dead', detail: since, tone: 'failed' }
    case 'paused':
      return { label: 'Paused', detail: '', tone: 'stopped' }
    case 'removing':
      return { label: 'Removing', detail: '', tone: 'stopped' }
    case 'created':
      return { label: 'Created', detail: '', tone: 'stopped' }
    default:
      return { label: capitalize(container.state), detail: since, tone: 'stopped' }
  }
}

function runningState(container: HostContainer, now: number): ContainerStateView {
  const detail = container.startedAt ? upFor(container.startedAt, now) : ''

  switch (container.health) {
    case 'healthy':
      return { label: 'Healthy', detail, tone: 'running' }
    case 'unhealthy':
      return { label: 'Unhealthy', detail, tone: 'warning' }
    case 'starting':
      return { label: 'Starting', detail, tone: 'warning' }
    default:
      return { label: 'Running', detail, tone: 'running' }
  }
}

function upFor(startedAt: string, now: number): string {
  const seconds = (now - new Date(startedAt).getTime()) / 1000

  return seconds < 60 ? '<1m' : formatDuration(seconds)
}

export function formatPorts(ports: ContainerPort[]): string[] {
  const seen = new Set<string>()

  for (const port of ports) {
    const protocol = port.type === 'tcp' ? '' : `/${port.type}`

    seen.add(`${portAddress(port.ip)}${port.publicPort}→${port.privatePort}${protocol}`)
  }

  return [...seen]
}

function portAddress(ip = ''): string {
  if (ANY_ADDRESS.includes(ip)) return ''

  return ip.includes(':') ? `[${ip}]:` : `${ip}:`
}

export interface RedeployEntry {
  hostId: number
  hostName: string
  container: HostContainer
}

export interface RedeployGroup {
  key: string
  hostId: number
  hostName: string
  name: string
  detail: string
  command: string | null
  commandTitle: string
}

export function redeployCommand(container: HostContainer): string | null {
  return composeCommand([container])
}

function composeCommand(containers: HostContainer[]): string | null {
  const dir = containers[0]?.composeWorkingDir
  const services = containers.map((c) => c.composeService)

  if (!dir || !containers.every((c) => c.tracking?.needsUpdate && c.composeService)) return null

  const names = services.map((s) => shellQuote(s ?? '')).join(' ')
  const up = `docker compose up -d ${names}`

  return needsPull(containers) ? `cd ${shellQuote(dir)} && docker compose pull ${names} && ${up}` : `cd ${shellQuote(dir)} && ${up}`
}

function needsPull(containers: HostContainer[]): boolean {
  return containers.some((c) => c.tracking?.status === 'outdated')
}

export function redeployGroups(entries: RedeployEntry[]): RedeployGroup[] {
  const groups = new Map<string, { hostId: number; hostName: string; project: string | null; containers: HostContainer[] }>()

  for (const { hostId, hostName, container } of entries) {
    if (!container.tracking?.needsUpdate) continue

    const project = container.composeProject ?? null
    const key = project ? `${hostId}:${project}:${container.composeWorkingDir ?? ''}` : `${hostId}:#${container.id}`
    const group = groups.get(key) ?? { hostId, hostName, project, containers: [] }

    group.containers.push(container)
    groups.set(key, group)
  }

  const result = [...groups.entries()].map(([key, { hostId, hostName, project, containers }]) => {
    const command = project ? composeCommand(containers) : null
    const members = containers.map((c) => c.name)
    const action = needsPull(containers) ? 'Pulls and recreates' : 'Recreates'

    return {
      key,
      hostId,
      hostName,
      name: project ?? containers[0].name,
      detail: command
        ? members.join(', ')
        : project
          ? `Update the agent on ${hostName} to get the command`
          : 'Started without Compose, so redeploy it by hand',
      command,
      commandTitle: command ? `${command}\n${action} ${joinNames(members)}. The rest of the stack keeps running.` : '',
    }
  })

  return result.sort(
    (a, b) =>
      Number(!a.command) - Number(!b.command) ||
      a.hostName.localeCompare(b.hostName) ||
      a.name.localeCompare(b.name),
  )
}

function joinNames(names: string[]): string {
  return names.length < 2 ? names.join('') : `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}`
}

function shellQuote(value: string): string {
  return SHELL_SAFE.test(value) ? value : `'${value.replaceAll("'", "'\\''")}'`
}

export function agentCompose(address: string, appVersion: string): string {
  const tag = PUBLISHED_VERSION.exec(appVersion)?.[1] ?? 'latest'
  const port = /:(\d+)$/.exec(address)?.[1] ?? '9002'

  return [
    'services:',
    '  dockvmap-agent:',
    `    image: ${AGENT_IMAGE}:${tag}`,
    '    restart: unless-stopped',
    '    ports:',
    `      - "${port}:9002"`,
    '    volumes:',
    '      - /var/run/docker.sock:/var/run/docker.sock:ro',
    '      - dockvmap-agent:/data',
    '',
    'volumes:',
    '  dockvmap-agent:',
  ].join('\n')
}
