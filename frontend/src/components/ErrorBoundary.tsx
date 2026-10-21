import { Component, type ErrorInfo, type ReactNode } from 'react'

interface Props {
  children: ReactNode
}

interface State {
  error: Error | null
}

/**
 * Catches a render error and shows it.
 *
 * Without one of these, a single failure - a stale lazily loaded chunk after
 * the binary was rebuilt, say - unmounts the whole tree and leaves a blank
 * page with nothing to act on. A viewer that goes blank looks broken in a way
 * the reader cannot report or recover from, so the error is shown along with
 * the one action that fixes most of them.
 */
export default class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    // Keep the stack in the console: the message on screen is for the reader,
    // this is for whoever they report it to.
    console.error('CodeLearn failed to render', error, info.componentStack)
  }

  render(): ReactNode {
    const { error } = this.state
    if (!error) return this.props.children

    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 px-6 text-center">
        <p className="text-[15px] text-ink">CodeLearn hit an error</p>
        <p className="max-w-md font-mono text-[12px] break-words text-del">{error.message}</p>
        <p className="max-w-md text-muted">
          If CodeLearn was rebuilt while this tab was open, reloading is enough. The full stack is
          in the browser console.
        </p>
        <button
          type="button"
          onClick={() => window.location.reload()}
          className="rounded border border-edge px-3 py-1 text-[12px] text-ink-dim transition-colors hover:border-accent hover:text-ink"
        >
          Reload
        </button>
      </div>
    )
  }
}
