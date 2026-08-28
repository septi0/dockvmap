export function formatDate(value: string | undefined, fallback = '-'): string {
  return value ? new Date(value).toLocaleString() : fallback
}

export function formatRelativeTime(value: string | undefined, fallback = '-'): string {
  if (!value) return fallback

  const diffMs = new Date(value).getTime() - Date.now()
  const abs = Math.abs(diffMs)

  if (abs < 60_000) return diffMs >= 0 ? 'any moment' : 'just now'

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
