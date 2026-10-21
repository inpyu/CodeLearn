import { useCallback } from 'react'

interface Props {
  /** Called with the pointer's horizontal movement, in pixels. */
  onResize: (deltaX: number) => void
  /** Restores the default width. */
  onReset: () => void
  label: string
}

/**
 * A draggable divider between two panes.
 *
 * The three panes have very different appetites for width - a shallow file
 * tree needs little, a wide diff needs a lot - and that balance changes with
 * the repository, so it is left to the reader rather than guessed.
 */
export default function Splitter({ onResize, onReset, label }: Props) {
  const handlePointerDown = useCallback(
    (event: React.PointerEvent<HTMLDivElement>) => {
      event.preventDefault()
      let last = event.clientX

      const move = (moveEvent: PointerEvent) => {
        onResize(moveEvent.clientX - last)
        last = moveEvent.clientX
      }
      const release = () => {
        window.removeEventListener('pointermove', move)
        window.removeEventListener('pointerup', release)
        document.body.style.cursor = ''
        document.body.style.userSelect = ''
      }

      // Setting these on the body keeps the resize cursor steady and stops the
      // drag from selecting text across the panes it passes over.
      document.body.style.cursor = 'col-resize'
      document.body.style.userSelect = 'none'
      window.addEventListener('pointermove', move)
      window.addEventListener('pointerup', release)
    },
    [onResize],
  )

  return (
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label={label}
      title={`${label} (double-click to reset)`}
      onPointerDown={handlePointerDown}
      onDoubleClick={onReset}
      className="group w-1 shrink-0 cursor-col-resize bg-edge-soft transition-colors hover:bg-accent/60"
    />
  )
}
