import Bowser from 'bowser'

export type DeviceType = 'desktop' | 'mobile' | 'tablet' | 'unknown'

export interface ParsedUA {
  browser?: string
  os?: string
  device: DeviceType
  label: string
  raw: string
}

const DEVICE_TYPES = new Set<DeviceType>(['desktop', 'mobile', 'tablet'])

export function parseUserAgent(ua?: string | null): ParsedUA {
  const raw = ua ?? ''

  if (!raw) {
    return { device: 'unknown', label: 'Unknown device', raw }
  }

  const { browser, os, platform } = Bowser.parse(raw)

  const browserLabel = browser.name
    ? [browser.name, majorVersion(browser.version)].filter(Boolean).join(' ')
    : undefined
  const osLabel = os.name || undefined
  const device = DEVICE_TYPES.has(platform.type as DeviceType)
    ? (platform.type as DeviceType)
    : 'unknown'

  const parts = [browserLabel, osLabel].filter(Boolean)

  return {
    browser: browserLabel,
    os: osLabel,
    device,
    label: parts.length > 0 ? parts.join(' · ') : raw,
    raw,
  }
}

function majorVersion(version?: string): string {
  return version ? version.split('.')[0] : ''
}
