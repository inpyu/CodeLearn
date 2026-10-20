import { Suspense, lazy } from 'react'

import type { ChangedFile, ChangeUnit, FileDiff } from '../types'

// Monaco is loaded only once a file is actually opened.
const DiffEditorPane = lazy(() => import('./DiffEditorPane'))

interface Props {
  file: ChangedFile | null
  diff: FileDiff | null
  loading: boolean
  selectedChangeId: string | null
  explainedChanges: Set<string>
  onSelectChange: (changeId: string) => void
}

function Shell({ children }: { children: React.ReactNode }) {
  return (
    <section className="flex min-w-0 flex-1 items-center justify-center text-[13px] text-muted">
      {children}
    </section>
  )
}

export default function DiffViewer({
  file,
  diff,
  loading,
  selectedChangeId,
  explainedChanges,
  onSelectChange,
}: Props) {
  if (!file) return <Shell>Select a changed file to compare it.</Shell>
  if (loading && !diff) return <Shell>Loading {file.path}...</Shell>
  if (!diff) return <Shell>No diff available.</Shell>

  const selected = diff.changes.find((change) => change.id === selectedChangeId) ?? null

  return (
    <section className="flex min-w-0 flex-1 flex-col">
      <header className="flex flex-wrap items-center gap-2 border-b border-edge bg-panel px-3 py-2">
        <span className="font-mono text-[12px] text-ink">{file.path}</span>
        {file.old_path && (
          <span className="font-mono text-[11px] text-muted">renamed from {file.old_path}</span>
        )}
        <span className="ml-auto flex max-h-16 flex-wrap items-center gap-1.5 overflow-y-auto">
          {diff.changes.map((unit: ChangeUnit) => (
            <button
              key={unit.id}
              type="button"
              onClick={() => onSelectChange(unit.id)}
              title={`${unit.kind} · lines ${unit.new_range.start}+${unit.new_range.count}`}
              className={`flex items-center gap-1 rounded border px-2 py-0.5 font-mono text-[11px] ${
                unit.id === selectedChangeId
                  ? 'border-accent bg-accent/15 text-ink'
                  : 'border-edge text-muted hover:border-accent/60 hover:text-ink'
              }`}
            >
              {unit.label}
              {explainedChanges.has(unit.id) && (
                <span className="h-1.5 w-1.5 rounded-full bg-accent" />
              )}
            </button>
          ))}
        </span>
      </header>

      {diff.binary ? (
        <Shell>Binary file - no code comparison available.</Shell>
      ) : diff.too_large ? (
        <Shell>
          This file is larger than 2 MiB and is not rendered. Its change units are still listed
          above.
        </Shell>
      ) : (
        <div className="min-h-0 flex-1">
          <Suspense
            fallback={
              <div className="flex h-full items-center justify-center text-[13px] text-muted">
                Loading editor...
              </div>
            }
          >
            <DiffEditorPane diff={diff} selected={selected} />
          </Suspense>
        </div>
      )}
    </section>
  )
}
