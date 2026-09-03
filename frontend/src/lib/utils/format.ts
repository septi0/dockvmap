export function formatDate(value: string | undefined, fallback = '-'): string {
  return value ? new Date(value).toLocaleString() : fallback
}

export function formatNumber(value: number): string {
  return value.toLocaleString()
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`

  const units = ['KB', 'MB', 'GB', 'TB']
  let value = bytes / 1024
  let unit = 0

  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit++
  }

  return `${value < 10 ? value.toFixed(1) : Math.round(value)} ${units[unit]}`
}

export function formatRelativeTime(
  value: string | number | undefined,
  fallback = '-',
  reference = Date.now(),
): string {
  if (value === undefined || value === '') return fallback

  const time = new Date(value).getTime()

  if (Number.isNaN(time)) return fallback

  const diffMs = time - reference
  const abs = Math.abs(diffMs)

  if (abs < 60_000) return diffMs > 0 ? 'any moment' : 'just now'

  const mins = Math.round(abs / 60_000)
  const hours = Math.round(abs / 3_600_000)
  const days = Math.round(abs / 86_400_000)
  const magnitude = mins < 60 ? `${mins}m` : hours < 24 ? `${hours}h` : `${days}d`

  return diffMs >= 0 ? `in ${magnitude}` : `${magnitude} ago`
}

export function formatAuditType(type: string): string {
  const label = type.toLowerCase().replaceAll('_', ' ')
  return label.charAt(0).toUpperCase() + label.slice(1)
}

export function toRfc3339DayStart(date: string): string | undefined {
  return date ? new Date(`${date}T00:00:00`).toISOString() : undefined
}

export function toRfc3339DayEnd(date: string): string | undefined {
  return date ? new Date(`${date}T23:59:59`).toISOString() : undefined
}
