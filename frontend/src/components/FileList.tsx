import type { ChangedFile, FileStatus } from '../types'

const STATUS: Record<FileStatus, { letter: string; tone: string; label: string }> = {
  modified: { letter: 'M', tone: 'text-amber-300', label: 'modified' },
  added: { letter: 'A', tone: 'text-add', label: 'added' },
  deleted: { letter: 'D', tone: 'text-del', label: 'deleted' },
  renamed: { letter: 'R', tone: 'text-sky-300', label: 'renamed' },
  untracked: { letter: 'U', tone: 'text-violet-300', label: 'untracked' },
  type_changed: { letter: 'T', tone: 'text-amber-300', label: 'type changed' },
}

interface Props {
  width: number
  files: ChangedFile[]
  selectedPath: string | null
  explainedFiles: Set<string>
  onSelect: (path: string) => void
}

export default function FileList({
  width,
  files,
  selectedPath,
  explainedFiles,
  onSelect,
}: Props) {
  const totals = files.reduce(
    (sum, file) => ({
      additions: sum.additions + file.additions,
      deletions: sum.deletions + file.deletions,
      units: sum.units + file.change_ids.length,
    }),
    { additions: 0, deletions: 0, units: 0 },
  )

  return (
    <aside
      style={{ width }}
      className="flex h-full shrink-0 flex-col border-r border-edge bg-panel"
    >
      <header className="flex items-baseline justify-between border-b border-edge px-3 py-2">
        <span className="text-[10px] font-semibold tracking-[0.08em] text-muted uppercase">
          Changed Files
        </span>
        <span className="font-mono text-[11px] text-muted">{files.length}</span>
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto">
        {files.length === 0 ? (
          <p className="px-3 py-4 text-muted">No uncommitted changes.</p>
        ) : (
          <ul className="py-1">
            {files.map((file) => {
              const name = file.path.split('/').pop() ?? file.path
              const directory = file.path.slice(0, file.path.length - name.length)
              const selected = file.path === selectedPath
              const status = STATUS[file.status]
              return (
                <li key={file.path}>
                  <button
                    type="button"
                    onClick={() => onSelect(file.path)}
                    title={file.path}
                    className={`flex w-full items-start gap-2 border-l-2 px-3 py-1.5 text-left transition-colors ${
                      selected
                        ? 'border-accent bg-raised'
                        : 'border-transparent hover:bg-raised/60'
                    }`}
                  >
                    <span
                      className={`mt-[3px] w-3 shrink-0 font-mono text-[11px] ${status.tone}`}
                      title={status.label}
                    >
                      {status.letter}
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="flex items-center gap-1.5">
                        <span
                          className={`truncate ${selected ? 'text-ink' : 'text-ink-dim'}`}
                        >
                          {name}
                        </span>
                        {explainedFiles.has(file.path) && (
                          <span
                            className="h-1.5 w-1.5 shrink-0 rounded-full bg-accent"
                            title="Has learning explanations"
                          />
                        )}
                      </span>
                      {directory && (
                        <span className="block truncate font-mono text-[10px] text-muted">
                          {directory}
                        </span>
                      )}
                      <span className="mt-0.5 flex flex-wrap items-center gap-x-2 font-mono text-[10px] text-muted">
                        {file.binary ? (
                          <span>binary</span>
                        ) : (
                          <>
                            <span>
                              <span className="text-add">+{file.additions}</span>{' '}
                              <span className="text-del">-{file.deletions}</span>
                            </span>
                            <span>
                              {file.change_ids.length} unit
                              {file.change_ids.length === 1 ? '' : 's'}
                            </span>
                          </>
                        )}
                        {file.index_only && (
                          <span className="rounded bg-amber-400/15 px-1 text-amber-300">
                            staged only
                          </span>
                        )}
                      </span>
                      {file.old_path && (
                        <span className="block truncate font-mono text-[10px] text-muted">
                          from {file.old_path}
                        </span>
                      )}
                    </span>
                  </button>
                </li>
              )
            })}
          </ul>
        )}
      </div>

      {/* Totals sit at the foot of the list, where the pane would otherwise be
          empty, and answer "how big is this change" at a glance. */}
      {files.length > 0 && (
        <footer className="border-t border-edge px-3 py-2 font-mono text-[10px] text-muted">
          <div className="flex items-center justify-between">
            <span>
              <span className="text-add">+{totals.additions}</span>{' '}
              <span className="text-del">-{totals.deletions}</span>
            </span>
            <span>
              {totals.units} unit{totals.units === 1 ? '' : 's'}
            </span>
          </div>
        </footer>
      )}
    </aside>
  )
}
