import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { Suspense, type ComponentType } from 'react'

import ErrorBoundary from '../components/ErrorBoundary'
import { chunkLoadSucceeded, lazyWithReload } from '../lib/lazyChunk'

function Boom(): never {
  throw new Error('render exploded')
}

describe('error boundary', () => {
  it('shows the error instead of a blank page', () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})

    render(
      <ErrorBoundary>
        <Boom />
      </ErrorBoundary>,
    )

    expect(screen.getByText('CodeLearn hit an error')).toBeInTheDocument()
    expect(screen.getByText('render exploded')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Reload' })).toBeInTheDocument()
  })

  it('leaves a working tree alone', () => {
    render(
      <ErrorBoundary>
        <p>all fine</p>
      </ErrorBoundary>,
    )

    expect(screen.getByText('all fine')).toBeInTheDocument()
  })
})

describe('stale chunk recovery', () => {
  let reloads = 0

  beforeEach(() => {
    reloads = 0
    window.sessionStorage.clear()
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: { ...window.location, reload: () => (reloads += 1) },
    })
  })

  it('reloads once when a chunk has moved on under an open tab', async () => {
    // A rebuilt binary renames every content-hashed chunk, so the import of
    // the name this page holds 404s. Retrying it can never work; the page is
    // what is stale.
    const Missing = lazyWithReload(() =>
      Promise.reject<{ default: ComponentType<object> }>(
        new Error('Failed to fetch dynamically imported module'),
      ),
    )

    render(
      <ErrorBoundary>
        <Suspense fallback={<p>loading</p>}>
          <Missing />
        </Suspense>
      </ErrorBoundary>,
    )

    await waitFor(() => expect(reloads).toBe(1))
    // The page is still waiting for the reload rather than flashing an error.
    expect(screen.getByText('loading')).toBeInTheDocument()
  })

  it('gives up rather than looping when the reload did not help', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    window.sessionStorage.setItem('codelearn.reloaded-for-stale-chunk', '1')

    const Missing = lazyWithReload(() =>
      Promise.reject<{ default: ComponentType<object> }>(new Error('still missing')),
    )

    render(
      <ErrorBoundary>
        <Suspense fallback={<p>loading</p>}>
          <Missing />
        </Suspense>
      </ErrorBoundary>,
    )

    expect(await screen.findByText('CodeLearn hit an error')).toBeInTheDocument()
    expect(screen.getByText('still missing')).toBeInTheDocument()
    expect(reloads).toBe(0)
  })

  it('clears the guard once a page has loaded', () => {
    window.sessionStorage.setItem('codelearn.reloaded-for-stale-chunk', '1')

    chunkLoadSucceeded()

    expect(window.sessionStorage.getItem('codelearn.reloaded-for-stale-chunk')).toBeNull()
  })
})

describe('transient connection failures', () => {
  it('retries a read once when the connection drops', async () => {
    const { fetchRepository } = await import('../api')
    let attempts = 0
    vi.spyOn(globalThis, 'fetch').mockImplementation(async () => {
      attempts += 1
      if (attempts === 1) throw new TypeError('NetworkError when attempting to fetch resource.')
      return new Response(JSON.stringify({ name: 'ok' }), { status: 200 })
    })

    const body = await fetchRepository()

    expect(attempts).toBe(2)
    expect(body.name).toBe('ok')
  })

  it('does not retry a response the server actually sent', async () => {
    const { fetchChanges } = await import('../api')
    let attempts = 0
    vi.spyOn(globalThis, 'fetch').mockImplementation(async () => {
      attempts += 1
      return new Response(JSON.stringify({ detail: 'git exploded' }), { status: 500 })
    })

    await expect(fetchChanges()).rejects.toThrow('git exploded')
    expect(attempts).toBe(1)
  })

  it('does not retry after the caller aborted', async () => {
    const { fetchAnalysis } = await import('../api')
    const controller = new AbortController()
    let attempts = 0
    vi.spyOn(globalThis, 'fetch').mockImplementation(async () => {
      attempts += 1
      controller.abort()
      throw new DOMException('aborted', 'AbortError')
    })

    await expect(fetchAnalysis(controller.signal)).rejects.toThrow()
    expect(attempts).toBe(1)
  })
})
