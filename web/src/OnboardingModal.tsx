import { useState, useEffect, useRef, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import i18n from './i18n'

export const ONBOARDING_DONE_KEY = 'chartnagari_onboarding_done'

interface ScenarioResult {
  bull_pct: number
  bear_pct: number
  sideways_pct: number
}

interface OnboardingModalProps {
  onClose: () => void
  onGoToSettings?: () => void
}

export function OnboardingModal({ onClose, onGoToSettings }: OnboardingModalProps) {
  const { t } = useTranslation()

  // Step completion state
  const [step1Done, setStep1Done] = useState(false)
  const [step2Done, setStep2Done] = useState(false)
  const [activeStep, setActiveStep] = useState<1 | 2>(1)

  // Alert banner state (non-numbered, shown in right panel)
  const [alertStatus, setAlertStatus] = useState<'loading' | 'ok' | 'missing' | 'unknown'>('loading')

  // Step 1: symbol input
  const [symbol, setSymbol] = useState('')
  const [step1Loading, setStep1Loading] = useState(false)
  const [step1Error, setStep1Error] = useState('')

  // Step 2 (scan): state
  const [scanLoading, setScanLoading] = useState(false)
  const [scanElapsed, setScanElapsed] = useState(0)
  const [scanError, setScanError] = useState('')
  const [scanResult, setScanResult] = useState<ScenarioResult | null>(null)
  const [scanSymbol, setScanSymbol] = useState('')
  const [llmUnavailable, setLlmUnavailable] = useState(false)

  const modalRef = useRef<HTMLDivElement>(null)
  const symbolInputRef = useRef<HTMLInputElement>(null)
  const prevFocusRef = useRef<HTMLElement | null>(null)
  const abortRef = useRef<AbortController | null>(null)
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const scanTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  // Save previous focus; focus symbol input on open
  useEffect(() => {
    prevFocusRef.current = document.activeElement as HTMLElement
    const t = setTimeout(() => symbolInputRef.current?.focus(), 50)
    return () => {
      clearTimeout(t)
      prevFocusRef.current?.focus()
    }
  }, [])

  // Check alert channel config on mount
  const checkAlerts = useCallback(async () => {
    setAlertStatus('loading')
    try {
      const res = await fetch('/api/settings/config')
      if (res.status === 404) {
        setAlertStatus('unknown')
        return
      }
      if (!res.ok) {
        setAlertStatus('unknown')
        return
      }
      const data: Record<string, string> = await res.json()
      const hasAlert =
        (data['TELEGRAM_BOT_TOKEN'] && data['TELEGRAM_BOT_TOKEN'] !== '') ||
        (data['DISCORD_WEBHOOK_URL'] && data['DISCORD_WEBHOOK_URL'] !== '')
      setAlertStatus(hasAlert ? 'ok' : 'missing')
    } catch {
      setAlertStatus('unknown')
    }
  }, [])

  useEffect(() => { checkAlerts() }, [checkAlerts])

  // ESC = skip (close without completing) — blocked while scan is in flight
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        if (scanLoading) return
        onClose()
      }
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [onClose, scanLoading])

  // Focus trap — recalculates when panel content changes
  useEffect(() => {
    const modal = modalRef.current
    if (!modal) return
    const handler = (e: KeyboardEvent) => {
      if (e.key !== 'Tab') return
      const focusable = modal.querySelectorAll<HTMLElement>(
        'button:not([disabled]), [href], input:not([disabled]), [tabindex]:not([tabindex="-1"])'
      )
      if (focusable.length === 0) return
      const first = focusable[0]
      const last = focusable[focusable.length - 1]
      if (e.shiftKey) {
        if (document.activeElement === first) { e.preventDefault(); last.focus() }
      } else {
        if (document.activeElement === last) { e.preventDefault(); first.focus() }
      }
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [step1Done, step2Done, alertStatus, activeStep])

  // Cleanup in-flight requests on unmount
  useEffect(() => {
    return () => {
      abortRef.current?.abort()
      if (timerRef.current) clearInterval(timerRef.current)
      if (scanTimeoutRef.current) clearTimeout(scanTimeoutRef.current)
    }
  }, [])

  // Step 1: add symbol via POST /api/symbols
  const handleAddSymbol = async () => {
    const sym = symbol.trim().toUpperCase()
    if (!sym) return
    setStep1Loading(true)
    setStep1Error('')
    try {
      const res = await fetch('/api/symbols', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ symbol: sym, type: 'stock', exchange: '' }),
      })
      if (res.status === 201) {
        setScanSymbol(sym)
        setStep1Done(true)
        setActiveStep(2)
      } else {
        let msg = ''
        try { msg = await res.text() } catch { /* ignore */ }
        setStep1Error(msg || t('onboarding.step1_error'))
      }
    } catch {
      setStep1Error(t('onboarding.step1_error'))
    } finally {
      setStep1Loading(false)
    }
  }

  // Step 2 (scan): POST /api/analysis/full with AbortController + 60s timeout
  const handleScan = async () => {
    const targetSymbol = scanSymbol || symbol.trim().toUpperCase()
    setScanLoading(true)
    setScanError('')
    setScanElapsed(0)
    setLlmUnavailable(false)

    abortRef.current?.abort()
    abortRef.current = new AbortController()

    timerRef.current = setInterval(() => {
      setScanElapsed(s => s + 1)
    }, 1000)

    scanTimeoutRef.current = setTimeout(() => {
      abortRef.current?.abort()
    }, 60000)

    try {
      const res = await fetch('/api/analysis/full', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ symbol: targetSymbol, timeframe: '1D', language: i18n.language }),
        signal: abortRef.current.signal,
      })
      if (scanTimeoutRef.current) { clearTimeout(scanTimeoutRef.current); scanTimeoutRef.current = null }

      if (res.status === 503) {
        // LLM unavailable — still counts as complete
        setLlmUnavailable(true)
        setStep2Done(true)
        localStorage.setItem(ONBOARDING_DONE_KEY, '1')
      } else if (res.ok) {
        const data: ScenarioResult = await res.json()
        setScanResult(data)
        setStep2Done(true)
        localStorage.setItem(ONBOARDING_DONE_KEY, '1')
      } else {
        let msg = ''
        try { msg = await res.text() } catch { /* ignore */ }
        setScanError(msg || t('onboarding.scan_error'))
      }
    } catch (e: unknown) {
      if (scanTimeoutRef.current) { clearTimeout(scanTimeoutRef.current); scanTimeoutRef.current = null }
      if (e instanceof Error && e.name === 'AbortError') {
        setScanError(t('onboarding.scan_timeout'))
      } else {
        setScanError(t('onboarding.scan_error'))
      }
    } finally {
      if (timerRef.current) {
        clearInterval(timerRef.current)
        timerRef.current = null
      }
      setScanLoading(false)
    }
  }

  const handleShareTwitter = () => {
    if (!window.confirm(t('onboarding.share_warning'))) return
    const text = t('onboarding.share_text')
    window.open(
      `https://twitter.com/intent/tweet?text=${encodeURIComponent(text)}`,
      '_blank',
      'noopener,noreferrer'
    )
  }

  // ── Render helpers ──────────────────────────────────────────────────────────

  const renderAlertBanner = () => {
    if (alertStatus === 'loading') return null

    if (alertStatus === 'ok') {
      return (
        <div className="ob-alert-banner ob-alert-ok">
          <span>✓</span>
          <span>{t('onboarding.alert_ok')}</span>
        </div>
      )
    }

    if (alertStatus === 'missing') {
      return (
        <div className="ob-alert-banner ob-alert-warning">
          <div>{t('onboarding.alert_missing')}</div>
          <div className="ob-alert-actions">
            {onGoToSettings && (
              <button
                className="ob-link-btn"
                onClick={() => { onGoToSettings(); onClose() }}
              >
                {t('onboarding.alert_settings_link')}
              </button>
            )}
            <button className="ob-link-btn" onClick={checkAlerts}>
              {t('onboarding.alert_refresh')}
            </button>
          </div>
        </div>
      )
    }

    return (
      <div className="ob-alert-banner ob-alert-unknown">
        {t('onboarding.alert_unknown')}
      </div>
    )
  }

  const renderStep1Content = () => (
    <div className="ob-panel-content">
      <div className="ob-step-heading">{t('onboarding.step1_title')}</div>
      <p className="ob-step-desc">{t('onboarding.step1_desc')}</p>
      <div className="ob-input-row">
        <input
          ref={symbolInputRef}
          className="ob-input"
          type="text"
          placeholder="e.g. AAPL, BTC"
          value={symbol}
          onChange={e => setSymbol(e.target.value.toUpperCase())}
          onKeyDown={e => {
            if (e.key === 'Enter' && symbol.trim() && !step1Loading) handleAddSymbol()
          }}
          disabled={step1Loading || step1Done}
          autoComplete="off"
          spellCheck={false}
        />
        <button
          className="tab-btn ob-btn-primary"
          onClick={handleAddSymbol}
          disabled={!symbol.trim() || step1Loading || step1Done}
        >
          {step1Loading ? t('onboarding.adding') : t('onboarding.step1_btn')}
        </button>
      </div>
      {step1Error && <div className="ob-error" role="alert">{step1Error}</div>}
    </div>
  )

  const renderStep2Content = () => {
    const targetSymbol = scanSymbol || symbol.trim().toUpperCase()
    return (
      <div className="ob-panel-content">
        <div className="ob-step-heading">{t('onboarding.step2_title')}</div>
        <p className="ob-step-desc">
          {t('onboarding.step2_desc', { symbol: targetSymbol })}
        </p>

        {renderAlertBanner()}

        <div className="ob-scan-row">
          <button
            className="tab-btn ob-btn-primary"
            onClick={handleScan}
            disabled={scanLoading}
          >
            {scanLoading
              ? t('onboarding.scanning', { n: scanElapsed })
              : t('onboarding.step2_btn')}
          </button>
        </div>
        {scanError && <div className="ob-error" role="alert">{scanError}</div>}
      </div>
    )
  }

  const renderCompletion = () => {
    const targetSymbol = scanSymbol || symbol.trim().toUpperCase()
    return (
      <div className="ob-panel-content ob-completion">
        <div className="ob-completion-title">
          {t('onboarding.complete_title', { symbol: targetSymbol })}
        </div>

        {!llmUnavailable && scanResult ? (
          <div className="ob-ai-card">
            <ProbBar label="BULL" pct={scanResult.bull_pct} color="var(--safe)" />
            <ProbBar label="BEAR" pct={scanResult.bear_pct} color="var(--danger)" />
            <ProbBar label="SIDE" pct={scanResult.sideways_pct} color="var(--muted)" />
          </div>
        ) : (
          <div className="ob-llm-note">{t('onboarding.technical_only')}</div>
        )}

        <div className="ob-cta-group">
          <button className="tab-btn ob-btn-primary" onClick={onClose}>
            {t('onboarding.open_dashboard')}
          </button>
          <a
            className="ob-link"
            href="/api/export/pinescript"
            target="_blank"
            rel="noopener noreferrer"
          >
            {t('onboarding.pinescript_link')} ↗
          </a>
          <button className="ob-link-btn" onClick={handleShareTwitter}>
            {t('onboarding.share_twitter')}
          </button>
        </div>
      </div>
    )
  }

  const renderRightPanel = () => {
    if (step2Done) return renderCompletion()
    if (activeStep === 2) return renderStep2Content()
    return renderStep1Content()
  }

  return (
    <div className="ob-overlay">
      <div
        className="ob-modal"
        ref={modalRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="ob-title"
      >
        {/* Left Rail */}
        <div className="ob-rail">
          <div className="ob-brand">
            <span id="ob-title" className="ob-brand-name">ChartNagari</span>
            <span className="ob-brand-sub">{t('onboarding.title')}</span>
          </div>

          <nav className="ob-steps" aria-label="Onboarding steps">
            <button
              className={`ob-step-item${activeStep === 1 && !step2Done ? ' ob-step-current' : ''}`}
              onClick={() => { if (!step2Done) setActiveStep(1) }}
              disabled={step2Done}
            >
              <span className={`ob-step-num${step1Done ? ' ob-num-done' : activeStep === 1 ? ' ob-num-active' : ''}`}>
                {step1Done ? '✓' : '1'}
              </span>
              <span className="ob-step-label">{t('onboarding.step1_nav')}</span>
            </button>

            <button
              className={`ob-step-item${activeStep === 2 && !step2Done ? ' ob-step-current' : ''}`}
              onClick={() => { if (step1Done && !step2Done) setActiveStep(2) }}
              disabled={!step1Done || step2Done}
            >
              <span className={`ob-step-num${step2Done ? ' ob-num-done' : activeStep === 2 ? ' ob-num-active' : ''}`}>
                {step2Done ? '✓' : '2'}
              </span>
              <span className="ob-step-label">{t('onboarding.step2_nav')}</span>
            </button>
          </nav>

          <div className="ob-rail-footer">
            <button className="ob-skip-btn" onClick={onClose}>
              {t('onboarding.skip')}
            </button>
          </div>
        </div>

        {/* Right Work Panel */}
        <div className="ob-panel">
          {renderRightPanel()}
        </div>
      </div>
    </div>
  )
}

function ProbBar({ label, pct, color }: { label: string; pct: number; color: string }) {
  return (
    <div className="ob-prob-row">
      <span className="ob-prob-label">{label}</span>
      <div className="ob-prob-track">
        <div
          className="ob-prob-fill"
          style={{ width: `${Math.min(Math.max(pct, 0), 100)}%`, background: color }}
        />
      </div>
      <span className="ob-prob-pct">{isFinite(pct) ? pct.toFixed(0) : '?'}%</span>
    </div>
  )
}
