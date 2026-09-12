import React from 'react'
import ReactDOM from 'react-dom/client'
import { App } from './App'
import { DEMO_STATIC, installDemoApi } from './demoApi'
import './i18n'
import './App.css'
import { authenticatedFetch } from './apiAuth'

// Zero-install demo build: serve all /api calls from canned fixtures.
if (DEMO_STATIC) {
  installDemoApi()
}
window.fetch = authenticatedFetch(window.fetch.bind(window), window.location.origin)

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
