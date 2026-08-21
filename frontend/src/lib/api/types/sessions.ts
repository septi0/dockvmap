export interface Session {
  id: number
  ip?: string
  userAgent?: string
  createdAt: string
  expiresAt: string
  current: boolean
}
