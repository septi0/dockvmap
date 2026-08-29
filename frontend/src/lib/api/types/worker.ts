export interface TagRefreshStatus {
  enabled: boolean
  interval: string
  running: boolean
  lastRun: string | null
  nextDue: string | null
}
