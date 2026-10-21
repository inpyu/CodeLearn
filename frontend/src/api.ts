import type { Analysis, ChangedFile, FileDiff, RepositoryInfo } from './types'

/**
 * Reads a JSON endpoint, retrying once if the connection itself failed.
 *
 * The viewer is often reached across an SSH tunnel or a VPN, where a dropped
 * connection surfaces as a rejected fetch with no status at all. One retry
 * turns a blip into a pause instead of an error in the header. A response
 * that arrived with an error status is not retried: the server answered, and
 * asking again would give the same answer.
 */
async function get<T>(path: string, signal?: AbortSignal): Promise<T> {
  let response: Response
  try {
    response = await fetch(path, { headers: { Accept: 'application/json' }, signal })
  } catch (cause) {
    if (signal?.aborted) throw cause
    await new Promise((resolve) => setTimeout(resolve, 250))
    response = await fetch(path, { headers: { Accept: 'application/json' }, signal })
  }
  if (!response.ok) {
    const detail = await response
      .json()
      .then((body: { detail?: string }) => body.detail)
      .catch(() => undefined)
    throw new Error(detail ?? `${path} responded with ${response.status}`)
  }
  return (await response.json()) as T
}

export function fetchRepository(signal?: AbortSignal): Promise<RepositoryInfo> {
  return get<RepositoryInfo>('/api/repository', signal)
}

export function fetchChanges(
  signal?: AbortSignal,
): Promise<{ snapshot_id: string; files: ChangedFile[] }> {
  return get('/api/changes', signal)
}

/** Per-file code is loaded on demand, so the file list stays cheap. */
export function fetchFileDiff(path: string, signal?: AbortSignal): Promise<FileDiff> {
  return get<FileDiff>(`/api/files/diff?path=${encodeURIComponent(path)}`, signal)
}

export function fetchAnalysis(signal?: AbortSignal): Promise<Analysis> {
  return get<Analysis>('/api/analysis', signal)
}

/**
 * Asks the server to re-read git and build a new snapshot.
 *
 * Reads are served from the snapshot the server already holds, so this is the
 * only thing that picks up edits made since the viewer was opened. CodeLearn
 * deliberately does not watch the filesystem; Refresh is the moment the user
 * asks for new data.
 */
export async function requestRefresh(
  signal?: AbortSignal,
): Promise<{ snapshot_id: string; changed_files: number }> {
  const response = await fetch('/api/refresh', {
    method: 'POST',
    headers: { Accept: 'application/json' },
    signal,
  })
  if (!response.ok) {
    throw new Error(`/api/refresh responded with ${response.status}`)
  }
  return (await response.json()) as { snapshot_id: string; changed_files: number }
}
