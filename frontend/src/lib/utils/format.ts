export function formatDate(value: string | undefined, fallback = '-'): string {
  return value ? new Date(value).toLocaleString() : fallback
}

export function formatAuditType(type: string): string {
  const label = type.toLowerCase().replaceAll('_', ' ')
  return label.charAt(0).toUpperCase() + label.slice(1)
}
