import { useCallback, useEffect, useRef, useState } from 'react'

import {
  fetchAnalysis,
  fetchChanges,
  fetchFileDiff,
  fetchRepository,
  requestRefresh,
} from '../api'
import type { Analysis, ChangedFile, FileDiff, RepositoryInfo } from '../types'

const EMPTY_ANALYSIS: Analysis = {
  snapshot_id: '',
  available: false,
  stale: false,
  snapshot_changed: false,
  dropped: 0,
  steps: [],
}

export interface ViewerState {
  repository: RepositoryInfo | null
  files: ChangedFile[]
  analysis: Analysis
  diff: FileDiff | null
  diffLoading: boolean
  loading: boolean
  error: string | null
  selectedFile: string | null
  selectedChangeId: string | null
  refresh: () => void
  selectFile: (path: string) => void
  selectChange: (changeId: string | null) => void
}

/**
 * Owns everything the viewer renders.
 *
 * The file list and the per-file diff are loaded separately so opening a
 * repository does not pay for every file's contents, and each diff request
 * carries an AbortController: switching files fast must not let a slow earlier
 * response overwrite the newer one.
 */
export function useViewerState(): ViewerState {
  const [repository, setRepository] = useState<RepositoryInfo | null>(null)
  const [files, setFiles] = useState<ChangedFile[]>([])
  const [analysis, setAnalysis] = useState<Analysis>(EMPTY_ANALYSIS)
  const [diff, setDiff] = useState<FileDiff | null>(null)
  const [diffLoading, setDiffLoading] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selectedFile, setSelectedFile] = useState<string | null>(null)
  const [selectedChangeId, setSelectedChangeId] = useState<string | null>(null)

  const diffRequest = useRef<AbortController | null>(null)
  const pendingChange = useRef<string | null>(null)

  // recollect asks the server for a new snapshot first; the initial load
  // reuses the one the CLI already built, so opening the viewer costs one
  // collection rather than two.
  const load = useCallback((recollect: boolean) => {
    setLoading(true)
    const controller = new AbortController()
    const start = recollect
      ? requestRefresh(controller.signal)
      : Promise.resolve(null)
    start
      .then(() =>
        Promise.all([
          fetchRepository(controller.signal),
          fetchChanges(controller.signal),
          fetchAnalysis(controller.signal),
        ]),
      )
      .then(([repositoryInfo, changes, analysisView]) => {
        setRepository(repositoryInfo)
        setFiles(changes.files)
        setAnalysis(analysisView)
        setError(null)

        // Keep the current file when it survived; otherwise open where the
        // learning guide starts so both panes agree on first load.
        const firstStep = analysisView.steps[0]
        setSelectedFile((current) => {
          if (current && changes.files.some((file) => file.path === current)) return current
          return firstStep?.file ?? changes.files[0]?.path ?? null
        })
        setSelectedChangeId((current) => {
          const ids = new Set(changes.files.flatMap((file) => file.change_ids))
          if (current && ids.has(current)) return current
          return firstStep?.change_id ?? null
        })
      })
      .catch((cause: unknown) => {
        if (cause instanceof DOMException && cause.name === 'AbortError') return
        setError(cause instanceof Error ? cause.message : String(cause))
      })
      .finally(() => setLoading(false))
  }, [])

  const refresh = useCallback(() => load(true), [load])

  useEffect(() => load(false), [load])

  // Load the selected file's code, cancelling whatever was in flight.
  useEffect(() => {
    diffRequest.current?.abort()
    if (!selectedFile) {
      setDiff(null)
      return
    }
    const controller = new AbortController()
    diffRequest.current = controller
    setDiffLoading(true)
    fetchFileDiff(selectedFile, controller.signal)
      .then((loaded) => {
        setDiff(loaded)
        setError(null)
        // A unit chosen before the diff arrived is applied once it does.
        const wanted = pendingChange.current
        pendingChange.current = null
        setSelectedChangeId((current) => {
          if (wanted && loaded.changes.some((change) => change.id === wanted)) return wanted
          if (current && loaded.changes.some((change) => change.id === current)) return current
          return loaded.changes[0]?.id ?? null
        })
      })
      .catch((cause: unknown) => {
        if (cause instanceof DOMException && cause.name === 'AbortError') return
        setError(cause instanceof Error ? cause.message : String(cause))
      })
      .finally(() => {
        if (!controller.signal.aborted) setDiffLoading(false)
      })
    return () => controller.abort()
  }, [selectedFile])

  const selectFile = useCallback((path: string) => {
    setSelectedFile(path)
  }, [])

  const selectChange = useCallback((changeId: string | null) => {
    pendingChange.current = changeId
    setSelectedChangeId(changeId)
  }, [])

  return {
    repository,
    files,
    analysis,
    diff,
    diffLoading,
    loading,
    error,
    selectedFile,
    selectedChangeId,
    refresh,
    selectFile,
    selectChange,
  }
}
