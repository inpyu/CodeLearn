import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import App from '../App'
import { analysis, cacheDiff, cacheFile, emptyAnalysis, notesFile, stubAPI } from './fixtures'

// The Monaco pane is lazily loaded and does not run in jsdom; the tests cover
// everything around it. Monaco's own rendering is checked in the browser.
vi.mock('../components/DiffEditorPane', () => ({
  default: ({ selected }: { selected: { label: string } | null }) => (
    <div data-testid="diff-editor">editor:{selected?.label ?? 'none'}</div>
  ),
}))

const file = (name: RegExp) => screen.getByRole('button', { name })

describe('viewer', () => {
  it('renders the changed file list with totals', async () => {
    stubAPI()
    render(<App />)

    expect(await screen.findByText('Changed Files')).toBeInTheDocument()
    expect(screen.getByText('cache.go')).toBeInTheDocument()
    expect(screen.getByText('notes.txt')).toBeInTheDocument()
    // Header stats fill the band that would otherwise be empty.
    expect(screen.getByText('2 files')).toBeInTheDocument()
    expect(screen.getByText('1 explained')).toBeInTheDocument()
  })

  it('requests the diff of the selected file only', async () => {
    const requested: string[] = []
    stubAPI({ onDiff: (path) => requested.push(path) })
    render(<App />)

    await screen.findByText('Changed Files')
    await waitFor(() => expect(requested).toContain(cacheFile.path))
    expect(requested).not.toContain(notesFile.path)

    await userEvent.click(file(/notes\.txt/))
    await waitFor(() => expect(requested).toContain(notesFile.path))
  })

  it('lists every unit of the selected file, explained or not', async () => {
    stubAPI()
    render(<App />)

    // cache.go has two units; only the first carries an explanation, and the
    // second is listed as unexplained rather than hidden.
    expect(await screen.findByRole('heading', { name: 'Cache reuse' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Get\(\).*Cache reuse/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /module level.*no explanation/ })).toBeInTheDocument()
    expect(screen.getByText('1 explained')).toBeInTheDocument()
  })

  it('scopes the learning guide to the selected file', async () => {
    stubAPI()
    render(<App />)

    expect(await screen.findByRole('heading', { name: 'Cache reuse' })).toBeInTheDocument()
    expect(screen.getByText('Creation became reuse.')).toBeInTheDocument()

    await userEvent.click(file(/notes\.txt/))
    await waitFor(() =>
      expect(screen.getByRole('heading', { name: 'Not explained yet' })).toBeInTheDocument(),
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

  it('moves the editor when a unit is selected in the guide', async () => {
    stubAPI()
    render(<App />)

    expect(await screen.findByTestId('diff-editor')).toHaveTextContent('editor:Get()')

    await userEvent.click(screen.getByRole('button', { name: /module level/ }))
    await waitFor(() =>
      expect(screen.getByTestId('diff-editor')).toHaveTextContent('editor:module level'),
    )
  })

  it('walks units with Prev and Next', async () => {
    stubAPI()
    render(<App />)

    await screen.findByTestId('diff-editor')
    expect(screen.getByText('1 / 2')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Next' }))
    await waitFor(() => expect(screen.getByText('2 / 2')).toBeInTheDocument())

    await userEvent.click(screen.getByRole('button', { name: 'Prev' }))
    await waitFor(() => expect(screen.getByText('1 / 2')).toBeInTheDocument())
  })

  it('keeps the selected file across a refresh', async () => {
    stubAPI()
    render(<App />)

    await screen.findByText('Changed Files')
    await userEvent.click(file(/notes\.txt/))
    const marker = () => screen.getByRole('heading', { name: 'Not explained yet' })
    await waitFor(() => expect(marker()).toBeInTheDocument())

    await userEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    await waitFor(() => expect(marker()).toBeInTheDocument())
  })

  it('asks the server for a new snapshot only when Refresh is pressed', async () => {
    let refreshes = 0
    stubAPI({ onRefresh: () => (refreshes += 1) })
    render(<App />)

    await screen.findByText('Changed Files')
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
    expect(screen.getByText('staged only · HEAD vs index')).toBeInTheDocument()
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

describe('layout', () => {
  it('remembers a pane width the reader dragged', async () => {
    stubAPI()
    const { unmount } = render(<App />)

    await screen.findByText('Changed Files')
    const splitter = screen.getByRole('separator', { name: 'Resize the file list' })

    // jsdom has no layout, so the drag is driven through the pointer events
    // the splitter listens for.
    splitter.dispatchEvent(
      new MouseEvent('pointerdown', { bubbles: true, clientX: 256 }) as PointerEvent,
    )
    window.dispatchEvent(new MouseEvent('pointermove', { clientX: 336 }) as PointerEvent)
    window.dispatchEvent(new MouseEvent('pointerup', { clientX: 336 }) as PointerEvent)

    await waitFor(() => expect(window.localStorage.getItem('codelearn.pane.files')).toBe('336'))
    unmount()

    // A fresh mount picks the stored width back up.
    render(<App />)
    await screen.findByText('Changed Files')
    expect(window.localStorage.getItem('codelearn.pane.files')).toBe('336')
  })

  it('keeps a stored width inside its bounds', async () => {
    window.localStorage.setItem('codelearn.pane.files', '5000')
    stubAPI()
    render(<App />)

    await screen.findByText('Changed Files')
    const panel = screen.getByText('Changed Files').closest('aside')
    // An out-of-range stored value is ignored in favour of the default.
    expect(panel).toHaveStyle({ width: '256px' })
  })
})
