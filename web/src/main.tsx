import React from 'react'
import ReactDOM from 'react-dom/client'
import { App } from './App'
import { DEMO_STATIC, installDemoApi } from './demoApi'
import './i18n'
import './App.css'

// Zero-install demo build: serve all /api calls from canned fixtures.
if (DEMO_STATIC) {
  installDemoApi()
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
