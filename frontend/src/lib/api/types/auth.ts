export type NotifyLevel = "all" | "upgrades" | "none"

export interface CurrentUser {
  id: number
  username: string
  email: string
  notifyLevel: NotifyLevel
}
