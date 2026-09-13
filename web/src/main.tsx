import React from 'react'
import ReactDOM from 'react-dom/client'
import { App } from './App'
import { DEMO_STATIC, installDemoApi } from './demoApi'
import './i18n'
import './App.css'
import { authenticatedFetch, downloadAPI } from './apiAuth'

// Zero-install demo build: serve all /api calls from canned fixtures.
if (DEMO_STATIC) {
  installDemoApi()
}
window.fetch = authenticatedFetch(window.fetch.bind(window), window.location.origin)
document.addEventListener('click', event => {
  const anchor = event.target instanceof Element ? event.target.closest('a[href]') : null
  if (!(anchor instanceof HTMLAnchorElement)) return
  const url = new URL(anchor.href)
  if (url.origin !== window.location.origin || !url.pathname.startsWith('/api/')) return
  event.preventDefault()
  void downloadAPI(url.pathname + url.search).catch(() => window.alert('Download failed. Check the selected server and authentication.'))
})

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
