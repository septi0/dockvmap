export interface DashboardCardProps<T> {
  data: T | null
  error: string | null
  loading: boolean
  busy: boolean
  onRetry: () => void
}
