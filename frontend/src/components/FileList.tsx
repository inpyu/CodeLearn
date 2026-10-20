import type { ChangedFile, FileStatus } from '../types'

const STATUS_LABEL: Record<FileStatus, string> = {
  modified: 'M',
  added: 'A',
  deleted: 'D',
  renamed: 'R',
  untracked: 'U',
  type_changed: 'T',
}

const STATUS_COLOR: Record<FileStatus, string> = {
  modified: 'text-amber-300',
  added: 'text-emerald-300',
  deleted: 'text-rose-300',
  renamed: 'text-sky-300',
  untracked: 'text-violet-300',
  type_changed: 'text-amber-300',
}

interface Props {
  files: ChangedFile[]
  selectedPath: string | null
  explainedFiles: Set<string>
  onSelect: (path: string) => void
}

export default function FileList({ files, selectedPath, explainedFiles, onSelect }: Props) {
  return (
    <aside className="flex h-full w-72 shrink-0 flex-col border-r border-edge bg-panel">
      <header className="border-b border-edge px-3 py-2 text-[11px] font-semibold tracking-wide text-muted uppercase">
        Changed Files ({files.length})
      </header>
      <div className="flex-1 overflow-y-auto">
        {files.length === 0 && (
          <p className="px-3 py-4 text-[13px] text-muted">No uncommitted changes.</p>
        )}
        <ul>
          {files.map((file) => {
            const name = file.path.split('/').pop() ?? file.path
            const directory = file.path.slice(0, file.path.length - name.length)
            return (
              <li key={file.path}>
                <button
                  type="button"
                  onClick={() => onSelect(file.path)}
                  className={`flex w-full items-start gap-2 px-3 py-2 text-left hover:bg-edge/50 ${
                    file.path === selectedPath ? 'bg-edge' : ''
                  }`}
                >
                  <span
                    className={`mt-0.5 w-3 shrink-0 font-mono text-[12px] ${STATUS_COLOR[file.status]}`}
                    title={file.status}
                  >
                    {STATUS_LABEL[file.status]}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="flex items-center gap-1.5">
                      <span className="truncate text-[13px] text-ink">{name}</span>
                      {explainedFiles.has(file.path) && (
                        <span
                          className="h-1.5 w-1.5 shrink-0 rounded-full bg-accent"
                          title="Has learning explanations"
                        />
                      )}
                    </span>
                    {directory && (
                      <span className="block truncate text-[11px] text-muted">{directory}</span>
                    )}
                    <span className="mt-0.5 block font-mono text-[11px] text-muted">
                      {file.binary ? (
                        'binary'
                      ) : (
                        <>
                          <span className="text-emerald-400">+{file.additions}</span>{' '}
                          <span className="text-rose-400">-{file.deletions}</span>
                          {' · '}
                          {file.change_ids.length} unit
                          {file.change_ids.length === 1 ? '' : 's'}
                        </>
                      )}
                      {file.old_path && ` · from ${file.old_path}`}
                    </span>
                  </span>
                </button>
              </li>
            )
          })}
        </ul>
      </div>
    </aside>
  )
}
