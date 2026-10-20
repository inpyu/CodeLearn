import Markdown from './Markdown'
import type { Analysis, LearningStep } from '../types'

interface Props {
  analysis: Analysis
  /** Steps are scoped to the file selected on the left. */
  file: string | null
  currentChangeId: string | null
  onSelectStep: (step: LearningStep) => void
  onLeaveFile: (direction: -1 | 1) => void
}

function Section({ label, body }: { label: string; body: string }) {
  return (
    <div>
      <h4 className="mb-1 text-[11px] font-semibold tracking-wide text-accent uppercase">
        {label}
      </h4>
      <Markdown source={body} />
    </div>
  )
}

function lineRange(step: LearningStep): string {
  if (step.new_range.count <= 0) return `removed near line ${step.old_range.start}`
  const { start, count } = step.new_range
  return count === 1 ? `line ${start}` : `lines ${start}-${start + count - 1}`
}

function Frame({ children }: { children: React.ReactNode }) {
  return (
    <aside className="flex h-full w-96 shrink-0 flex-col border-l border-edge bg-panel">
      {children}
    </aside>
  )
}

export default function LearningPanel({
  analysis,
  file,
  currentChangeId,
  onSelectStep,
  onLeaveFile,
}: Props) {
  const steps = analysis.steps.filter((step) => step.file === file)
  const index = steps.findIndex((step) => step.change_id === currentChangeId)
  const step = index >= 0 ? steps[index] : null

  const move = (direction: -1 | 1) => {
    const next = index + direction
    if (next >= 0 && next < steps.length) {
      onSelectStep(steps[next])
      return
    }
    onLeaveFile(direction)
  }

  const header = (
    <header className="flex items-center justify-between border-b border-edge px-3 py-2">
      <span className="text-[11px] font-semibold tracking-wide text-muted uppercase">
        Learning Guide
      </span>
      {steps.length > 0 && (
        <span className="font-mono text-[11px] text-muted">
          {index >= 0 ? `${index + 1} / ${steps.length}` : `${steps.length} explained`}
        </span>
      )}
    </header>
  )

  if (!file) {
    return (
      <Frame>
        {header}
        <p className="px-3 py-4 text-[13px] text-muted">Select a file to see its explanations.</p>
      </Frame>
    )
  }

  if (steps.length === 0) {
    return (
      <Frame>
        {header}
        <div className="flex-1 overflow-y-auto px-3 py-4 text-[13px] leading-relaxed text-muted">
          <p className="font-mono text-[11px] text-ink">{file}</p>
          <p className="mt-2">
            {analysis.available
              ? 'No explanation was generated for this file.'
              : 'No learning explanations have been generated yet.'}
          </p>
          <p className="mt-2">
            Ask your coding agent to analyse the current changes with the CodeLearn skill, then
            press Refresh.
          </p>
          <pre className="mt-3 overflow-x-auto rounded-md border border-edge bg-shell p-2 font-mono text-[11px]">
            <code>{'codelearn export --output changes.json\ncodelearn import analysis.json'}</code>
          </pre>
        </div>
      </Frame>
    )
  }

  return (
    <Frame>
      {header}

      <nav className="max-h-48 shrink-0 overflow-y-auto border-b border-edge">
        <p className="px-3 pt-2 font-mono text-[10px] text-muted">{file}</p>
        <ol className="pb-1">
          {steps.map((candidate, position) => (
            <li key={candidate.change_id}>
              <button
                type="button"
                onClick={() => onSelectStep(candidate)}
                className={`flex w-full items-baseline gap-2 px-3 py-1.5 text-left hover:bg-edge/50 ${
                  position === index ? 'bg-edge' : ''
                }`}
              >
                <span className="font-mono text-[11px] text-muted">{position + 1}</span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-mono text-[12px] text-accent">
                    {candidate.label}
                  </span>
                  <span className="block truncate text-[12px] text-ink">{candidate.title}</span>
                </span>
              </button>
            </li>
          ))}
        </ol>
      </nav>

      <div className="min-h-0 flex-1 overflow-y-auto px-3 py-3">
        {step ? (
          <div className="space-y-4">
            <div>
              <p className="font-mono text-[12px] text-accent">{step.label}</p>
              <h3 className="text-[15px] font-semibold text-ink">{step.title}</h3>
              <p className="mt-0.5 font-mono text-[11px] text-muted">{lineRange(step)}</p>
            </div>
            <Section label="What" body={step.what} />
            <Section label="How" body={step.how} />
            {step.why ? (
              <Section label="Why" body={step.why} />
            ) : (
              <div>
                <h4 className="mb-1 text-[11px] font-semibold tracking-wide text-muted uppercase">
                  Why
                </h4>
                <p className="text-[13px] text-muted">
                  Not stated - the code alone did not give enough evidence.
                </p>
              </div>
            )}
          </div>
        ) : (
          <p className="text-[13px] text-muted">
            The selected change unit has no explanation. Pick one of the units above.
          </p>
        )}
      </div>

      <footer className="flex items-center justify-between gap-2 border-t border-edge px-3 py-2">
        <button
          type="button"
          onClick={() => move(-1)}
          className="rounded border border-edge px-3 py-1 text-[12px] text-ink hover:border-accent"
        >
          Prev
        </button>
        {analysis.stale && (
          <span
            className="text-[11px] text-amber-300"
            title="Steps whose code changed are withheld rather than shown against the wrong lines"
          >
            {analysis.dropped} step(s) dropped
          </span>
        )}
        <button
          type="button"
          onClick={() => move(1)}
          className="rounded border border-edge px-3 py-1 text-[12px] text-ink hover:border-accent"
        >
          Next
        </button>
      </footer>
    </Frame>
  )
}
