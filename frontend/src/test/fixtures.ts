import { vi } from 'vitest'

import type { Analysis, ChangedFile, FileDiff, LearningStep, RepositoryInfo } from '../types'

export const repository: RepositoryInfo = {
  name: 'sample-repo',
  path: '/tmp/sample-repo',
  branch: 'main',
  base: 'HEAD',
  base_commit: 'abc123',
  snapshot_id: 'sha256:' + 'a'.repeat(64),
  changed_files: 2,
  version: '0.3.0',
}

export const cacheFile: ChangedFile = {
  path: 'internal/cache.go',
  status: 'modified',
  binary: false,
  additions: 3,
  deletions: 1,
  change_ids: ['sha256:' + '1'.repeat(64), 'sha256:' + '2'.repeat(64)],
}

export const notesFile: ChangedFile = {
  path: 'notes.txt',
  status: 'untracked',
  binary: false,
  additions: 2,
  deletions: 0,
  change_ids: ['sha256:' + '3'.repeat(64)],
}

export const cacheDiff: FileDiff = {
  snapshot_id: repository.snapshot_id,
  path: cacheFile.path,
  binary: false,
  too_large: false,
  before: 'package cache\n\nfunc Get() int {\n\treturn create()\n}\n',
  after: 'package cache\n\nfunc Get() int {\n\treturn reuse()\n}\n',
  changes: [
    {
      id: cacheFile.change_ids[0],
      symbol: 'Get',
      kind: 'function',
      label: 'Get()',
      old_range: { start: 3, count: 3 },
      new_range: { start: 3, count: 3 },
    },
    {
      id: cacheFile.change_ids[1],
      symbol: null,
      kind: 'block',
      label: 'module level',
      old_range: { start: 1, count: 1 },
      new_range: { start: 1, count: 1 },
    },
  ],
}

export const notesDiff: FileDiff = {
  snapshot_id: repository.snapshot_id,
  path: notesFile.path,
  binary: false,
  too_large: false,
  before: '',
  after: 'alpha\nbravo\n',
  changes: [
    {
      id: notesFile.change_ids[0],
      symbol: null,
      kind: 'block',
      label: 'module level',
      old_range: { start: 1, count: 0 },
      new_range: { start: 1, count: 2 },
    },
  ],
}

export const step: LearningStep = {
  change_id: cacheFile.change_ids[0],
  title: 'Cache reuse',
  what: 'Creation became reuse.',
  how: 'The existing object is returned.',
  why: 'Avoids paying construction cost per call.',
  file: cacheFile.path,
  symbol: 'Get',
  kind: 'function',
  label: 'Get()',
  old_range: { start: 3, count: 3 },
  new_range: { start: 3, count: 3 },
}

export const analysis: Analysis = {
  snapshot_id: repository.snapshot_id,
  available: true,
  stale: false,
  snapshot_changed: false,
  dropped: 0,
  steps: [step],
}

export const emptyAnalysis: Analysis = {
  snapshot_id: repository.snapshot_id,
  available: false,
  stale: false,
  snapshot_changed: false,
  dropped: 0,
  steps: [],
}

/** Install a fetch stub covering every endpoint the viewer calls. */
export function stubAPI(overrides: {
  files?: ChangedFile[]
  analysis?: Analysis
  diffs?: Record<string, FileDiff>
  onDiff?: (path: string) => void
} = {}) {
  const files = overrides.files ?? [cacheFile, notesFile]
  const diffs = overrides.diffs ?? {
    [cacheFile.path]: cacheDiff,
    [notesFile.path]: notesDiff,
  }
  const analysisBody = overrides.analysis ?? analysis

  return vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
    const url = typeof input === 'string' ? input : (input as Request).url
    const json = (body: unknown) =>
      new Response(JSON.stringify(body), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })

    if (url.startsWith('/api/repository')) {
      return json({ ...repository, changed_files: files.length })
    }
    if (url.startsWith('/api/changes')) {
      return json({ snapshot_id: repository.snapshot_id, files })
    }
    if (url.startsWith('/api/analysis')) {
      return json(analysisBody)
    }
    if (url.startsWith('/api/files/diff')) {
      const path = decodeURIComponent(url.split('path=')[1] ?? '')
      overrides.onDiff?.(path)
      const diff = diffs[path]
      if (!diff) {
        return new Response(JSON.stringify({ detail: 'not a changed file' }), { status: 404 })
      }
      return json(diff)
    }
    return new Response('{}', { status: 404 })
  })
}
