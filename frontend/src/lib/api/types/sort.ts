export type SortDirection = 'asc' | 'desc'

export interface SortParams<F extends string = string> {
  sort?: F
  order?: SortDirection
}
