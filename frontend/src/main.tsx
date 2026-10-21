import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

import App from './App'
import ErrorBoundary from './components/ErrorBoundary'
import { chunkLoadSucceeded } from './lib/lazyChunk'
import './index.css'

// The page loaded, so whatever made an earlier load fail is behind us.
chunkLoadSucceeded()

// Monaco is deliberately not imported here: it is pulled in by the lazily
// loaded diff editor pane, so the file list renders before the largest chunk
// has been downloaded.
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ErrorBoundary>
      <App />
    </ErrorBoundary>
  </StrictMode>,
)
