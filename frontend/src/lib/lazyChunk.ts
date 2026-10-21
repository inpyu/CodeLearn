import { lazy, type ComponentType } from 'react'

const RELOADED = 'codelearn.reloaded-for-stale-chunk'

function alreadyReloaded(): boolean {
  try {
    return window.sessionStorage.getItem(RELOADED) === '1'
  } catch {
    // Session storage can be unavailable; then the reload is simply not
    // attempted, and the error boundary explains the problem instead.
    return true
  }
}

function rememberReload(): void {
  try {
    window.sessionStorage.setItem(RELOADED, '1')
  } catch {
    /* ignore: the guard below is best effort */
  }
}

/** Clears the guard once the page has loaded successfully. */
export function chunkLoadSucceeded(): void {
  try {
    window.sessionStorage.removeItem(RELOADED)
  } catch {
    /* ignore */
  }
}

/**
 * Lazily loads a component, recovering from a stale asset manifest.
 *
 * Vite gives every chunk a content-hashed name, so rebuilding CodeLearn
 * changes them. A browser tab opened before the rebuild still holds the old
 * names, and the first dynamic import after it - clicking a file, which pulls
 * in the Monaco pane - asks for a file the new binary does not have. The
 * request 404s and the import rejects.
 *
 * Retrying the same URL can never succeed, because the file genuinely no
 * longer exists. What is stale is the page itself, so the fix is to fetch it
 * again. Assets are served with `Cache-Control: no-store`, so a reload is
 * guaranteed to pick up the new manifest.
 *
 * The guard makes this happen at most once per tab: if the import fails again
 * after a reload the cause is something else, and the error is allowed
 * through to the error boundary rather than starting a reload loop.
 */
export function lazyWithReload<P extends object>(
  load: () => Promise<{ default: ComponentType<P> }>,
) {
  return lazy<ComponentType<P>>(() =>
    load().catch((cause: unknown) => {
      if (alreadyReloaded()) throw cause
      rememberReload()
      window.location.reload()
      // Hang until the reload takes effect, rather than flashing an error the
      // reader cannot act on.
      return new Promise<{ default: ComponentType<P> }>(() => {})
    }),
  )
}
