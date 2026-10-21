/** An inclusive 1-based line range. A count of 0 means the side has no lines. */
export interface LineRange {
  start: number
  count: number
}

export type FileStatus =
  | 'modified'
  | 'added'
  | 'deleted'
  | 'renamed'
  | 'untracked'
  | 'type_changed'

export interface RepositoryInfo {
  name: string
  path: string
  branch: string
  base: string
  base_commit: string
  snapshot_id: string
  changed_files: number
  version: string
}

/** A changed file as listed by /api/changes: metadata and unit ids, no code. */
export interface ChangedFile {
  path: string
  old_path?: string
  status: FileStatus
  binary: boolean
  /** The change exists only in the index; the working tree matches HEAD. */
  index_only: boolean
  additions: number
  deletions: number
  change_ids: string[]
}

/** A change unit as returned with a file's diff. */
export interface ChangeUnit {
  id: string
  /** The function, method or class it covers; null for module level code. */
  symbol: string | null
  kind: string
  /** Display name, e.g. "Collect()" or "module level". */
  label: string
  old_range: LineRange
  new_range: LineRange
}

/** The response of /api/files/diff, loaded only for the selected file. */
export interface FileDiff {
  snapshot_id: string
  path: string
  binary: boolean
  /** Before/After compare HEAD with the index rather than the working tree. */
  index_only: boolean
  too_large: boolean
  before: string
  after: string
  changes: ChangeUnit[]
}

export interface LearningStep {
  change_id: string
  title: string
  what: string
  how: string
  why?: string
  file: string
  symbol: string | null
  kind: string
  label: string
  old_range: LineRange
  new_range: LineRange
}

export interface Analysis {
  snapshot_id: string
  available: boolean
  /** Explanations were dropped because the code they describe changed. */
  stale: boolean
  /** The working tree moved, which on its own invalidates nothing. */
  snapshot_changed: boolean
  dropped: number
  created_at?: string
  generator?: string
  steps: LearningStep[]
}
