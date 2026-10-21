import '@testing-library/jest-dom/vitest'
import { afterEach, vi } from 'vitest'
import { cleanup } from '@testing-library/react'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  window.localStorage.clear()
})

// jsdom does not implement PointerEvent, and the splitter listens for it.
if (!('PointerEvent' in window)) {
  // @ts-expect-error - a MouseEvent carries everything the splitter reads.
  window.PointerEvent = MouseEvent
}
