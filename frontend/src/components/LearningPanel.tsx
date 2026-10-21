import Markdown from './Markdown'
import type { Analysis, ChangeUnit, LearningStep } from '../types'

interface Props {
  width: number
  analysis: Analysis
  /** The selected file; the panel is entirely scoped to it. */
  file: string | null
  /** Every unit of that file, explained or not. */
  units: ChangeUnit[]
  currentChangeId: string | null
  onSelectUnit: (changeId: string) => void
  onLeaveFile: (direction: -1 | 1) => void
}

function Section({ label, body }: { label: string; body: string }) {
  return (
    <section>
      <h4 className="mb-1 text-[10px] font-semibold tracking-[0.08em] text-accent uppercase">
        {label}
      </h4>
      <Markdown source={body} />
    </section>
  )
}

function lineRange(unit: ChangeUnit): string {
  if (unit.new_range.count <= 0) return `removed near line ${unit.old_range.start}`
  const { start, count } = unit.new_range
  return count === 1 ? `line ${start}` : `lines ${start}-${start + count - 1}`
}

function Frame({ width, children }: { width: number; children: React.ReactNode }) {
  return (
    <aside
      style={{ width }}
      className="flex h-full shrink-0 flex-col border-l border-edge bg-panel"
    >
      {children}
    </aside>
  )
}

export default function LearningPanel({
  width,
  analysis,
  file,
  units,
  currentChangeId,
  onSelectUnit,
  onLeaveFile,
}: Props) {
  const stepFor = new Map<string, LearningStep>()
  analysis.steps
    .filter((candidate) => candidate.file === file)
    .forEach((candidate) => stepFor.set(candidate.change_id, candidate))

  const index = units.findIndex((unit) => unit.id === currentChangeId)
  const unit = index >= 0 ? units[index] : null
  const step = unit ? stepFor.get(unit.id) : undefined
  const explained = stepFor.size

  const move = (direction: -1 | 1) => {
    const next = index + direction
    if (next >= 0 && next < units.length) {
      onSelectUnit(units[next].id)
      return
    }
    onLeaveFile(direction)
  }

  const header = (
    <header className="flex items-baseline justify-between border-b border-edge px-3 py-2">
      <span className="text-[10px] font-semibold tracking-[0.08em] text-muted uppercase">
        Learning Guide
      </span>
      {units.length > 0 && (
        <span className="font-mono text-[11px] text-muted">
          {index >= 0 ? `${index + 1} / ${units.length}` : `${units.length} units`}
          {explained > 0 && <span className="text-accent"> · {explained} explained</span>}
        </span>
      )}
    </header>
  )

  if (!file) {
    return (
      <Frame width={width}>
        {header}
        <p className="px-3 py-4 text-muted">Select a file to see its change units.</p>
      </Frame>
    )
  }

  return (
    <Frame width={width}>
      {header}

      {/* Every unit of the file, so this pane is the one place to navigate
          them: an explained unit shows its title, an unexplained one says so
          rather than being hidden. */}
      {/* The list takes only the height its units need, up to a share of the
          pane; a short file must not leave a gap above the explanation. */}
      <nav className="flex max-h-[38%] shrink-0 flex-col border-b border-edge">
        <p className="truncate px-3 pt-2 pb-1 font-mono text-[10px] text-muted" title={file}>
          {file}
        </p>
        <ol className="min-h-0 overflow-y-auto pb-1">
          {units.map((candidate, position) => {
            const candidateStep = stepFor.get(candidate.id)
            const active = position === index
            return (
              <li key={candidate.id}>
                <button
                  type="button"
                  onClick={() => onSelectUnit(candidate.id)}
                  className={`flex w-full items-baseline gap-2 border-l-2 px-3 py-1.5 text-left transition-colors ${
                    active ? 'border-accent bg-raised' : 'border-transparent hover:bg-raised/60'
                  }`}
                >
                  <span className="w-4 shrink-0 font-mono text-[10px] text-muted">
                    {position + 1}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="flex items-center gap-1.5">
                      <span
                        className={`truncate font-mono text-[11px] ${
                          candidateStep ? 'text-accent' : 'text-muted'
                        }`}
                      >
                        {candidate.label}
                      </span>
                      {!candidateStep && (
                        <span className="shrink-0 text-[10px] text-muted">·</span>
                      )}
                    </span>
                    <span
                      className={`block truncate text-[12px] ${
                        candidateStep ? 'text-ink-dim' : 'text-muted italic'
                      }`}
                    >
                      {candidateStep?.title ?? 'no explanation'}
                    </span>
                  </span>
                </button>
              </li>
            )
          })}
          {units.length === 0 && (
            <li className="px-3 py-2 text-muted">This file has no change units.</li>
          )}
        </ol>
      </nav>

      <div className="min-h-0 flex-1 overflow-y-auto px-3 py-3">
        {unit ? (
          <div className="space-y-4">
            <div>
              <p className="font-mono text-[12px] text-accent">{unit.label}</p>
              {step ? (
                <h3 className="text-[15px] leading-snug font-semibold text-ink">{step.title}</h3>
              ) : (
                <h3 className="text-[15px] leading-snug font-semibold text-muted">
                  Not explained yet
                </h3>
              )}
              <p className="mt-0.5 font-mono text-[10px] text-muted">
                {unit.kind} · {lineRange(unit)}
              </p>
            </div>

            {step ? (
              <>
                <Section label="What" body={step.what} />
                <Section label="How" body={step.how} />
                {step.why ? (
                  <Section label="Why" body={step.why} />
                ) : (
                  <section>
                    <h4 className="mb-1 text-[10px] font-semibold tracking-[0.08em] text-muted uppercase">
                      Why
                    </h4>
                    <p className="text-muted">
                      Not stated - the code alone did not give enough evidence.
                    </p>
                  </section>
                )}
              </>
            ) : (
              <div className="space-y-2 text-muted">
                <p>
                  {analysis.available
                    ? 'This unit was not covered by the imported analysis.'
                    : 'No learning explanations have been generated yet.'}
                </p>
                <p>
                  Ask your coding agent to analyse the current changes with the CodeLearn skill,
                  then press Refresh.
                </p>
                <pre className="overflow-x-auto rounded-md border border-edge bg-shell p-2 font-mono text-[11px]">
                  <code>
                    {'codelearn export --output changes.json\ncodelearn import analysis.json'}
                  </code>
                </pre>
              </div>
            )}
          </div>
        ) : (
          <p className="text-muted">Pick a unit above to read it.</p>
        )}
      </div>

      <footer className="flex items-center justify-between gap-2 border-t border-edge px-3 py-2">
        <button
          type="button"
          onClick={() => move(-1)}
          className="rounded border border-edge px-2.5 py-1 text-[12px] text-ink-dim transition-colors hover:border-accent hover:text-ink"
        >
          Prev
        </button>
        {analysis.stale && (
          <span
            className="text-[10px] text-amber-300"
            title="Steps whose code changed are withheld rather than shown against the wrong lines"
          >
            {analysis.dropped} step(s) dropped
          </span>
        )}
        <button
          type="button"
          onClick={() => move(1)}
          className="rounded border border-edge px-2.5 py-1 text-[12px] text-ink-dim transition-colors hover:border-accent hover:text-ink"
        >
          Next
        </button>
      </footer>
    </Frame>
  )
}
