import { useEffect, useRef } from 'react'
import { DiffEditor, loader } from '@monaco-editor/react'
import * as monaco from 'monaco-editor'
import type { editor } from 'monaco-editor'
import editorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker'

import { languageOf } from '../lib/language'
import type { ChangeUnit, FileDiff } from '../types'

interface Props {
  diff: FileDiff
  selected: ChangeUnit | null
}

// Monaco is bundled locally rather than fetched from a CDN, so the viewer
// works offline. Configuring it here, in the lazily loaded module, keeps it
// out of the entry chunk.
self.MonacoEnvironment = { getWorker: () => new editorWorker() }
loader.config({ monaco })

/**
 * The Monaco diff editor itself, in its own module so it can be lazy loaded:
 * Monaco is by far the largest chunk, and the file list is useful before it
 * has arrived.
 */
export default function DiffEditorPane({ diff, selected }: Props) {
  const editorRef = useRef<editor.IStandaloneDiffEditor | null>(null)
  const originalDecorations = useRef<editor.IEditorDecorationsCollection | null>(null)
  const modifiedDecorations = useRef<editor.IEditorDecorationsCollection | null>(null)

  // Highlight the selected unit and scroll it into view. Doing this from a
  // prop rather than from the click handler is what makes a jump triggered by
  // the learning panel behave exactly like a click on the unit.
  useEffect(() => {
    const instance = editorRef.current
    if (!instance) return
    const original = instance.getOriginalEditor()
    const modified = instance.getModifiedEditor()

    if (!selected) {
      originalDecorations.current?.set([])
      modifiedDecorations.current?.set([])
      return
    }

    const decorate = (start: number, count: number) =>
      count <= 0
        ? []
        : [
            {
              range: {
                startLineNumber: start,
                startColumn: 1,
                endLineNumber: start + count - 1,
                endColumn: 1,
              },
              options: {
                isWholeLine: true,
                className: 'codelearn-unit',
                marginClassName: 'codelearn-unit-margin',
              },
            },
          ]

    originalDecorations.current?.set(decorate(selected.old_range.start, selected.old_range.count))
    modifiedDecorations.current?.set(decorate(selected.new_range.start, selected.new_range.count))

    const onNewSide = selected.new_range.count > 0
    const target = onNewSide ? selected.new_range.start : selected.old_range.start
    ;(onNewSide ? modified : original).revealLineInCenter(target)
  }, [selected, diff.path])

  // Dispose the editor when the pane goes away, so switching files repeatedly
  // does not leak Monaco models.
  useEffect(
    () => () => {
      editorRef.current?.dispose()
      editorRef.current = null
    },
    [],
  )

  return (
    <DiffEditor
      key={diff.path}
      original={diff.before}
      modified={diff.after}
      language={languageOf(diff.path)}
      theme="vs-dark"
      loading={<span className="text-[13px] text-muted">Loading editor...</span>}
      onMount={(instance) => {
        editorRef.current = instance
        originalDecorations.current = instance.getOriginalEditor().createDecorationsCollection([])
        modifiedDecorations.current = instance.getModifiedEditor().createDecorationsCollection([])
      }}
      options={{
        renderSideBySide: true,
        readOnly: true,
        originalEditable: false,
        automaticLayout: true,
        renderOverviewRuler: false,
        scrollBeyondLastLine: false,
        minimap: { enabled: false },
        lineNumbers: 'on',
        fontSize: 12,
      }}
    />
  )
}
