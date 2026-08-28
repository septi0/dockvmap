export interface TagRefreshStatus {
  enabled: boolean
  interval: string
  lastRun: string | null
  nextDue: string | null
}
