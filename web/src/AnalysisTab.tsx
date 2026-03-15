import { useState, useEffect, useRef, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import i18n from './i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'

interface ScenarioResult {
  id?: number
  symbol: string
  bull_pct: number
  bear_pct: number
  sideways_pct: number
  final: string
  confidence: string
  macro_text: string
  fundamental_text: string
  sentiment_text: string
  aggregator_reason: string
}

interface HistoryRecord {
  id: number
  symbol: string
  final: string
  confidence: string
  bull_pct: number
  bear_pct: number
  sideways_pct: number
  created_at: string
}

async function postJSON<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch('/api' + path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(`HTTP ${res.status}: ${res.statusText}`)
  return res.json() as Promise<T>
}

async function getJSON<T>(path: string): Promise<T> {
  const res = await fetch('/api' + path)
  if (!res.ok) throw new Error(`HTTP ${res.status}: ${res.statusText}`)
  return res.json() as Promise<T>
}

function renderMd(text: string): string {
  return DOMPurify.sanitize(marked.parse(text) as string)
}

// ── Sub-components ────────────────────────────────────────────────────────────

const labelStyle: React.CSSProperties = {
  fontSize: '0.7rem',
  fontWeight: 600,
  letterSpacing: '0.08em',
  textTransform: 'uppercase' as const,
  color: 'var(--muted)',
  marginBottom: '8px',
}

const cardStyle: React.CSSProperties = {
  background: 'rgba(91,146,121,0.06)',
  border: '1px solid rgba(91,146,121,0.2)',
  borderRadius: '8px',
  padding: '16px',
  marginBottom: '16px',
}

function ProbBar({ label, pct, color }: { label: string; pct: number; color: string }) {
  return (
    <div style={{ marginBottom: '10px' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '4px' }}>
        <span style={{ fontSize: '0.8rem', color: 'var(--text)' }}>{label}</span>
        <span style={{ fontSize: '0.8rem', color, fontWeight: 600 }}>{pct.toFixed(1)}%</span>
      </div>
      <div style={{ height: '6px', background: 'rgba(91,146,121,0.15)', borderRadius: '3px', overflow: 'hidden' }}>
        <div style={{ height: '100%', width: `${pct}%`, background: color, borderRadius: '3px', transition: 'width 0.5s ease' }} />
      </div>
    </div>
  )
}

function AnalystPanel({ title, text }: { title: string; text: string }) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<'preview' | 'raw'>('preview')
  const content = text || t('no_analysis')

  const toggleStyle = (active: boolean): React.CSSProperties => ({
    fontSize: '0.7rem',
    fontWeight: 600,
    padding: '2px 8px',
    borderRadius: '4px',
    border: '1px solid rgba(91,146,121,0.4)',
    background: active ? 'var(--green)' : 'transparent',
    color: active ? 'var(--bg)' : 'var(--muted)',
    cursor: 'pointer',
    letterSpacing: '0.05em',
  })

  return (
    <details className="analyst-panel" style={{ marginBottom: '8px' }}>
      <summary style={{ cursor: 'pointer', fontSize: '0.85rem', color: 'var(--text)', padding: '10px 0', userSelect: 'none', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <span>{title}</span>
      </summary>
      <div style={{ padding: '12px', background: 'rgba(91,146,121,0.04)', borderRadius: '6px', marginTop: '6px' }}>
        <div style={{ display: 'flex', gap: '6px', marginBottom: '10px' }}>
          <button style={toggleStyle(mode === 'preview')} onClick={() => setMode('preview')}>Preview</button>
          <button style={toggleStyle(mode === 'raw')} onClick={() => setMode('raw')}>Markdown</button>
        </div>
        {mode === 'preview' ? (
          <div
            className="md-preview"
            dangerouslySetInnerHTML={{ __html: renderMd(content) }}
          />
        ) : (
          <pre style={{ fontSize: '0.8rem', color: 'var(--muted)', whiteSpace: 'pre-wrap', lineHeight: 1.6, margin: 0 }}>
            {content}
          </pre>
        )}
      </div>
    </details>
  )
}

// ── Main component ────────────────────────────────────────────────────────────

export function AnalysisTab() {
  const { t } = useTranslation()
  const [symbol, setSymbol] = useState('SPY')
  const [loading, setLoading] = useState(false)
  const [elapsed, setElapsed] = useState(0)
  const [result, setResult] = useState<ScenarioResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [exporting, setExporting] = useState(false)
  const [exportMsg, setExportMsg] = useState<string | null>(null)
  const [history, setHistory] = useState<HistoryRecord[]>([])
  const [historyLoading, setHistoryLoading] = useState(false)
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const loadHistory = useCallback(async (sym?: string) => {
    setHistoryLoading(true)
    try {
      const query = sym ? `?symbol=${sym}&limit=30` : '?limit=30'
      const data = await getJSON<HistoryRecord[]>('/analysis/history' + query)
      setHistory(data || [])
    } catch { /* silently ignore */ } finally {
      setHistoryLoading(false)
    }
  }, [])

  useEffect(() => { loadHistory() }, [loadHistory])

  const loadDetail = async (id: number) => {
    try {
      const rec = await getJSON<{ result: ScenarioResult }>(`/analysis/history/${id}`)
      if (rec.result) setResult(rec.result)
      window.scrollTo({ top: 0, behavior: 'smooth' })
    } catch (e) {
      setError(e instanceof Error ? e.message : t('history_load_failed'))
    }
  }

  const startAnalysis = async () => {
    setLoading(true)
    setResult(null)
    setError(null)
    setExportMsg(null)
    setElapsed(0)
    timerRef.current = setInterval(() => setElapsed(e => e + 1), 1000)
    try {
      const data = await postJSON<ScenarioResult>('/analysis/full', { symbol: symbol.toUpperCase(), timeframe: '1D', language: i18n.language })
      setResult(data)
      loadHistory(symbol.toUpperCase())
    } catch (e) {
      setError(e instanceof Error ? e.message : t('analysis_failed'))
    } finally {
      setLoading(false)
      if (timerRef.current) clearInterval(timerRef.current)
    }
  }

  const handlePrint = () => window.print()

  const handleTelegram = async () => {
    if (!result) return
    setExporting(true)
    setExportMsg(null)
    try {
      await postJSON('/analysis/export', { result })
      setExportMsg(t('telegram_sent'))
    } catch (e) {
      setExportMsg(e instanceof Error ? e.message : t('send_failed'))
    } finally {
      setExporting(false)
    }
  }

  useEffect(() => {
    return () => { if (timerRef.current) clearInterval(timerRef.current) }
  }, [])

  const finalBadgeColor = (f: string) =>
    f === 'BULL' ? 'var(--mint)' : f === 'BEAR' ? 'var(--muted)' : 'var(--green)'

  const confBadgeColor = (c: string) =>
    c === 'HIGH' ? 'var(--mint)' : c === 'MEDIUM' ? 'var(--green)' : 'var(--muted)'

  return (
    <>
      {/* Print styles injected inline */}
      <style>{`
        @media print {
          nav, .no-print { display: none !important; }
          .print-area { display: block !important; }
          body { background: #fff !important; color: #000 !important; }
          .analyst-panel[open] details { display: block; }
          .analyst-panel summary { list-style: none; }
          .md-preview { color: #000; }
        }
        .md-preview h1,.md-preview h2,.md-preview h3 { color: var(--mint); margin: 0.6em 0 0.3em; }
        .md-preview p { margin: 0.4em 0; line-height: 1.65; color: var(--text); font-size: 0.82rem; }
        .md-preview ul,.md-preview ol { padding-left: 1.4em; color: var(--muted); font-size: 0.82rem; }
        .md-preview strong { color: var(--text); }
        .md-preview code { background: rgba(91,146,121,0.12); padding: 1px 4px; border-radius: 3px; font-size: 0.78rem; }
        .md-preview hr { border: none; border-top: 1px solid rgba(91,146,121,0.2); margin: 0.8em 0; }
      `}</style>

      <section className="print-area">
        <div style={labelStyle}>{t('multi_analyst_ai')}</div>

        {/* Input row */}
        <div className="no-print" style={{ display: 'flex', gap: '8px', marginBottom: '20px', alignItems: 'center' }}>
          <input
            value={symbol}
            onChange={e => setSymbol(e.target.value.toUpperCase())}
            placeholder={t('symbol_placeholder')}
            disabled={loading}
            style={{
              background: 'rgba(91,146,121,0.06)',
              border: '1px solid rgba(91,146,121,0.25)',
              borderRadius: '6px',
              padding: '8px 12px',
              color: 'var(--text)',
              fontSize: '0.9rem',
              width: '120px',
              outline: 'none',
            }}
          />
          <button
            onClick={startAnalysis}
            disabled={loading || !symbol}
            style={{
              background: loading ? 'rgba(91,146,121,0.3)' : 'var(--green)',
              border: 'none',
              borderRadius: '6px',
              padding: '8px 20px',
              color: 'var(--bg)',
              fontSize: '0.85rem',
              fontWeight: 600,
              cursor: loading ? 'not-allowed' : 'pointer',
            }}
          >
            {loading ? t('analyzing', { elapsed }) : t('run_analysis')}
          </button>
        </div>

        {error && (
          <div style={{ color: 'var(--muted)', fontSize: '0.85rem', marginBottom: '16px', padding: '10px', background: 'rgba(143,128,115,0.1)', borderRadius: '6px' }}>
            {t('error')}: {error}
          </div>
        )}

        {result && (
          <>
            {/* Probability bars */}
            <div style={cardStyle}>
              <div style={labelStyle}>{t('scenario_probability')}</div>
              <ProbBar label={t('bull_rising')} pct={result.bull_pct} color="var(--mint)" />
              <ProbBar label={t('bear_falling')} pct={result.bear_pct} color="var(--muted)" />
              <ProbBar label={t('sideways_neutral')} pct={result.sideways_pct} color="var(--green)" />
            </div>

            {/* Final verdict */}
            <div style={{ ...cardStyle, display: 'flex', alignItems: 'center', gap: '20px' }}>
              <div>
                <div style={labelStyle}>{t('final_verdict')}</div>
                <span style={{ fontSize: '1.1rem', fontWeight: 700, color: finalBadgeColor(result.final), letterSpacing: '0.05em' }}>
                  {result.final === 'BULL' ? t('bull_buy') :
                   result.final === 'BEAR' ? t('bear_sell') : t('sideways_hold')}
                </span>
              </div>
              <div>
                <div style={labelStyle}>{t('confidence')}</div>
                <span style={{ fontSize: '0.9rem', fontWeight: 600, color: confBadgeColor(result.confidence) }}>
                  {result.confidence}
                </span>
              </div>
            </div>

            {result.aggregator_reason && (
              <div style={{ fontSize: '0.8rem', color: 'var(--muted)', marginBottom: '16px', fontStyle: 'italic' }}>
                {result.aggregator_reason}
              </div>
            )}

            {/* Analyst panels */}
            <AnalystPanel title={t('macro_analyst')} text={result.macro_text} />
            <AnalystPanel title={t('fundamental_analyst')} text={result.fundamental_text} />
            <AnalystPanel title={t('sentiment_analyst')} text={result.sentiment_text} />

            {/* Export actions */}
            <div className="no-print" style={{ display: 'flex', gap: '10px', marginTop: '20px', alignItems: 'center' }}>
              <button
                onClick={handlePrint}
                style={{ background: 'rgba(91,146,121,0.15)', border: '1px solid rgba(91,146,121,0.3)', borderRadius: '6px', padding: '8px 16px', color: 'var(--text)', fontSize: '0.82rem', cursor: 'pointer' }}
              >
                {t('save_pdf')}
              </button>
              <button
                onClick={handleTelegram}
                disabled={exporting}
                style={{ background: 'rgba(91,146,121,0.15)', border: '1px solid rgba(91,146,121,0.3)', borderRadius: '6px', padding: '8px 16px', color: 'var(--text)', fontSize: '0.82rem', cursor: exporting ? 'not-allowed' : 'pointer', opacity: exporting ? 0.6 : 1 }}
              >
                {exporting ? t('sending') : t('send_telegram')}
              </button>
              {exportMsg && (
                <span style={{ fontSize: '0.78rem', color: exportMsg === t('telegram_sent') ? 'var(--mint)' : 'var(--muted)' }}>
                  {exportMsg}
                </span>
              )}
            </div>
          </>
        )}
        {/* History section */}
        <div className="no-print" style={{ marginTop: '32px' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '10px' }}>
            <div style={labelStyle}>{t('analysis_history')}</div>
            <div style={{ display: 'flex', gap: '6px' }}>
              <button
                onClick={() => loadHistory(symbol || undefined)}
                style={{ fontSize: '0.72rem', padding: '3px 8px', background: 'transparent', border: '1px solid rgba(91,146,121,0.3)', borderRadius: '4px', color: 'var(--muted)', cursor: 'pointer' }}
              >
                {t('symbol_only', { symbol })}
              </button>
              <button
                onClick={() => loadHistory()}
                style={{ fontSize: '0.72rem', padding: '3px 8px', background: 'transparent', border: '1px solid rgba(91,146,121,0.3)', borderRadius: '4px', color: 'var(--muted)', cursor: 'pointer' }}
              >
                {t('all')}
              </button>
            </div>
          </div>

          {historyLoading && <div style={{ fontSize: '0.78rem', color: 'var(--muted)' }}>{t('loading_history')}</div>}

          {!historyLoading && history.length === 0 && (
            <div style={{ fontSize: '0.78rem', color: 'var(--muted)', fontStyle: 'italic' }}>{t('no_history')}</div>
          )}

          {history.map(rec => {
            const finalColor = rec.final === 'BULL' ? 'var(--mint)' : rec.final === 'BEAR' ? 'var(--muted)' : 'var(--green)'
            const date = new Date(rec.created_at).toLocaleString(i18n.language, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
            return (
              <div
                key={rec.id}
                onClick={() => loadDetail(rec.id)}
                style={{
                  display: 'flex', alignItems: 'center', gap: '12px',
                  padding: '8px 12px', marginBottom: '4px',
                  background: result?.id === rec.id ? 'rgba(91,146,121,0.12)' : 'rgba(91,146,121,0.04)',
                  border: `1px solid ${result?.id === rec.id ? 'rgba(91,146,121,0.4)' : 'rgba(91,146,121,0.12)'}`,
                  borderRadius: '6px', cursor: 'pointer',
                  transition: 'background 0.15s',
                }}
              >
                <span style={{ fontSize: '0.75rem', color: 'var(--muted)', minWidth: '80px' }}>{date}</span>
                <span style={{ fontSize: '0.8rem', fontWeight: 600, color: 'var(--text)', minWidth: '48px' }}>{rec.symbol}</span>
                <span style={{ fontSize: '0.78rem', fontWeight: 700, color: finalColor, minWidth: '72px' }}>
                  {rec.final === 'BULL' ? '↑ BULL' : rec.final === 'BEAR' ? '↓ BEAR' : '→ SIDEWAYS'}
                </span>
                <span style={{ fontSize: '0.72rem', color: 'var(--muted)' }}>
                  {rec.bull_pct.toFixed(0)}% / {rec.bear_pct.toFixed(0)}% / {rec.sideways_pct.toFixed(0)}%
                </span>
                <span style={{ fontSize: '0.7rem', color: 'var(--muted)', marginLeft: 'auto' }}>{rec.confidence}</span>
              </div>
            )
          })}
        </div>
      </section>
    </>
  )
}
