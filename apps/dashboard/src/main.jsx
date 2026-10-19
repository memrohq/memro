import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.jsx'

// An extension view whose page isn't served falls back to this app; don't
// load the whole dashboard again inside its own frame.
const embedded = window.self !== window.top && new URLSearchParams(window.location.search).get('embed') === '1'

createRoot(document.getElementById('root')).render(
  <StrictMode>
    {embedded ? (
      <div className="h-full flex items-center justify-center p-8 text-center">
        <p className="text-[14px] text-muted">This page isn't available right now.</p>
      </div>
    ) : (
      <App />
    )}
  </StrictMode>,
)
