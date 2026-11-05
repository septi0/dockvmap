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

export function redeployCommand(container: HostContainer): string | null {
  const { tracking, composeWorkingDir: dir, composeService: service } = container

  if (!tracking?.needsUpdate || !dir || !service) return null

  const cd = `cd ${shellQuote(dir)}`
  const name = shellQuote(service)

  switch (tracking.status) {
    case 'outdated':
      return `${cd} && docker compose pull ${name} && docker compose up -d ${name}`
    case 'pulledNotRecreated':
      return `${cd} && docker compose up -d ${name}`
    default:
      return null
  }
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
