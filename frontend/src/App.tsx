import { useMemo } from 'react'

import DiffViewer from './components/DiffViewer'
import FileList from './components/FileList'
import LearningPanel from './components/LearningPanel'
import Splitter from './components/Splitter'
import { usePaneWidth } from './hooks/usePaneWidth'
import { useViewerState } from './hooks/useViewerState'

export default function App() {
  const state = useViewerState()
  const left = usePaneWidth('codelearn.pane.files', 256, 180, 520)
  const right = usePaneWidth('codelearn.pane.guide', 352, 260, 680)
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

  // The files that have explanations, in file list order, so Prev/Next can
  // carry the reader across file boundaries without losing the tour.
  const explainedOrder = useMemo(
    () => files.map((candidate) => candidate.path).filter((path) => explainedFiles.has(path)),
    [files, explainedFiles],
  )

  // Totals for the header, which would otherwise be a wide empty band.
  const totals = useMemo(
    () =>
      files.reduce(
        (sum, candidate) => ({
          additions: sum.additions + candidate.additions,
          deletions: sum.deletions + candidate.deletions,
          units: sum.units + candidate.change_ids.length,
        }),
        { additions: 0, deletions: 0, units: 0 },
      ),
    [files],
  )

  const units = diff?.path === selectedFile ? diff.changes : []

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
      <header className="flex h-11 shrink-0 items-center gap-3 border-b border-edge bg-panel px-4">
        <span className="text-[14px] font-semibold tracking-tight text-ink">CodeLearn</span>

        {repository && (
          <span className="flex min-w-0 items-center gap-2 font-mono text-[11px] text-muted">
            <span className="truncate text-ink-dim">{repository.name}</span>
            <span className="text-edge">/</span>
            <span className="truncate">{repository.branch}</span>
            <span className="text-edge">/</span>
            <span>vs {repository.base}</span>
          </span>
        )}

        {/* Stats live in the header band, which is otherwise dead width. */}
        {files.length > 0 && (
          <span className="flex items-center gap-3 font-mono text-[11px] text-muted">
            <span className="h-3 w-px bg-edge" />
            <span>
              {files.length} file{files.length === 1 ? '' : 's'}
            </span>
            <span>
              <span className="text-add">+{totals.additions}</span>{' '}
              <span className="text-del">-{totals.deletions}</span>
            </span>
            <span>
              {totals.units} unit{totals.units === 1 ? '' : 's'}
            </span>
            {analysis.steps.length > 0 && (
              <span className="text-accent">{analysis.steps.length} explained</span>
            )}
          </span>
        )}

        <div className="ml-auto flex shrink-0 items-center gap-3">
          {error && <span className="text-[12px] text-del">{error}</span>}
          {analysis.snapshot_changed && !analysis.stale && (
            <span className="font-mono text-[10px] text-muted" title="Press Refresh to re-read git">
              analysis from an earlier snapshot
            </span>
          )}
          <button
            type="button"
            onClick={state.refresh}
            disabled={loading}
            className="rounded border border-edge px-2.5 py-1 text-[12px] text-ink-dim transition-colors enabled:hover:border-accent enabled:hover:text-ink disabled:opacity-40"
          >
            {loading ? 'Refreshing...' : 'Refresh'}
          </button>
        </div>
      </header>

      {files.length === 0 && !loading ? (
        <main className="flex flex-1 flex-col items-center justify-center gap-2 text-center">
          <p className="text-[15px] text-ink">No uncommitted changes</p>
          <p className="max-w-sm text-muted">
            Edit some files in this repository and press Refresh to compare them against{' '}
            {repository?.base ?? 'HEAD'}.
          </p>
        </main>
      ) : (
        <main className="flex min-h-0 flex-1">
          <FileList
            width={left.width}
            files={files}
            selectedPath={selectedFile}
            explainedFiles={explainedFiles}
            onSelect={selectFile}
          />
          <Splitter label="Resize the file list" onResize={left.resizeBy} onReset={left.reset} />
          <DiffViewer
            file={file}
            diff={diff}
            loading={diffLoading}
            selectedChangeId={selectedChangeId}
            onSelectChange={state.selectChange}
          />
          <Splitter
            label="Resize the learning guide"
            onResize={(delta) => right.resizeBy(-delta)}
            onReset={right.reset}
          />
          <LearningPanel
            width={right.width}
            analysis={analysis}
            file={selectedFile}
            units={units}
            currentChangeId={selectedChangeId}
            onSelectUnit={state.selectChange}
            onLeaveFile={leaveFile}
          />
        </main>
      )}
    </div>
  )
}
