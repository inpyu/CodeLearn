import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

import App from './App'
import './index.css'

// Monaco is deliberately not imported here: it is pulled in by the lazily
// loaded diff editor pane, so the file list renders before the largest chunk
// has been downloaded.
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
