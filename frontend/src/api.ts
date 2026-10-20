import type { Analysis, ChangedFile, FileDiff, RepositoryInfo } from './types'

async function get<T>(path: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(path, { headers: { Accept: 'application/json' }, signal })
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
