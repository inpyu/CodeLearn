import { useMemo } from 'react'

import DiffViewer from './components/DiffViewer'
import FileList from './components/FileList'
import LearningPanel from './components/LearningPanel'
import { useViewerState } from './hooks/useViewerState'

export default function App() {
  const state = useViewerState()
  const {
    repository,
    files,
    analysis,
    diff,
    diffLoading,
    loading,
    error,
    selectedFile,
    selectedChangeId,
  } = state

  const file = useMemo(
    () => files.find((candidate) => candidate.path === selectedFile) ?? null,
    [files, selectedFile],
  )

  const explainedFiles = useMemo(
    () => new Set(analysis.steps.map((step) => step.file)),
    [analysis.steps],
  )
  const explainedChanges = useMemo(
    () => new Set(analysis.steps.map((step) => step.change_id)),
    [analysis.steps],
  )

  // The files that have explanations, in file list order, so Prev/Next can
  // carry the reader across file boundaries without losing the tour.
  const explainedOrder = useMemo(
    () => files.map((candidate) => candidate.path).filter((path) => explainedFiles.has(path)),
    [files, explainedFiles],
  )

  const selectFile = (path: string) => {
    state.selectFile(path)
    const firstStep = analysis.steps.find((step) => step.file === path)
    state.selectChange(firstStep?.change_id ?? null)
  }

  const leaveFile = (direction: -1 | 1) => {
    if (!selectedFile) return
    const position = explainedOrder.indexOf(selectedFile)
    const next = explainedOrder[position + direction]
    if (position < 0 || next === undefined) return
    const inNext = analysis.steps.filter((step) => step.file === next)
    const landing = direction === 1 ? inNext[0] : inNext[inNext.length - 1]
    state.selectFile(next)
    state.selectChange(landing?.change_id ?? null)
  }

  return (
    <div className="flex h-full flex-col">
      <header className="flex shrink-0 items-center gap-3 border-b border-edge bg-panel px-4 py-2">
        <span className="text-[15px] font-semibold text-ink">CodeLearn</span>
        {repository && (
          <span className="font-mono text-[11px] text-muted">
            {repository.name} · {repository.branch} · vs {repository.base}
          </span>
        )}
        <div className="ml-auto flex items-center gap-3">
          {error && <span className="text-[12px] text-rose-300">{error}</span>}
          <button
            type="button"
            onClick={state.refresh}
            disabled={loading}
            className="rounded border border-edge px-3 py-1 text-[12px] text-ink enabled:hover:border-accent disabled:opacity-40"
          >
            {loading ? 'Refreshing...' : 'Refresh'}
          </button>
        </div>
      </header>

      {files.length === 0 && !loading ? (
        <main className="flex flex-1 flex-col items-center justify-center gap-2 text-center">
          <p className="text-[15px] text-ink">No uncommitted changes</p>
          <p className="max-w-sm text-[13px] text-muted">
            Edit some files in this repository and press Refresh to compare them against{' '}
            {repository?.base ?? 'HEAD'}.
          </p>
        </main>
      ) : (
        <main className="flex min-h-0 flex-1">
          <FileList
            files={files}
            selectedPath={selectedFile}
            explainedFiles={explainedFiles}
            onSelect={selectFile}
          />
          <DiffViewer
            file={file}
            diff={diff}
            loading={diffLoading}
            selectedChangeId={selectedChangeId}
            explainedChanges={explainedChanges}
            onSelectChange={state.selectChange}
          />
          <LearningPanel
            analysis={analysis}
            file={selectedFile}
            currentChangeId={selectedChangeId}
            onSelectStep={(step) => state.selectChange(step.change_id)}
            onLeaveFile={leaveFile}
          />
        </main>
      )}
    </div>
  )
}
