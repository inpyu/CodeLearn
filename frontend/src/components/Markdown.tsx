import ReactMarkdown from 'react-markdown'

/**
 * Renders an explanation.
 *
 * Analysis text is written by an agent that read repository code, so it is
 * untrusted input. react-markdown does not render raw HTML unless rehype-raw
 * is added, and it is deliberately not added here.
 */
export default function Markdown({ source }: { source: string }) {
  return (
    <div className="space-y-2 text-[13px] leading-relaxed text-ink/90">
      <ReactMarkdown
        components={{
          p: ({ children }) => <p>{children}</p>,
          ul: ({ children }) => <ul className="ml-4 list-disc space-y-1">{children}</ul>,
          ol: ({ children }) => <ol className="ml-4 list-decimal space-y-1">{children}</ol>,
          code: ({ children }) => (
            <code className="rounded bg-edge/70 px-1 py-0.5 font-mono text-[12px]">
              {children}
            </code>
          ),
          pre: ({ children }) => (
            <pre className="overflow-x-auto rounded-md border border-edge bg-shell p-2 font-mono text-[12px]">
              {children}
            </pre>
          ),
          strong: ({ children }) => (
            <strong className="font-semibold text-ink">{children}</strong>
          ),
          // Links are rendered as plain text: an imported analysis should not
          // be able to put a clickable destination in front of the reader.
          a: ({ children }) => <span className="text-ink">{children}</span>,
        }}
      >
        {source}
      </ReactMarkdown>
    </div>
  )
}
