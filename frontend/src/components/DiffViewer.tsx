import { Suspense, lazy } from 'react'

import type { ChangedFile, FileDiff } from '../types'

// Monaco is loaded only once a file is actually opened.
const DiffEditorPane = lazy(() => import('./DiffEditorPane'))

interface Props {
  file: ChangedFile | null
  diff: FileDiff | null
  loading: boolean
  selectedChangeId: string | null
  onSelectChange: (changeId: string) => void
}

function Notice({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex flex-1 items-center justify-center px-6 text-center text-muted">
      {children}
    </div>
  )
}

export default function DiffViewer({
  file,
  diff,
  loading,
  selectedChangeId,
  onSelectChange,
}: Props) {
  if (!file) {
    return (
      <section className="flex min-w-0 flex-1 flex-col">
        <Notice>Select a changed file to compare it.</Notice>
      </section>
    )
  }

  const selected = diff?.changes.find((change) => change.id === selectedChangeId) ?? null

  return (
    <section className="flex min-w-0 flex-1 flex-col bg-shell">
      <header className="flex h-9 shrink-0 items-center gap-2 border-b border-edge bg-panel px-3">
        <span className="truncate font-mono text-[12px] text-ink">{file.path}</span>
        {file.old_path && (
          <span className="shrink-0 font-mono text-[10px] text-muted">
            renamed from {file.old_path}
          </span>
        )}
        {diff?.index_only && (
          <span
            className="shrink-0 rounded bg-amber-400/15 px-1.5 py-0.5 font-mono text-[10px] text-amber-300"
            title="The working tree matches HEAD; this compares HEAD with the index"
          >
            staged only · HEAD vs index
          </span>
        )}
        {/* The selected unit is named here rather than in a row of chips: the
            full list lives in the Learning Guide, so this only has to say
            where the reader is. */}
        {selected && (
          <span className="ml-auto shrink-0 font-mono text-[11px] text-accent">
            {selected.label}
          </span>
        )}
      </header>

      {loading && !diff ? (
        <Notice>Loading {file.path}...</Notice>
      ) : !diff ? (
        <Notice>No diff available.</Notice>
      ) : diff.binary ? (
        <Notice>Binary file - no code comparison available.</Notice>
      ) : diff.too_large ? (
        <Notice>
          This file is larger than 2 MiB and is not rendered. Its change units are still listed
          in the Learning Guide.
        </Notice>
      ) : (
        <div className="min-h-0 flex-1">
          <Suspense fallback={<Notice>Loading editor...</Notice>}>
            <DiffEditorPane diff={diff} selected={selected} onSelectChange={onSelectChange} />
          </Suspense>
        </div>
      )}
    </section>
  )
}
