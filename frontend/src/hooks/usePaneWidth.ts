import { useCallback, useState } from 'react'

/**
 * A pane width the reader can drag, remembered between visits.
 *
 * localStorage can throw outright in a private window or with site data
 * blocked, so every access is guarded and the default simply stands.
 */
export function usePaneWidth(key: string, fallback: number, min: number, max: number) {
  const [width, setWidth] = useState(() => {
    try {
      const stored = Number(window.localStorage.getItem(key))
      if (Number.isFinite(stored) && stored >= min && stored <= max) return stored
    } catch {
      /* ignore: the default is a fine answer */
    }
    return fallback
  })

  const persist = useCallback(
    (next: number) => {
      const clamped = Math.min(max, Math.max(min, next))
      setWidth(clamped)
      try {
        window.localStorage.setItem(key, String(clamped))
      } catch {
        /* ignore: resizing still works for this session */
      }
      return clamped
    },
    [key, min, max],
  )

  const resizeBy = useCallback(
    (delta: number) => setWidth((current) => persist(current + delta)),
    [persist],
  )

  const reset = useCallback(() => persist(fallback), [persist, fallback])

  return { width, resizeBy, reset }
}
