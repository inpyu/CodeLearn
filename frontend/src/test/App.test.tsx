import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import App from '../App'
import {
  analysis,
  cacheDiff,
  cacheFile,
  emptyAnalysis,
  notesFile,
  stubAPI,
} from './fixtures'

// The Monaco pane is lazily loaded and does not run in jsdom; the tests cover
// everything around it. Monaco's own rendering is checked in the browser.
vi.mock('../components/DiffEditorPane', () => ({
  default: ({ selected }: { selected: { label: string } | null }) => (
    <div data-testid="diff-editor">editor:{selected?.label ?? 'none'}</div>
  ),
}))

describe('viewer', () => {
  it('renders the changed file list', async () => {
    stubAPI()
    render(<App />)

    expect(await screen.findByText('Changed Files (2)')).toBeInTheDocument()
    expect(screen.getByText('cache.go')).toBeInTheDocument()
    expect(screen.getByText('notes.txt')).toBeInTheDocument()
  })

  it('requests the diff of the selected file only', async () => {
    const requested: string[] = []
    stubAPI({ onDiff: (path) => requested.push(path) })
    render(<App />)

    await screen.findByText('Changed Files (2)')
    await waitFor(() => expect(requested).toContain(cacheFile.path))
    expect(requested).not.toContain(notesFile.path)

    await userEvent.click(screen.getByRole('button', { name: /notes\.txt/ }))
    await waitFor(() => expect(requested).toContain(notesFile.path))
  })

  it('scopes the learning guide to the selected file', async () => {
    stubAPI()
    render(<App />)

    // cache.go has the only step, and the viewer opens there.
    expect(await screen.findByRole('heading', { name: 'Cache reuse' })).toBeInTheDocument()
    expect(screen.getByText('Creation became reuse.')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /notes\.txt/ }))
    await waitFor(() =>
      expect(screen.getByText('No explanation was generated for this file.')).toBeInTheDocument(),
    )
    expect(screen.queryByRole('heading', { name: 'Cache reuse' })).not.toBeInTheDocument()
  })

  it('shows the empty state when nothing has been analysed', async () => {
    stubAPI({ analysis: emptyAnalysis })
    render(<App />)

    expect(
      await screen.findByText('No learning explanations have been generated yet.'),
    ).toBeInTheDocument()
  })

  it('moves the editor when a change unit is selected', async () => {
    stubAPI()
    render(<App />)

    expect(await screen.findByTestId('diff-editor')).toHaveTextContent('editor:Get()')

    await userEvent.click(screen.getByRole('button', { name: /module level/ }))
    await waitFor(() =>
      expect(screen.getByTestId('diff-editor')).toHaveTextContent('editor:module level'),
    )
  })

  it('moves to the code of a learning step when the step is selected', async () => {
    stubAPI()
    render(<App />)

    await screen.findByTestId('diff-editor')
    await userEvent.click(screen.getByRole('button', { name: /module level/ }))
    await waitFor(() =>
      expect(screen.getByTestId('diff-editor')).toHaveTextContent('editor:module level'),
    )

    await userEvent.click(screen.getByText('Cache reuse'))
    await waitFor(() => expect(screen.getByTestId('diff-editor')).toHaveTextContent('editor:Get()'))
  })

  it('keeps the selected file across a refresh', async () => {
    stubAPI()
    render(<App />)

    await screen.findByText('Changed Files (2)')
    await userEvent.click(screen.getByRole('button', { name: /notes\.txt/ }))
    const selectedFileLabel = () =>
      screen.getByText('No explanation was generated for this file.')
    await waitFor(() => expect(selectedFileLabel()).toBeInTheDocument())

    await userEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    await waitFor(() => expect(selectedFileLabel()).toBeInTheDocument())
  })

  it('asks the server for a new snapshot only when Refresh is pressed', async () => {
    let refreshes = 0
    stubAPI({ onRefresh: () => (refreshes += 1) })
    render(<App />)

    await screen.findByText('Changed Files (2)')
    // The initial load reuses the snapshot the CLI already built.
    expect(refreshes).toBe(0)

    await userEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    await waitFor(() => expect(refreshes).toBe(1))
  })

  it('marks a file whose change exists only in the index', async () => {
    stubAPI({
      files: [{ ...cacheFile, index_only: true }],
      diffs: { [cacheFile.path]: { ...cacheDiff, index_only: true } },
    })
    render(<App />)

    expect(await screen.findByText('staged only')).toBeInTheDocument()
    expect(
      screen.getByText('staged only - comparing HEAD with the index'),
    ).toBeInTheDocument()
  })

  it('reports an API error', async () => {
    // A fresh Response per call: a Response body can only be read once.
    vi.spyOn(globalThis, 'fetch').mockImplementation(
      async () => new Response(JSON.stringify({ detail: 'git exploded' }), { status: 500 }),
    )
    render(<App />)

    expect(await screen.findByText('git exploded')).toBeInTheDocument()
  })

  it('warns when steps were dropped because their code changed', async () => {
    stubAPI({ analysis: { ...analysis, stale: true, dropped: 2 } })
    render(<App />)

    expect(await screen.findByText('2 step(s) dropped')).toBeInTheDocument()
  })
})
