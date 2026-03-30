import { useState, useEffect, useCallback, useRef, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import i18n from './i18n'
import { AnalysisTab } from './AnalysisTab'
import { OnboardingModal, ONBOARDING_DONE_KEY } from './OnboardingModal'
import {
  createChart,
  createSeriesMarkers,
  CandlestickSeries,
  HistogramSeries,
  CrosshairMode,
  type IChartApi,
  type ISeriesApi,
  type UTCTimestamp,
} from 'lightweight-charts'

// ── types ─────────────────────────────────────────────────────────────────────

type Tab = 'symbols' | 'rules' | 'status' | 'chart' | 'backtest' | 'paper' | 'report' | 'history' | 'alert' | 'performance' | 'analysis' | 'settings' | 'price-alerts' | 'calendar'

interface OHLCVBar {
  time: number
  open: number
  high: number
  low: number
  close: number
  volume: number
}

interface SignalBar {
  symbol: string
  timeframe: string
  time: number
  direction: string
  rule: string
  score: number
  message: string
  ai_interpretation: string
}

interface SymbolItem {
  symbol: string
  type: 'crypto' | 'stock'
  exchange: string
  enabled: boolean
}

interface WyckoffEvent {
  type: string
  time: number
  bar_index: number
  price: number
  volume_rel: number
}

interface WyckoffPhaseZone {
  phase: string
  start_time: number
  end_time: number
  price_low: number
  price_high: number
}

interface WyckoffAnalysis {
  symbol: string
  timeframe: string
  phase: string
  swing_high: number
  swing_low: number
  ema_50: number
  events: WyckoffEvent[]
  phase_zones: WyckoffPhaseZone[]
}

interface RuleItem {
  name: string
  enabled: boolean
  methodology: string
}

interface StatusData {
  phase: string
  symbols: number
  rules: number
  uptime_sec: number
  last_signal_unix: number
  data_sources: string[]
}

interface PriceAlert {
  ID: number
  Symbol: string
  Target: number
  Condition: 'above' | 'below'
  Note: string
  Triggered: boolean
  CreatedAt: string
  TriggeredAt?: string
}

// ── API client ────────────────────────────────────────────────────────────────

async function apiFetch<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch('/api' + path, options)
  if (!res.ok) throw new Error(`HTTP ${res.status}: ${res.statusText}`)
  if (res.status === 204) return null as T
  return res.json() as Promise<T>
}

async function putJSON(path: string, body: unknown): Promise<void> {
  await apiFetch<null>(path, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

// ── helpers ───────────────────────────────────────────────────────────────────

// abbreviateRule converts a full rule name to a short chart label.
const RULE_ABBR: Record<string, string> = {
  rsi_overbought_oversold:       'RSI',
  rsi_divergence:                'DIV',
  ema_cross:                     'EMA',
  support_resistance_breakout:   'S/R',
  fibonacci_confluence:          'FIB',
  volume_spike:                  'VOL',
  ict_order_block:               'OB',
  ict_fair_value_gap:            'FVG',
  ict_liquidity_sweep:           'LQD',
  ict_breaker_block:             'BB',
  ict_kill_zone:                 'KZ',
  wyckoff_accumulation:          'ACC',
  wyckoff_distribution:          'DIST',
  wyckoff_spring:                'SPR',
  wyckoff_upthrust:              'UP',
  wyckoff_volume_anomaly:        'VANOM',
  smc_bos:                       'BOS',
  smc_choch:                     'CHoCH',
}

function abbreviateRule(rule: string): string {
  return RULE_ABBR[rule] ?? rule.slice(0, 6).toUpperCase()
}

// ── rule descriptions ─────────────────────────────────────────────────────────

const RULE_DESC_KEYS: Record<string, string> = {
  rsi_overbought_oversold:     'rule_desc_rsi_overbought_oversold',
  rsi_divergence:              'rule_desc_rsi_divergence',
  ema_cross:                   'rule_desc_ema_cross',
  support_resistance_breakout: 'rule_desc_support_resistance_breakout',
  fibonacci_confluence:        'rule_desc_fibonacci_confluence',
  volume_spike:                'rule_desc_volume_spike',
  ict_order_block:             'rule_desc_ict_order_block',
  ict_fair_value_gap:          'rule_desc_ict_fair_value_gap',
  ict_liquidity_sweep:         'rule_desc_ict_liquidity_sweep',
  ict_breaker_block:           'rule_desc_ict_breaker_block',
  ict_kill_zone:               'rule_desc_ict_kill_zone',
  wyckoff_accumulation:        'rule_desc_wyckoff_accumulation',
  wyckoff_distribution:        'rule_desc_wyckoff_distribution',
  wyckoff_spring:              'rule_desc_wyckoff_spring',
  wyckoff_upthrust:            'rule_desc_wyckoff_upthrust',
  wyckoff_volume_anomaly:      'rule_desc_wyckoff_volume_anomaly',
  smc_bos:                     'rule_desc_smc_bos',
  smc_choch:                   'rule_desc_smc_choch',
}

// ── sub-components ────────────────────────────────────────────────────────────

function Toggle({ checked, onChange }: { checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <label className="toggle">
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} />
      <span className="slider" />
    </label>
  )
}

function SymbolsTab() {
  const { t } = useTranslation()
  const [symbols, setSymbols] = useState<SymbolItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [newSymbol, setNewSymbol] = useState('')
  const [newType, setNewType] = useState<'crypto' | 'stock'>('stock')
  const [newExchange, setNewExchange] = useState('')
  const [adding, setAdding] = useState(false)
  const [marketFilter, setMarketFilter] = useState('all')

  const markets = useMemo(() => {
    const seen = new Set<string>()
    symbols.forEach(s => {
      if (s.type === 'crypto') seen.add('crypto')
      else if (s.exchange) seen.add(s.exchange.toUpperCase())
    })
    return ['all', ...Array.from(seen).sort()]
  }, [symbols])

  const filteredSymbols = useMemo(() => {
    if (marketFilter === 'all') return symbols
    if (marketFilter === 'crypto') return symbols.filter(s => s.type === 'crypto')
    return symbols.filter(s => s.type === 'stock' && s.exchange.toUpperCase() === marketFilter)
  }, [symbols, marketFilter])

  const reload = useCallback(() => {
    apiFetch<SymbolItem[]>('/symbols')
      .then(setSymbols)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { reload() }, [reload])

  const toggle = useCallback(async (sym: SymbolItem, enabled: boolean) => {
    try {
      await putJSON(`/symbols/${encodeURIComponent(sym.symbol)}`, { enabled })
      setSymbols((prev) => prev.map((s) => (s.symbol === sym.symbol ? { ...s, enabled } : s)))
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : t('unknown_error'))
    }
  }, [t])

  const remove = useCallback(async (sym: SymbolItem) => {
    if (!confirm(t('confirm_delete', { symbol: sym.symbol }))) return
    try {
      await apiFetch<null>(`/symbols/${encodeURIComponent(sym.symbol)}`, { method: 'DELETE' })
      setSymbols((prev) => prev.filter((s) => s.symbol !== sym.symbol))
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : t('delete_failed'))
    }
  }, [t])

  const add = useCallback(async () => {
    if (!newSymbol.trim()) return
    setAdding(true)
    try {
      await apiFetch<null>('/symbols', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ symbol: newSymbol.trim().toUpperCase(), type: newType, exchange: newExchange.trim() }),
      })
      setNewSymbol('')
      setNewExchange('')
      reload()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : t('add_failed'))
    } finally {
      setAdding(false)
    }
  }, [newSymbol, newType, newExchange, reload, t])

  if (loading) return <p className="loading">{t('loading')}</p>
  if (error) return <p className="error-msg">{t('error')}: {error}</p>

  return (
    <>
      <p className="section-title">{t('symbol_management')}</p>
      {markets.length > 1 && (
        <div className="tab-group" style={{ display: 'flex', gap: 6, marginBottom: 12, flexWrap: 'wrap' }}>
          {markets.map(m => (
            <button
              key={m}
              className={`tab-btn${marketFilter === m ? ' active' : ''}`}
              onClick={() => setMarketFilter(m)}
            >
              {m === 'all' ? t('all') : m}
            </button>
          ))}
        </div>
      )}
      {filteredSymbols.map((sym) => (
        <div key={sym.symbol} className="item">
          <div>
            <div className="item-name">
              <span className={`badge badge-${sym.type}`}>{sym.type}</span>
              {sym.symbol}
            </div>
            <div className="item-meta">{sym.exchange}</div>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <Toggle checked={sym.enabled} onChange={(v) => toggle(sym, v)} />
            <button className="remove-btn" onClick={() => remove(sym)} title={t('delete')}>✕</button>
          </div>
        </div>
      ))}
      {filteredSymbols.length === 0 && <p className="loading">{t('no_symbols')}</p>}

      <p className="section-title" style={{ marginTop: 24 }}>{t('add_symbol')}</p>
      <div className="add-symbol-form">
        <select
          className="chart-select"
          value={newType}
          onChange={(e) => setNewType(e.target.value as 'crypto' | 'stock')}
        >
          <option value="stock">{t('stock')}</option>
          <option value="crypto">{t('crypto')}</option>
        </select>
        <input
          className="symbol-input"
          placeholder={t('symbol_placeholder_nvda')}
          value={newSymbol}
          onChange={(e) => setNewSymbol(e.target.value.toUpperCase())}
          onKeyDown={(e) => e.key === 'Enter' && add()}
        />
        <input
          className="symbol-input"
          placeholder={t('exchange_placeholder')}
          value={newExchange}
          onChange={(e) => setNewExchange(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && add()}
        />
        <button className="run-btn" onClick={add} disabled={adding || !newSymbol.trim()}>
          {adding ? '...' : t('add')}
        </button>
      </div>
      <p className="item-meta" style={{ marginTop: 8 }}>
        {t('restart_notice')}
      </p>
    </>
  )
}

function RulesTab() {
  const { t } = useTranslation()
  const [rules, setRules] = useState<RuleItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    apiFetch<RuleItem[]>('/rules')
      .then(setRules)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  const toggle = useCallback(async (rule: RuleItem, enabled: boolean) => {
    try {
      await putJSON(`/rules/${encodeURIComponent(rule.name)}`, { enabled })
      setRules((prev) => prev.map((r) => (r.name === rule.name ? { ...r, enabled } : r)))
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : t('unknown_error'))
    }
  }, [t])

  if (loading) return <p className="loading">{t('loading')}</p>
  if (error) return <p className="error-msg">{t('error')}: {error}</p>

  const grouped = rules.reduce<Record<string, RuleItem[]>>((acc, r) => {
    ;(acc[r.methodology] ??= []).push(r)
    return acc
  }, {})

  return (
    <>
      {Object.entries(grouped).map(([method, items]) => (
        <div key={method}>
          <p className="section-title">{method.replace('_', ' ')}</p>
          {items.map((rule) => (
            <div key={rule.name} className="item">
              <div>
                <div className="item-name">
                  <span className={`badge badge-${rule.methodology}`}>{rule.methodology}</span>
                  {rule.name}
                </div>
                {RULE_DESC_KEYS[rule.name] && (
                  <div className="item-meta">{t(RULE_DESC_KEYS[rule.name])}</div>
                )}
              </div>
              <Toggle checked={rule.enabled} onChange={(v) => toggle(rule, v)} />
            </div>
          ))}
        </div>
      ))}
      {rules.length === 0 && <p className="loading">{t('no_rules')}</p>}
    </>
  )
}

function fmtUptime(sec: number | undefined): string {
  if (!sec || isNaN(sec)) return i18n.t('calculating')
  if (sec < 60) return i18n.t('uptime_seconds', { n: sec })
  if (sec < 3600) return i18n.t('uptime_minutes', { n: Math.floor(sec / 60) })
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  return i18n.t('uptime_hours_minutes', { h, m })
}

function fmtRelTime(unix: number): string {
  if (!unix) return i18n.t('no_signals')
  const diff = Math.floor(Date.now() / 1000 - unix)
  if (diff < 60) return i18n.t('seconds_ago', { n: diff })
  if (diff < 3600) return i18n.t('minutes_ago', { n: Math.floor(diff / 60) })
  if (diff < 86400) return i18n.t('hours_ago', { n: Math.floor(diff / 3600) })
  return i18n.t('days_ago', { n: Math.floor(diff / 86400) })
}

function StatusTab() {
  const { t } = useTranslation()
  const [status, setStatus] = useState<StatusData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [tick, setTick] = useState(0)

  useEffect(() => {
    apiFetch<StatusData>('/status')
      .then(setStatus)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  // Refresh uptime and last-signal display every 30s.
  useEffect(() => {
    const id = setInterval(() => setTick((t) => t + 1), 30_000)
    return () => clearInterval(id)
  }, [])
  void tick // used to trigger re-render

  if (loading) return <p className="loading">{t('loading')}</p>
  if (error) return <p className="error-msg">{t('error')}: {error}</p>
  if (!status) return null

  return (
    <>
      <div className="status-banner">
        <span className="status-dot" />
        <span>{t('pipeline_running')}</span>
        <span className="status-uptime">{t('uptime_label', { time: fmtUptime(status.uptime_sec) })}</span>
      </div>

      <p className="section-title">{t('data_sources')}</p>
      <div className="source-list">
        {(status.data_sources ?? []).map((src) => (
          <div key={src} className="source-item">
            <span className="source-dot">✅</span>
            <span>{src}</span>
          </div>
        ))}
      </div>

      <p className="section-title">{t('analysis_status')}</p>
      <div className="status-grid">
        <div className="stat-card">
          <div className="stat-value">{status.symbols}</div>
          <div className="stat-label">{t('monitoring_symbols')}</div>
        </div>
        <div className="stat-card">
          <div className="stat-value">{status.rules}</div>
          <div className="stat-label">{t('active_rules')}</div>
        </div>
        <div className="stat-card" style={{ gridColumn: 'span 2' }}>
          <div className="stat-value" style={{ fontSize: '1.2rem' }}>
            {fmtRelTime(status.last_signal_unix)}
          </div>
          <div className="stat-label">{t('last_signal_detected')}</div>
        </div>
      </div>

      <p className="phase-info" style={{ marginTop: 16 }}>{status.phase}</p>
    </>
  )
}

// ── Chart Tab ─────────────────────────────────────────────────────────────────

const TFS = ['1H', '4H', '1D', '1W'] as const
type TF = (typeof TFS)[number]

const PHASE_COLORS: Record<string, string> = {
  accumulation: '#5B9279',
  markup:       '#8FCB9B',
  distribution: '#B47B4A',
  markdown:     '#8F8073',
  ranging:      '#5A5A5A',
}

const PHASE_LABELS: Record<string, string> = {
  accumulation: 'Accumulation',
  markup:       'Markup ↑',
  distribution: 'Distribution',
  markdown:     'Markdown ↓',
  ranging:      'Ranging',
}

function ChartTab() {
  const { t } = useTranslation()
  const [symbol, setSymbol] = useState('')
  const [symbols, setSymbols] = useState<string[]>([])
  const [tf, setTf] = useState<TF>('1H')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [signals, setSignals] = useState<SignalBar[]>([])
  const [wyckoffEnabled, setWyckoffEnabled] = useState(false)
  const [wyckoffData, setWyckoffData] = useState<WyckoffAnalysis | null>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const chartRef = useRef<IChartApi | null>(null)
  const seriesRef = useRef<ISeriesApi<'Candlestick'> | null>(null)
  const volRef = useRef<ISeriesApi<'Histogram'> | null>(null)
  const swingHighLineRef = useRef<ReturnType<ISeriesApi<'Candlestick'>['createPriceLine']> | null>(null)
  const swingLowLineRef = useRef<ReturnType<ISeriesApi<'Candlestick'>['createPriceLine']> | null>(null)

  // Load enabled symbols for the selector
  useEffect(() => {
    apiFetch<SymbolItem[]>('/symbols').then((items) => {
      const enabled = items.filter((i) => i.enabled).map((i) => i.symbol)
      setSymbols(enabled)
      if (enabled.length > 0) setSymbol(enabled[0])
    }).catch(() => {/* silently ignore */})
  }, [])

  // Create the chart instance once on mount
  useEffect(() => {
    if (!containerRef.current) return
    const chart = createChart(containerRef.current, {
      layout: {
        background: { color: '#12130F' },
        textColor: '#8F8073',
      },
      grid: {
        vertLines: { color: 'rgba(234,230,229,0.06)' },
        horzLines: { color: 'rgba(234,230,229,0.06)' },
      },
      crosshair: { mode: CrosshairMode.Normal },
      width: containerRef.current.clientWidth,
      height: 480,
      timeScale: { borderColor: 'rgba(91,146,121,0.25)' },
      rightPriceScale: { borderColor: 'rgba(91,146,121,0.25)' },
    })
    const series = chart.addSeries(CandlestickSeries, {
      upColor: '#8FCB9B',
      downColor: 'rgba(143,128,115,0.7)',
      borderUpColor: '#8FCB9B',
      borderDownColor: 'rgba(143,128,115,0.7)',
      wickUpColor: '#8FCB9B',
      wickDownColor: 'rgba(143,128,115,0.7)',
    })
    // Candlestick uses top 78% of chart, leaving bottom 22% for volume.
    series.priceScale().applyOptions({ scaleMargins: { top: 0.05, bottom: 0.22 } })

    const vol = chart.addSeries(HistogramSeries, {
      priceFormat: { type: 'volume' },
      priceScaleId: 'vol',
    })
    chart.priceScale('vol').applyOptions({
      scaleMargins: { top: 0.82, bottom: 0 },
      visible: false,
    })

    chartRef.current = chart
    seriesRef.current = series
    volRef.current = vol

    const onResize = () => {
      if (containerRef.current) chart.applyOptions({ width: containerRef.current.clientWidth })
    }
    window.addEventListener('resize', onResize)
    return () => {
      window.removeEventListener('resize', onResize)
      chart.remove()
      chartRef.current = null
      seriesRef.current = null
      volRef.current = null
    }
  }, [])

  // Load OHLCV + signals whenever symbol or TF changes
  useEffect(() => {
    if (!symbol || !seriesRef.current) return
    setLoading(true)
    setError('')

    apiFetch<OHLCVBar[]>(`/ohlcv/${encodeURIComponent(symbol)}/${tf}?limit=200`)
      .then((bars) => {
        seriesRef.current?.setData(
          bars.map((b) => ({
            time: b.time as UTCTimestamp,
            open: b.open,
            high: b.high,
            low: b.low,
            close: b.close,
          })),
        )
        volRef.current?.setData(
          bars.map((b) => ({
            time: b.time as UTCTimestamp,
            value: b.volume,
            color: b.close >= b.open
              ? 'rgba(143,203,155,0.35)'
              : 'rgba(143,128,115,0.35)',
          })),
        )
        return apiFetch<SignalBar[]>(`/signals?symbol=${encodeURIComponent(symbol)}&limit=50`)
      })
      .then((sigs) => {
        setSignals(sigs)
        if (!seriesRef.current) return
        const markers = sigs
          .filter((s) => s.direction !== 'NEUTRAL')
          .map((s) => ({
            time: s.time as UTCTimestamp,
            position: s.direction === 'LONG' ? ('belowBar' as const) : ('aboveBar' as const),
            color: s.direction === 'LONG' ? '#8FCB9B' : 'rgba(143,128,115,0.9)',
            shape: s.direction === 'LONG' ? ('arrowUp' as const) : ('arrowDown' as const),
            text: abbreviateRule(s.rule),
          }))
        createSeriesMarkers(seriesRef.current, markers)
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [symbol, tf])

  // Remove Wyckoff overlays when disabled or symbol/tf changes
  useEffect(() => {
    if (!seriesRef.current) return
    if (swingHighLineRef.current) {
      seriesRef.current.removePriceLine(swingHighLineRef.current)
      swingHighLineRef.current = null
    }
    if (swingLowLineRef.current) {
      seriesRef.current.removePriceLine(swingLowLineRef.current)
      swingLowLineRef.current = null
    }
    if (!wyckoffEnabled) {
      setWyckoffData(null)
      return
    }
    if (!symbol) return

    apiFetch<WyckoffAnalysis>(`/wyckoff/${encodeURIComponent(symbol)}/${tf}`)
      .then((data) => {
        setWyckoffData(data)
        if (!seriesRef.current) return
        if (data.swing_high > 0) {
          swingHighLineRef.current = seriesRef.current.createPriceLine({
            price: data.swing_high,
            color: 'rgba(143,203,155,0.7)',
            lineWidth: 1,
            lineStyle: 2, // dashed
            axisLabelVisible: true,
            title: 'Swing H',
          })
        }
        if (data.swing_low > 0) {
          swingLowLineRef.current = seriesRef.current.createPriceLine({
            price: data.swing_low,
            color: 'rgba(143,128,115,0.7)',
            lineWidth: 1,
            lineStyle: 2,
            axisLabelVisible: true,
            title: 'Swing L',
          })
        }
        // Draw Spring / Upthrust event markers on top of existing signal markers
        if (data.events && data.events.length > 0 && seriesRef.current) {
          const wyckoffMarkers = data.events
            .filter((e) => e.type === 'spring' || e.type === 'upthrust')
            .map((e) => ({
              time: e.time as UTCTimestamp,
              position: e.type === 'spring' ? ('belowBar' as const) : ('aboveBar' as const),
              color: e.type === 'spring' ? '#8FCB9B' : '#B47B4A',
              shape: e.type === 'spring' ? ('circle' as const) : ('circle' as const),
              text: e.type === 'spring' ? 'Sp' : 'Ut',
            }))
          // Append to existing markers by fetching current set is not possible via API,
          // so we merge with signals-derived markers
          const signalMarkers = signals
            .filter((s) => s.direction !== 'NEUTRAL')
            .map((s) => ({
              time: s.time as UTCTimestamp,
              position: s.direction === 'LONG' ? ('belowBar' as const) : ('aboveBar' as const),
              color: s.direction === 'LONG' ? '#8FCB9B' : 'rgba(143,128,115,0.9)',
              shape: s.direction === 'LONG' ? ('arrowUp' as const) : ('arrowDown' as const),
              text: abbreviateRule(s.rule),
            }))
          const allMarkers = [...signalMarkers, ...wyckoffMarkers].sort((a, b) =>
            (a.time as number) - (b.time as number)
          )
          createSeriesMarkers(seriesRef.current, allMarkers)
        }
      })
      .catch(() => {/* silently ignore wyckoff fetch errors */})
  }, [wyckoffEnabled, symbol, tf, signals])

  return (
    <>
      <div className="chart-controls">
        <select
          className="chart-select"
          value={symbol}
          onChange={(e) => setSymbol(e.target.value)}
        >
          {symbols.length === 0 && <option value="">{t('no_symbols_chart')}</option>}
          {symbols.map((s) => (
            <option key={s} value={s}>
              {s}
            </option>
          ))}
        </select>
        <div className="tf-group">
          {TFS.map((t) => (
            <button
              key={t}
              className={`tf-btn${tf === t ? ' active' : ''}`}
              onClick={() => setTf(t)}
            >
              {t}
            </button>
          ))}
        </div>
        <button
          className={`tf-btn${wyckoffEnabled ? ' active' : ''}`}
          onClick={() => setWyckoffEnabled((v) => !v)}
          title="Toggle Wyckoff overlay"
          style={{ marginLeft: 8, fontSize: '0.78rem', letterSpacing: '0.02em' }}
        >
          Wyckoff
        </button>
      </div>
      {wyckoffEnabled && wyckoffData && (
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 6, flexWrap: 'wrap' }}>
          <span
            style={{
              padding: '2px 10px',
              borderRadius: 4,
              fontSize: '0.78rem',
              fontWeight: 600,
              background: `${PHASE_COLORS[wyckoffData.phase] ?? '#5A5A5A'}22`,
              color: PHASE_COLORS[wyckoffData.phase] ?? '#8F8073',
              border: `1px solid ${PHASE_COLORS[wyckoffData.phase] ?? '#5A5A5A'}55`,
            }}
          >
            {PHASE_LABELS[wyckoffData.phase] ?? wyckoffData.phase}
          </span>
          {wyckoffData.events.filter((e) => e.type === 'spring' || e.type === 'upthrust').slice(-3).map((e, i) => (
            <span
              key={i}
              style={{
                padding: '2px 8px',
                borderRadius: 4,
                fontSize: '0.72rem',
                background: e.type === 'spring' ? 'rgba(143,203,155,0.1)' : 'rgba(180,123,74,0.1)',
                color: e.type === 'spring' ? '#8FCB9B' : '#B47B4A',
                border: `1px solid ${e.type === 'spring' ? '#8FCB9B' : '#B47B4A'}55`,
              }}
            >
              {e.type === 'spring' ? '↑ Spring' : '↓ Upthrust'}
            </span>
          ))}
        </div>
      )}
      {loading && <p className="loading">{t('chart_loading')}</p>}
      {error && <p className="error-msg">{t('no_data_error', { error })}</p>}
      <div ref={containerRef} className="chart-area" />
      {signals.some((s) => s.ai_interpretation) && (
        <>
          <p className="section-title" style={{ marginTop: 20 }}>{t('ai_interpretation')}</p>
          <div className="ai-panel">
            {signals
              .filter((s) => s.ai_interpretation)
              .map((s, i) => (
                <div key={i} className="ai-signal-item">
                  <div className="ai-signal-header">
                    <span className={s.direction === 'LONG' ? 'dir-long' : 'dir-short'}>
                      {s.direction}
                    </span>
                    <span className="ai-rule-badge">{abbreviateRule(s.rule)}</span>
                    <span className="ai-time">{new Date(s.time * 1000).toLocaleString()}</span>
                  </div>
                  <p className="ai-text">{s.ai_interpretation}</p>
                </div>
              ))}
          </div>
        </>
      )}
    </>
  )
}

// ── Backtest Tab ──────────────────────────────────────────────────────────────

interface BacktestStats {
  win_rate: number
  avg_rr: number
  profit_factor: number
  max_drawdown: number
  sharpe: number
  total_return_pct: number
  max_consec_losses: number
}

interface TradeOutcome {
  entry_time: string
  entry_price: number
  direction: string
  rule: string
  score: number
  tp: number
  sl: number
  exit_price: number
  exit_bars: number
  win: boolean
  pnl_pct: number
}

interface BacktestResult {
  symbol: string
  timeframe: string
  bars: number
  trades: number
  stats: BacktestStats
  outcomes: TradeOutcome[]
}

interface RuleStats {
  rule: string
  trades: number
  win_rate: number
  avg_rr: number
  profit_factor: number
  max_drawdown: number
  total_return_pct: number
}

function fmt2(n: number) { return n.toFixed(2) }
function fmtPct(n: number) { return (n >= 0 ? '+' : '') + n.toFixed(2) + '%' }

function BacktestTab() {
  const { t } = useTranslation()
  const [symbols, setSymbols] = useState<string[]>([])
  const [rules, setRules] = useState<string[]>([])
  const [symbol, setSymbol] = useState('')
  const [tf, setTf] = useState<TF>('1H')
  const [ruleFilter, setRuleFilter] = useState('')
  const [tpMult, setTpMult] = useState(2.0)
  const [slMult, setSlMult] = useState(1.0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [result, setResult] = useState<BacktestResult | null>(null)
  const [ruleStats, setRuleStats] = useState<RuleStats[] | null>(null)
  const [rulesLoading, setRulesLoading] = useState(false)
  const [selectedTrade, setSelectedTrade] = useState<number | null>(null)
  const btContainerRef = useRef<HTMLDivElement>(null)
  const btChartRef = useRef<IChartApi | null>(null)
  const btSeriesRef = useRef<ISeriesApi<'Candlestick'> | null>(null)

  useEffect(() => {
    apiFetch<SymbolItem[]>('/symbols').then((items) => {
      const enabled = items.filter((i) => i.enabled).map((i) => i.symbol)
      setSymbols(enabled)
      if (enabled.length > 0) setSymbol(enabled[0])
    }).catch(() => {/* silently ignore */})

    apiFetch<RuleItem[]>('/rules').then((items) => {
      setRules(items.filter((r) => r.enabled).map((r) => r.name))
    }).catch(() => {/* silently ignore */})
  }, [])

  const run = useCallback(async () => {
    if (!symbol) return
    setLoading(true)
    setError('')
    setResult(null)
    try {
      const r = await apiFetch<BacktestResult>('/backtest', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ symbol, timeframe: tf, rule: ruleFilter, tp_mult: tpMult, sl_mult: slMult }),
      })
      setResult(r)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : t('unknown_error'))
    } finally {
      setLoading(false)
    }
  }, [symbol, tf, ruleFilter, tpMult, slMult, t])

  const runPerRule = useCallback(async () => {
    if (!symbol) return
    setRulesLoading(true)
    setRuleStats(null)
    try {
      const stats = await apiFetch<RuleStats[]>(
        `/backtest/rules?symbol=${encodeURIComponent(symbol)}&timeframe=${tf}&tp_mult=${tpMult}&sl_mult=${slMult}`
      )
      setRuleStats(stats)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : t('unknown_error'))
    } finally {
      setRulesLoading(false)
    }
  }, [symbol, tf, tpMult, slMult, t])

  // Create backtest chart once the result container is rendered
  useEffect(() => {
    if (!result || !btContainerRef.current) return

    // Destroy previous chart instance if result changed
    if (btChartRef.current) {
      btChartRef.current.remove()
      btChartRef.current = null
      btSeriesRef.current = null
    }

    const chart = createChart(btContainerRef.current, {
      layout: {
        background: { color: '#0E0F0C' },
        textColor: '#8F8073',
      },
      grid: {
        vertLines: { color: 'rgba(234,230,229,0.04)' },
        horzLines: { color: 'rgba(234,230,229,0.04)' },
      },
      crosshair: { mode: CrosshairMode.Normal },
      width: btContainerRef.current.clientWidth,
      height: 320,
      timeScale: { borderColor: 'rgba(91,146,121,0.2)' },
      rightPriceScale: { borderColor: 'rgba(91,146,121,0.2)' },
    })

    const series = chart.addSeries(CandlestickSeries, {
      upColor: '#8FCB9B',
      downColor: 'rgba(143,128,115,0.6)',
      borderUpColor: '#8FCB9B',
      borderDownColor: 'rgba(143,128,115,0.6)',
      wickUpColor: '#8FCB9B',
      wickDownColor: 'rgba(143,128,115,0.6)',
    })
    series.priceScale().applyOptions({ scaleMargins: { top: 0.08, bottom: 0.08 } })

    btChartRef.current = chart
    btSeriesRef.current = series

    // Fetch OHLCV bars for the backtested symbol/tf
    apiFetch<OHLCVBar[]>(`/ohlcv/${encodeURIComponent(result.symbol)}/${result.timeframe}?limit=500`)
      .then((bars) => {
        series.setData(
          bars.map((b) => ({
            time: b.time as UTCTimestamp,
            open: b.open,
            high: b.high,
            low: b.low,
            close: b.close,
          }))
        )

        // Build entry + exit markers from trade outcomes
        if (result.outcomes && result.outcomes.length > 0) {
          type MarkerItem = {
            time: UTCTimestamp
            position: 'belowBar' | 'aboveBar'
            color: string
            shape: 'arrowUp' | 'arrowDown' | 'circle'
            text: string
          }
          const markers: MarkerItem[] = []
          for (const o of result.outcomes) {
            const entryTimeSec = Math.floor(new Date(o.entry_time).getTime() / 1000) as UTCTimestamp
            // Entry marker
            markers.push({
              time: entryTimeSec,
              position: o.direction === 'LONG' ? 'belowBar' : 'aboveBar',
              color: o.direction === 'LONG' ? '#8FCB9B' : '#B47B4A',
              shape: o.direction === 'LONG' ? 'arrowUp' : 'arrowDown',
              text: o.win ? '✓' : '✗',
            })
            // Exit marker — approximate via exit_bars * tf seconds offset
            const tfSeconds: Record<string, number> = { '1H': 3600, '4H': 14400, '1D': 86400, '1W': 604800 }
            const exitTimeSec = (entryTimeSec + o.exit_bars * (tfSeconds[result.timeframe] ?? 3600)) as UTCTimestamp
            markers.push({
              time: exitTimeSec,
              position: o.direction === 'LONG' ? 'aboveBar' : 'belowBar',
              color: o.win ? 'rgba(143,203,155,0.6)' : 'rgba(143,128,115,0.6)',
              shape: 'circle',
              text: '',
            })
          }
          markers.sort((a, b) => (a.time as number) - (b.time as number))
          createSeriesMarkers(series, markers)
        }

        // Fit chart to show all bars initially
        chart.timeScale().fitContent()
      })
      .catch(() => {/* silently ignore */})

    const onResize = () => {
      if (btContainerRef.current) chart.applyOptions({ width: btContainerRef.current.clientWidth })
    }
    window.addEventListener('resize', onResize)
    return () => {
      window.removeEventListener('resize', onResize)
      chart.remove()
      btChartRef.current = null
      btSeriesRef.current = null
    }
  }, [result]) // re-create chart whenever result changes

  // Scroll chart to selected trade when row is clicked
  useEffect(() => {
    if (selectedTrade === null || !result || !btChartRef.current) return
    const o = result.outcomes[selectedTrade]
    if (!o) return
    const entryTimeSec = Math.floor(new Date(o.entry_time).getTime() / 1000) as UTCTimestamp
    btChartRef.current.timeScale().scrollToPosition(0, false)
    btChartRef.current.timeScale().scrollToRealTime()
    // Set visible range to ±20 bars around the entry
    const tfSeconds: Record<string, number> = { '1H': 3600, '4H': 14400, '1D': 86400, '1W': 604800 }
    const pad = 20 * (tfSeconds[tf] ?? 3600)
    btChartRef.current.timeScale().setVisibleRange({
      from: (entryTimeSec - pad) as UTCTimestamp,
      to: (entryTimeSec + pad) as UTCTimestamp,
    })
  }, [selectedTrade, result, tf])

  return (
    <>
      <p className="section-title">{t('backtest_settings')}</p>
      <div className="backtest-controls">
        <select
          className="chart-select"
          value={symbol}
          onChange={(e) => setSymbol(e.target.value)}
          disabled={loading}
        >
          {symbols.length === 0 && <option value="">{t('no_symbols_chart')}</option>}
          {symbols.map((s) => <option key={s} value={s}>{s}</option>)}
        </select>

        <div className="tf-group">
          {TFS.map((t) => (
            <button
              key={t}
              className={`tf-btn${tf === t ? ' active' : ''}`}
              onClick={() => setTf(t)}
              disabled={loading}
            >
              {t}
            </button>
          ))}
        </div>

        <select
          className="chart-select"
          value={ruleFilter}
          onChange={(e) => setRuleFilter(e.target.value)}
          disabled={loading}
        >
          <option value="">{t('all_rules')}</option>
          {rules.map((r) => <option key={r} value={r}>{r}</option>)}
        </select>

        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <span className="item-meta">TP×</span>
          <input
            className="symbol-input"
            type="number"
            step="0.5"
            min="0.5"
            max="10"
            value={tpMult}
            onChange={(e) => setTpMult(Number(e.target.value))}
            disabled={loading}
            style={{ width: 72 }}
          />
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <span className="item-meta">SL×</span>
          <input
            className="symbol-input"
            type="number"
            step="0.5"
            min="0.5"
            max="10"
            value={slMult}
            onChange={(e) => setSlMult(Number(e.target.value))}
            disabled={loading}
            style={{ width: 72 }}
          />
        </div>

        <button className="run-btn" onClick={run} disabled={loading || !symbol}>
          {loading ? t('calculating') : t('run')}
        </button>
        <button
          className="run-btn"
          onClick={runPerRule}
          disabled={rulesLoading || !symbol}
          style={{ background: 'rgba(91,146,121,0.12)', color: 'var(--green)', border: '1px solid rgba(91,146,121,0.4)' }}
        >
          {rulesLoading ? t('analyzing_rules') : t('per_rule_analysis')}
        </button>
      </div>

      {error && <p className="error-msg">{t('error')}: {error}</p>}

      {result && !loading && (
        <>
          <p className="section-title">
            {t('backtest_result_summary', { symbol: result.symbol, timeframe: result.timeframe, bars: result.bars, trades: result.trades, tp: tpMult, sl: slMult })}
          </p>

          <div className="backtest-stats">
            <div className="stat-card">
              <div className="stat-value" style={{ fontSize: '1.4rem' }}>
                {(result.stats.win_rate * 100).toFixed(1)}%
              </div>
              <div className="stat-label">{t('win_rate')}</div>
            </div>
            <div className="stat-card">
              <div className="stat-value" style={{ fontSize: '1.4rem' }}>
                {fmt2(result.stats.avg_rr)}
              </div>
              <div className="stat-label">{t('avg_pnl_ratio')}</div>
            </div>
            <div className="stat-card">
              <div className="stat-value" style={{ fontSize: '1.4rem' }}>
                {fmt2(result.stats.profit_factor)}
              </div>
              <div className="stat-label">{t('profit_factor')}</div>
            </div>
            <div className="stat-card">
              <div className="stat-value" style={{ fontSize: '1.4rem', color: 'var(--muted)' }}>
                {(result.stats.max_drawdown * 100).toFixed(1)}%
              </div>
              <div className="stat-label">{t('max_drawdown')}</div>
            </div>
            <div className="stat-card">
              <div className="stat-value" style={{ fontSize: '1.4rem' }}>
                {fmt2(result.stats.sharpe)}
              </div>
              <div className="stat-label">{t('sharpe_ratio')}</div>
            </div>
            <div className="stat-card">
              <div
                className="stat-value"
                style={{
                  fontSize: '1.4rem',
                  color: result.stats.total_return_pct >= 0 ? 'var(--mint)' : 'var(--muted)',
                }}
              >
                {fmtPct(result.stats.total_return_pct)}
              </div>
              <div className="stat-label">{t('total_return')}</div>
            </div>
          </div>

          {/* Trade chart */}
          <p className="section-title" style={{ marginTop: 24 }}>{t('trade_chart')}</p>
          <div
            ref={btContainerRef}
            style={{
              width: '100%',
              height: 320,
              borderRadius: 6,
              overflow: 'hidden',
              border: '1px solid rgba(91,146,121,0.15)',
              marginBottom: 8,
            }}
          />
          <p className="item-meta" style={{ marginBottom: 16 }}>{t('trade_chart_hint')}</p>

          {result.outcomes && result.outcomes.length > 0 && (
            <>
              <p className="section-title" style={{ marginTop: 8 }}>{t('trade_list')}</p>
              <div className="backtest-table-wrap">
                <table className="backtest-table">
                  <thead>
                    <tr>
                      <th>{t('entry_time')}</th>
                      <th>{t('direction')}</th>
                      <th>{t('rule')}</th>
                      <th>{t('entry_price')}</th>
                      <th>{t('exit_price')}</th>
                      <th>{t('bars')}</th>
                      <th>{t('return_pct')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {result.outcomes.map((o, i) => (
                      <tr
                        key={i}
                        className={o.win ? 'outcome-win' : 'outcome-loss'}
                        style={{ cursor: 'pointer', outline: selectedTrade === i ? '1px solid rgba(91,146,121,0.5)' : undefined }}
                        onClick={() => setSelectedTrade(i === selectedTrade ? null : i)}
                      >
                        <td>{new Date(o.entry_time).toLocaleDateString()}</td>
                        <td className={o.direction === 'LONG' ? 'dir-long' : 'dir-short'}>
                          {o.direction}
                        </td>
                        <td style={{ color: 'var(--muted)', maxWidth: 140, overflow: 'hidden', textOverflow: 'ellipsis' }}>
                          {o.rule}
                        </td>
                        <td>{o.entry_price.toFixed(2)}</td>
                        <td>{o.exit_price.toFixed(2)}</td>
                        <td>{o.exit_bars}</td>
                        <td className={o.win ? 'pnl-win' : 'pnl-loss'}>
                          {fmtPct(o.pnl_pct)}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}

          {result.trades === 0 && (
            <p className="loading">{t('no_backtest_data')}</p>
          )}
        </>
      )}

      {ruleStats !== null && (
        <>
          <p className="section-title" style={{ marginTop: 24 }}>
            {t('per_rule_result_summary', { symbol, timeframe: tf, tp: tpMult, sl: slMult })}
          </p>
          {ruleStats.length === 0 ? (
            <p className="loading">{t('no_trade_data')}</p>
          ) : (
            <>
              <div className="backtest-table-wrap">
                <table className="backtest-table">
                  <thead>
                    <tr>
                      <th>{t('rule_stats_header_rule')}</th><th>{t('rule_stats_header_trades')}</th><th>{t('win_rate')}</th><th>{t('avg_rr')}</th><th>{t('profit_factor')}</th><th>{t('total_return')}</th><th>{t('rule_stats_header_export')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {ruleStats.map((s) => (
                      <tr key={s.rule}>
                        <td style={{ color: 'var(--muted)', maxWidth: 160, overflow: 'hidden', textOverflow: 'ellipsis' }}>
                          {s.rule}
                        </td>
                        <td>{s.trades}</td>
                        <td style={{ color: s.win_rate >= 0.45 ? 'var(--mint)' : 'var(--muted)', fontWeight: 600 }}>
                          {(s.win_rate * 100).toFixed(1)}%
                        </td>
                        <td>{s.avg_rr.toFixed(2)}</td>
                        <td>{s.profit_factor.toFixed(2)}</td>
                        <td className={s.total_return_pct >= 0 ? 'pnl-win' : 'pnl-loss'}>
                          {s.total_return_pct >= 0 ? '+' : ''}{s.total_return_pct.toFixed(2)}%
                        </td>
                        <td>
                          <button
                            className="run-btn"
                            style={{ padding: '2px 10px', fontSize: '0.75rem', background: 'rgba(91,146,121,0.12)', color: 'var(--green)', border: '1px solid rgba(91,146,121,0.4)' }}
                            onClick={() => {
                              const url = `/api/export/pinescript?rule=${encodeURIComponent(s.rule)}&win_rate=${(s.win_rate * 100).toFixed(1)}&avg_rr=${s.avg_rr.toFixed(2)}`
                              fetch(url)
                                .then((res) => res.blob())
                                .then((blob) => {
                                  const a = document.createElement('a')
                                  a.href = URL.createObjectURL(blob)
                                  a.download = `${s.rule}.pine`
                                  a.click()
                                  URL.revokeObjectURL(a.href)
                                })
                            }}
                          >
                            .pine
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <p className="item-meta" style={{ marginTop: 8 }}>
                {t('backtest_hint')}
              </p>
            </>
          )}
        </>
      )}
    </>
  )
}

// ── Paper Trading Tab ─────────────────────────────────────────────────────────

interface PaperPosition {
  id: number
  symbol: string
  timeframe: string
  rule: string
  direction: string
  entry_price: number
  tp: number
  sl: number
  entry_time: string
  exit_price: number
  exit_time: string | null
  status: string
  pnl_pct: number
}

interface PaperSummary {
  open_positions: number
  closed_trades: number
  wins: number
  losses: number
  win_rate: number
  total_pnl_pct: number
  avg_win_pct: number
  avg_loss_pct: number
}

function PaperTab() {
  const { t } = useTranslation()
  const [summary, setSummary] = useState<PaperSummary | null>(null)
  const [positions, setPositions] = useState<PaperPosition[]>([])
  const [history, setHistory] = useState<PaperPosition[]>([])
  const [loading, setLoading] = useState(true)

  const reload = useCallback(() => {
    setLoading(true)
    Promise.all([
      apiFetch<PaperSummary>('/paper/summary'),
      apiFetch<PaperPosition[]>('/paper/positions'),
      apiFetch<PaperPosition[]>('/paper/history?limit=50'),
    ])
      .then(([s, p, h]) => {
        setSummary(s)
        setPositions(p ?? [])
        setHistory(h ?? [])
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { reload() }, [reload])

  if (loading) return <p className="loading">{t('loading')}</p>

  return (
    <>
      <p className="section-title">{t('paper_title')}</p>

      {summary && (
        <div className="backtest-stats">
          <div className="stat-card">
            <div className="stat-value" style={{ fontSize: '1.4rem' }}>{summary.open_positions}</div>
            <div className="stat-label">{t('open_positions')}</div>
          </div>
          <div className="stat-card">
            <div className="stat-value" style={{ fontSize: '1.4rem' }}>{summary.closed_trades}</div>
            <div className="stat-label">{t('total_trades_label')}</div>
          </div>
          <div className="stat-card">
            <div className="stat-value" style={{ fontSize: '1.4rem' }}>{(summary.win_rate * 100).toFixed(1)}%</div>
            <div className="stat-label">{t('win_rate')}</div>
          </div>
          <div className="stat-card">
            <div
              className="stat-value"
              style={{ fontSize: '1.4rem', color: summary.total_pnl_pct >= 0 ? 'var(--mint)' : 'var(--muted)' }}
            >
              {summary.total_pnl_pct >= 0 ? '+' : ''}{summary.total_pnl_pct.toFixed(2)}%
            </div>
            <div className="stat-label">{t('cumulative_pnl')}</div>
          </div>
          <div className="stat-card">
            <div className="stat-value" style={{ fontSize: '1.4rem', color: 'var(--mint)' }}>
              +{summary.avg_win_pct.toFixed(2)}%
            </div>
            <div className="stat-label">{t('avg_win')}</div>
          </div>
          <div className="stat-card">
            <div className="stat-value" style={{ fontSize: '1.4rem', color: 'var(--muted)' }}>
              {summary.avg_loss_pct.toFixed(2)}%
            </div>
            <div className="stat-label">{t('avg_loss')}</div>
          </div>
        </div>
      )}

      {positions.length > 0 && (
        <>
          <p className="section-title" style={{ marginTop: 24 }}>{t('open_positions_count', { count: positions.length })}</p>
          <div className="backtest-table-wrap">
            <table className="backtest-table">
              <thead>
                <tr><th>{t('symbol')}</th><th>{t('direction')}</th><th>{t('rule')}</th><th>{t('entry_price')}</th><th>{t('tp')}</th><th>{t('sl')}</th><th>{t('entry_time')}</th></tr>
              </thead>
              <tbody>
                {positions.map((p) => (
                  <tr key={p.id}>
                    <td>{p.symbol}</td>
                    <td className={p.direction === 'LONG' ? 'dir-long' : 'dir-short'}>{p.direction}</td>
                    <td style={{ color: 'var(--muted)' }}>{p.rule}</td>
                    <td>{p.entry_price.toFixed(2)}</td>
                    <td className="pnl-win">{p.tp.toFixed(2)}</td>
                    <td className="pnl-loss">{p.sl.toFixed(2)}</td>
                    <td style={{ color: 'var(--muted)', fontSize: '0.72rem' }}>
                      {new Date(p.entry_time).toLocaleString()}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}

      {positions.length === 0 && (
        <p className="loading" style={{ marginTop: 16 }}>{t('no_open_positions')}</p>
      )}

      {history.length > 0 && (
        <>
          <p className="section-title" style={{ marginTop: 24 }}>{t('close_history')}</p>
          <div className="backtest-table-wrap">
            <table className="backtest-table">
              <thead>
                <tr><th>{t('symbol')}</th><th>{t('direction')}</th><th>{t('entry_price')}</th><th>{t('exit_price')}</th><th>{t('result')}</th><th>{t('pnl')}</th></tr>
              </thead>
              <tbody>
                {history.map((p) => (
                  <tr key={p.id} className={p.pnl_pct > 0 ? 'outcome-win' : 'outcome-loss'}>
                    <td>{p.symbol}</td>
                    <td className={p.direction === 'LONG' ? 'dir-long' : 'dir-short'}>{p.direction}</td>
                    <td>{p.entry_price.toFixed(2)}</td>
                    <td>{p.exit_price.toFixed(2)}</td>
                    <td style={{ fontSize: '0.72rem', color: p.status === 'CLOSED_TP' ? 'var(--mint)' : 'var(--muted)' }}>
                      {p.status === 'CLOSED_TP' ? 'TP ✓' : 'SL ✗'}
                    </td>
                    <td className={p.pnl_pct > 0 ? 'pnl-win' : 'pnl-loss'}>
                      {p.pnl_pct >= 0 ? '+' : ''}{p.pnl_pct.toFixed(2)}%
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </>
  )
}

// ── Report Tab ────────────────────────────────────────────────────────────────

interface ReportConfig {
  enabled: boolean
  time: string
  timezone: string
  ai_min_score: number
  only_if_signals: boolean
  compact: boolean
}

function ReportTab() {
  const { t } = useTranslation()
  const [config, setConfig] = useState<ReportConfig | null>(null)
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    apiFetch<ReportConfig>('/report/config')
      .then(setConfig)
      .catch((e: Error) => setError(e.message))
  }, [])

  const save = useCallback(async () => {
    if (!config) return
    setSaving(true)
    setError('')
    try {
      await putJSON('/report/config', config)
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : t('save_failed'))
    } finally {
      setSaving(false)
    }
  }, [config, t])

  if (!config && !error) return <p className="loading">{t('loading')}</p>
  if (error && !config) return <p className="error-msg">{t('error')}: {error}</p>
  if (!config) return null

  return (
    <>
      <p className="section-title">{t('daily_report_config')}</p>

      <div className="report-field">
        <span className="field-label">{t('report_enabled')}</span>
        <Toggle checked={config.enabled} onChange={(v) => setConfig({ ...config, enabled: v })} />
      </div>

      <div className="report-field">
        <span className="field-label">{t('send_time_kst')}</span>
        <input
          className="report-input"
          placeholder="09:00"
          value={config.time}
          onChange={(e) => setConfig({ ...config, time: e.target.value })}
        />
      </div>

      <div className="report-field">
        <span className="field-label">{t('time_zone')}</span>
        <input
          className="report-input"
          value={config.timezone}
          readOnly
          style={{ opacity: 0.6, cursor: 'default' }}
        />
      </div>

      <div className="report-field">
        <span className="field-label">{t('ai_min_score')}</span>
        <input
          className="report-input"
          type="number"
          step={0.5}
          min={0}
          value={config.ai_min_score}
          onChange={(e) => setConfig({ ...config, ai_min_score: parseFloat(e.target.value) || 0 })}
        />
      </div>

      <div className="report-field">
        <span className="field-label">{t('skip_no_signal_days')}</span>
        <Toggle checked={config.only_if_signals} onChange={(v) => setConfig({ ...config, only_if_signals: v })} />
      </div>

      <div className="report-field">
        <span className="field-label">{t('compact_mode_label')}</span>
        <Toggle checked={config.compact} onChange={(v) => setConfig({ ...config, compact: v })} />
      </div>

      <div style={{ marginTop: 24, display: 'flex', alignItems: 'center', gap: 12 }}>
        <button className="run-btn" onClick={save} disabled={saving}>
          {saving ? t('saving') : t('save')}
        </button>
        {saved && <span className="save-success">{t('saved')}</span>}
        {error && <span className="error-msg" style={{ padding: '4px 8px' }}>{error}</span>}
      </div>
    </>
  )
}

// ── Alert Tab ─────────────────────────────────────────────────────────────────

interface AlertConfig {
  score_threshold: number
  cooldown_hours: number
  mtf_consensus_min: number
  crypto_tp_mult: number
  crypto_sl_mult: number
  stock_tp_mult: number
  stock_sl_mult: number
}

function AlertTab() {
  const { t } = useTranslation()
  const [config, setConfig] = useState<AlertConfig | null>(null)
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    apiFetch<AlertConfig>('/alert/config')
      .then(setConfig)
      .catch((e: Error) => setError(e.message))
  }, [])

  const save = useCallback(async () => {
    if (!config) return
    setSaving(true)
    setError('')
    try {
      await putJSON('/alert/config', config)
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : t('save_failed'))
    } finally {
      setSaving(false)
    }
  }, [config, t])

  if (!config && !error) return <p className="loading">{t('loading')}</p>
  if (error && !config) return <p className="error-msg">{t('error')}: {error}</p>
  if (!config) return null

  return (
    <>
      <p className="section-title">{t('alert_settings')}</p>

      <div className="report-field">
        <span className="field-label">{t('signal_score_threshold')}</span>
        <input
          className="report-input"
          type="number"
          step={0.5}
          min={5}
          value={config.score_threshold}
          onChange={(e) => setConfig({ ...config, score_threshold: parseFloat(e.target.value) || 0 })}
        />
      </div>
      <p className="item-meta" style={{ marginBottom: 12 }}>
        {t('score_threshold_hint')}
      </p>

      <div className="report-field">
        <span className="field-label">{t('cooldown_label')}</span>
        <input
          className="report-input"
          type="number"
          step={1}
          min={1}
          value={config.cooldown_hours}
          onChange={(e) => setConfig({ ...config, cooldown_hours: parseInt(e.target.value) || 1 })}
        />
      </div>
      <p className="item-meta" style={{ marginBottom: 12 }}>
        {t('cooldown_hint')}
      </p>

      <div className="report-field">
        <span className="field-label">{t('mtf_consensus_filter')}</span>
        <select
          className="report-input"
          value={config.mtf_consensus_min}
          onChange={(e) => setConfig({ ...config, mtf_consensus_min: parseInt(e.target.value) })}
        >
          <option value={1}>{t('mtf_option_1')}</option>
          <option value={2}>{t('mtf_option_2')}</option>
          <option value={3}>{t('mtf_option_3')}</option>
          <option value={4}>{t('mtf_option_4')}</option>
        </select>
      </div>
      <p className="item-meta" style={{ marginBottom: 12 }}>
        {t('mtf_hint')}
      </p>

      <p className="section-title" style={{ marginTop: 20 }}>{t('tp_sl_settings')}</p>
      <p className="item-meta">{t('tp_sl_hint')}</p>

      <div className="report-field">
        <span className="field-label">{t('crypto_tp_mult')}</span>
        <input
          className="report-input"
          type="number"
          step="0.25"
          min="0.25"
          value={config.crypto_tp_mult}
          onChange={(e) => setConfig({ ...config, crypto_tp_mult: parseFloat(e.target.value) || 0 })}
        />
      </div>

      <div className="report-field">
        <span className="field-label">{t('crypto_sl_mult')}</span>
        <input
          className="report-input"
          type="number"
          step="0.25"
          min="0.25"
          value={config.crypto_sl_mult}
          onChange={(e) => setConfig({ ...config, crypto_sl_mult: parseFloat(e.target.value) || 0 })}
        />
      </div>

      <div className="report-field">
        <span className="field-label">{t('stock_tp_mult')}</span>
        <input
          className="report-input"
          type="number"
          step="0.25"
          min="0.25"
          value={config.stock_tp_mult}
          onChange={(e) => setConfig({ ...config, stock_tp_mult: parseFloat(e.target.value) || 0 })}
        />
      </div>

      <div className="report-field">
        <span className="field-label">{t('stock_sl_mult')}</span>
        <input
          className="report-input"
          type="number"
          step="0.25"
          min="0.25"
          value={config.stock_sl_mult}
          onChange={(e) => setConfig({ ...config, stock_sl_mult: parseFloat(e.target.value) || 0 })}
        />
      </div>

      <div style={{ marginTop: 24, display: 'flex', alignItems: 'center', gap: 12 }}>
        <button className="run-btn" onClick={save} disabled={saving}>
          {saving ? t('saving') : t('save')}
        </button>
        {saved && <span className="save-success">{t('saved')}</span>}
        {error && <span className="error-msg" style={{ padding: '4px 8px' }}>{error}</span>}
      </div>
    </>
  )
}

// ── History Tab ───────────────────────────────────────────────────────────────

function HistoryTab() {
  const { t } = useTranslation()
  const [symbols, setSymbols] = useState<string[]>([])
  const [symbol, setSymbol] = useState('ALL')
  const [direction, setDirection] = useState('ALL')
  const [limit, setLimit] = useState(100)
  const [signals, setSignals] = useState<SignalBar[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    apiFetch<SymbolItem[]>('/symbols').then((items) => {
      setSymbols(items.filter((i) => i.enabled).map((i) => i.symbol))
    }).catch(() => {})
  }, [])

  const load = useCallback(() => {
    setLoading(true)
    setError('')
    apiFetch<SignalBar[]>(`/history?symbol=${encodeURIComponent(symbol)}&direction=${direction}&limit=${limit}`)
      .then(setSignals)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [symbol, direction, limit])

  useEffect(() => { load() }, [load])

  return (
    <>
      <div className="backtest-controls">
        <select className="chart-select" value={symbol} onChange={(e) => setSymbol(e.target.value)}>
          <option value="ALL">{t('all_symbols')}</option>
          {symbols.map((s) => <option key={s} value={s}>{s}</option>)}
        </select>
        <select className="chart-select" value={direction} onChange={(e) => setDirection(e.target.value)}>
          <option value="ALL">{t('all_directions')}</option>
          <option value="LONG">LONG</option>
          <option value="SHORT">SHORT</option>
        </select>
        <select className="chart-select" value={limit} onChange={(e) => setLimit(Number(e.target.value))}>
          <option value={50}>{t('items_50')}</option>
          <option value={100}>{t('items_100')}</option>
          <option value={200}>{t('items_200')}</option>
        </select>
      </div>
      {loading && <p className="loading">{t('loading')}</p>}
      {error && <p className="error-msg">{t('error')}: {error}</p>}
      {!loading && signals.length === 0 && (
        <p className="loading">{t('no_signals_hint')}</p>
      )}
      {signals.length > 0 && (
        <div className="backtest-table-wrap">
          <table className="backtest-table">
            <thead>
              <tr>
                <th>{t('time_col')}</th>
                <th>{t('symbol_col')}</th>
                <th>TF</th>
                <th>{t('direction')}</th>
                <th>{t('rule')}</th>
                <th>{t('score')}</th>
                <th>{t('ai_interpretation')}</th>
              </tr>
            </thead>
            <tbody>
              {signals.map((s, i) => (
                <tr key={i}>
                  <td style={{ color: 'var(--muted)', fontSize: '0.72rem', whiteSpace: 'nowrap' }}>
                    {new Date(s.time * 1000).toLocaleString()}
                  </td>
                  <td style={{ fontWeight: 600 }}>{s.symbol}</td>
                  <td style={{ color: 'var(--muted)' }}>{s.timeframe}</td>
                  <td className={s.direction === 'LONG' ? 'dir-long' : 'dir-short'}>{s.direction}</td>
                  <td style={{ color: 'var(--muted)', maxWidth: 120, overflow: 'hidden', textOverflow: 'ellipsis' }}>
                    {abbreviateRule(s.rule)}
                  </td>
                  <td>{s.score.toFixed(1)}</td>
                  <td style={{ color: 'var(--muted)', maxWidth: 200, overflow: 'hidden', textOverflow: 'ellipsis', fontSize: '0.75rem' }}
                      title={s.ai_interpretation}>
                    {s.ai_interpretation ? s.ai_interpretation.slice(0, 80) + (s.ai_interpretation.length > 80 ? '…' : '') : '─'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  )
}

// ── performance tab ───────────────────────────────────────────────────────────

interface AggregatedRuleStat {
  rule: string
  symbols_tested: number
  total_trades: number
  avg_win_rate: number
  avg_rr: number
  avg_profit_factor: number
  exportable: boolean
}

function PerformanceTab() {
  const { t } = useTranslation()
  const [symbols, setSymbols] = useState<string[]>([])
  const [selected, setSelected] = useState<string[]>([])
  const [tf, setTf] = useState<TF>('1H')
  const [tpMult, setTpMult] = useState(2.0)
  const [slMult, setSlMult] = useState(1.0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [stats, setStats] = useState<AggregatedRuleStat[] | null>(null)
  const [exporting, setExporting] = useState<string | null>(null)

  useEffect(() => {
    apiFetch<SymbolItem[]>('/symbols').then((items) => {
      const enabled = items.filter((i) => i.enabled).map((i) => i.symbol)
      setSymbols(enabled)
      setSelected(enabled)
    }).catch(() => {})
  }, [])

  const toggleSymbol = (sym: string) => {
    setSelected((prev) =>
      prev.includes(sym) ? prev.filter((s) => s !== sym) : [...prev, sym]
    )
  }

  const run = useCallback(async () => {
    if (selected.length === 0) return
    setLoading(true)
    setError('')
    setStats(null)
    try {
      const result = await apiFetch<AggregatedRuleStat[]>(
        `/performance/rules?symbols=${encodeURIComponent(selected.join(','))}&timeframe=${tf}&tp_mult=${tpMult}&sl_mult=${slMult}`
      )
      setStats(result)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : t('unknown_error'))
    } finally {
      setLoading(false)
    }
  }, [selected, tf, tpMult, slMult, t])

  const exportRule = async (rule: string, winRate: number, avgRR: number) => {
    setExporting(rule)
    try {
      const res = await fetch(`/api/export/pinescript?rule=${encodeURIComponent(rule)}&win_rate=${(winRate * 100).toFixed(1)}&avg_rr=${avgRR.toFixed(2)}`)
      const blob = await res.blob()
      const a = document.createElement('a')
      a.href = URL.createObjectURL(blob)
      a.download = `${rule}.pine`
      a.click()
      URL.revokeObjectURL(a.href)
    } finally {
      setExporting(null)
    }
  }

  return (
    <>
      <p className="section-title">{t('symbol_select')}</p>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginBottom: 16 }}>
        {symbols.map((sym) => (
          <button
            key={sym}
            onClick={() => toggleSymbol(sym)}
            className="tf-btn"
            style={{
              background: selected.includes(sym) ? 'rgba(91,146,121,0.18)' : 'transparent',
              color: selected.includes(sym) ? 'var(--mint)' : 'var(--muted)',
              border: selected.includes(sym) ? '1px solid rgba(91,146,121,0.5)' : '1px solid rgba(143,128,115,0.3)',
            }}
          >
            {sym}
          </button>
        ))}
        <button
          className="tf-btn"
          style={{ color: 'var(--muted)', fontSize: '0.75rem' }}
          onClick={() => setSelected(symbols.length === selected.length ? [] : [...symbols])}
        >
          {symbols.length === selected.length ? t('deselect_all') : t('select_all')}
        </button>
      </div>

      <div className="backtest-controls">
        <div className="tf-group">
          {TFS.map((t) => (
            <button
              key={t}
              className={`tf-btn${tf === t ? ' active' : ''}`}
              onClick={() => setTf(t)}
              disabled={loading}
            >
              {t}
            </button>
          ))}
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <span className="item-meta">TP×</span>
          <input className="symbol-input" type="number" step="0.5" min="0.5" max="10"
            value={tpMult} onChange={(e) => setTpMult(Number(e.target.value))}
            disabled={loading} style={{ width: 64 }} />
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <span className="item-meta">SL×</span>
          <input className="symbol-input" type="number" step="0.5" min="0.5" max="10"
            value={slMult} onChange={(e) => setSlMult(Number(e.target.value))}
            disabled={loading} style={{ width: 64 }} />
        </div>
        <button className="run-btn" onClick={run} disabled={loading || selected.length === 0}>
          {loading ? t('analyzing_rules') : t('analyze_n_symbols', { n: selected.length })}
        </button>
      </div>

      {error && <p className="error-msg">{t('error')}: {error}</p>}

      {stats !== null && !loading && (
        <>
          <p className="section-title" style={{ marginTop: 24 }}>
            {t('rule_ranking', { tf, tp: tpMult, sl: slMult, n: selected.length })}
          </p>
          {stats.length === 0 ? (
            <p className="loading">{t('no_performance_data')}</p>
          ) : (
            <>
              <div className="backtest-table-wrap">
                <table className="backtest-table">
                  <thead>
                    <tr>
                      <th>{t('rank')}</th>
                      <th>{t('rule')}</th>
                      <th>{t('avg_win_rate')}</th>
                      <th>{t('avg_rr')}</th>
                      <th>{t('avg_profit_factor')}</th>
                      <th>{t('total_trades')}</th>
                      <th>{t('symbol_count')}</th>
                      <th style={{ minWidth: 140 }}>{t('tv_export')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {stats.map((s, idx) => (
                      <tr key={s.rule}>
                        <td style={{ color: idx < 3 ? 'var(--mint)' : 'var(--muted)', fontWeight: idx < 3 ? 700 : 400 }}>
                          {idx + 1}
                        </td>
                        <td style={{ color: 'var(--text)' }}>{s.rule}</td>
                        <td style={{ color: s.avg_win_rate >= 0.45 ? 'var(--mint)' : 'var(--muted)', fontWeight: 600 }}>
                          {(s.avg_win_rate * 100).toFixed(1)}%
                        </td>
                        <td>{s.avg_rr.toFixed(2)}</td>
                        <td>{s.avg_profit_factor.toFixed(2)}</td>
                        <td>{s.total_trades}</td>
                        <td style={{ color: 'var(--muted)' }}>{s.symbols_tested}</td>
                        <td>
                          {s.exportable ? (
                            <button
                              className="run-btn"
                              disabled={exporting === s.rule}
                              onClick={() => exportRule(s.rule, s.avg_win_rate, s.avg_rr)}
                              style={{
                                padding: '4px 14px',
                                fontSize: '0.8rem',
                                background: s.avg_win_rate >= 0.45
                                  ? 'rgba(91,146,121,0.18)'
                                  : 'rgba(143,128,115,0.12)',
                                color: s.avg_win_rate >= 0.45 ? 'var(--mint)' : 'var(--muted)',
                                border: s.avg_win_rate >= 0.45
                                  ? '1px solid rgba(91,146,121,0.5)'
                                  : '1px solid rgba(143,128,115,0.3)',
                              }}
                            >
                              {exporting === s.rule ? '...' : t('export_to_tv')}
                            </button>
                          ) : (
                            <span style={{ color: 'var(--muted)', fontSize: '0.75rem' }}>{t('not_supported')}</span>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <p className="item-meta" style={{ marginTop: 8 }}>
                {t('performance_hint')}
              </p>
            </>
          )}
        </>
      )}
    </>
  )
}

// ── SettingsTab ───────────────────────────────────────────────────────────────

const ENV_SENTINEL = '__configured__'
type EnvMap = Record<string, string>

interface EnvField {
  key: string
  label: string
  type: 'text' | 'password' | 'select'
  options?: string[]
}
interface EnvGroup { label: string; fields: EnvField[] }

const ENV_GROUPS: EnvGroup[] = [
  {
    label: 'Server',
    fields: [
      { key: 'ENV',                  label: 'Environment',            type: 'select', options: ['development', 'production'] },
      { key: 'SERVER_PORT',          label: 'Server Port',            type: 'text' },
      { key: 'LOG_LEVEL',            label: 'Log Level',              type: 'select', options: ['debug', 'info', 'warn', 'error'] },
      { key: 'ALERT_COOLDOWN_HOURS', label: 'Alert Cooldown (hours)', type: 'text' },
    ],
  },
  {
    label: 'Notifications',
    fields: [
      { key: 'TELEGRAM_BOT_TOKEN',  label: 'Telegram Bot Token',   type: 'password' },
      { key: 'TELEGRAM_CHAT_ID',    label: 'Telegram Chat ID',     type: 'text' },
      { key: 'DISCORD_WEBHOOK_URL', label: 'Discord Webhook URL',  type: 'password' },
    ],
  },
  {
    label: 'Data Sources',
    fields: [
      { key: 'TIINGO_API_KEY',       label: 'Tiingo API Key',              type: 'password' },
      { key: 'TIINGO_POLL_INTERVAL', label: 'Tiingo Poll Interval (sec)',  type: 'text' },
      { key: 'YAHOO_POLL_INTERVAL',  label: 'Yahoo Poll Interval (sec)',   type: 'text' },
      { key: 'BINANCE_API_KEY',      label: 'Binance API Key',             type: 'password' },
      { key: 'BINANCE_SECRET_KEY',   label: 'Binance Secret Key',          type: 'password' },
      { key: 'ALPHAVANTAGE_API_KEY', label: 'AlphaVantage API Key',        type: 'password' },
    ],
  },
  {
    label: 'Economic Calendar (둘 중 하나만 설정)',
    fields: [
      { key: 'FMP_API_KEY',             label: 'FMP API Key (무료 — 권장)',           type: 'password' },
      { key: 'FINNHUB_API_KEY',         label: 'Finnhub API Key (유료 플랜 필요)',     type: 'password' },
      { key: 'CALENDAR_ALERT_WINDOW',   label: '사전 알림 (분, 기본 30)',              type: 'text' },
    ],
  },
  {
    label: 'AI / LLM',
    fields: [
      { key: 'LLM_PROVIDER',     label: 'LLM Provider',    type: 'select', options: ['', 'anthropic', 'openai', 'groq', 'gemini'] },
      { key: 'ANTHROPIC_API_KEY', label: 'Anthropic API Key', type: 'password' },
      { key: 'OPENAI_API_KEY',    label: 'OpenAI API Key',    type: 'password' },
      { key: 'GROQ_API_KEY',      label: 'Groq API Key',      type: 'password' },
      { key: 'GEMINI_API_KEY',    label: 'Gemini API Key',    type: 'password' },
      { key: 'AI_MIN_SCORE',      label: 'AI Min Score',      type: 'text' },
      { key: 'LLM_LANGUAGE',      label: 'LLM Language',      type: 'select', options: ['en', 'ko', 'ja'] },
    ],
  },
]

function SettingsTab() {
  const [env, setEnv] = useState<EnvMap>({})
  const [edits, setEdits] = useState<EnvMap>({})
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
  const [error, setError] = useState('')

  const loadEnv = useCallback(() => {
    setLoading(true)
    fetch('/api/env/config')
      .then(r => r.json())
      .then((data: EnvMap) => { setEnv(data); setEdits({}) })
      .catch(() => setError('Failed to load settings'))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { loadEnv() }, [loadEnv])

  const getValue = (key: string) => {
    if (key in edits) return edits[key]
    const v = env[key] ?? ''
    return v === ENV_SENTINEL ? '' : v
  }

  const getPlaceholder = (key: string, type: string) =>
    type === 'password' && env[key] === ENV_SENTINEL
      ? 'already configured — leave blank to keep'
      : ''

  const handleChange = (key: string, value: string) => {
    setEdits(prev => ({ ...prev, [key]: value }))
    setSaved(false)
  }

  const handleSave = async () => {
    setSaving(true)
    setError('')
    const payload: EnvMap = {}
    for (const group of ENV_GROUPS) {
      for (const field of group.fields) {
        const k = field.key
        if (k in edits) {
          const v = edits[k]
          // blank password field + was previously set → keep existing
          payload[k] = (v === '' && field.type === 'password' && env[k] === ENV_SENTINEL)
            ? ENV_SENTINEL
            : v
        } else {
          payload[k] = env[k] ?? ''
        }
      }
    }
    try {
      const res = await fetch('/api/env/config', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
      if (!res.ok) throw new Error(await res.text())
      setSaved(true)
      loadEnv()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Save failed')
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <div className="section"><p>Loading…</p></div>

  return (
    <div className="section">
      <h2>Environment Settings</h2>
      <p style={{ marginBottom: '1.5rem', fontSize: '0.85rem', color: 'var(--muted)' }}>
        Changes are written to <code>.env</code>. <strong>Restart the server</strong> to apply.
      </p>
      {error && (
        <div className="save-success" style={{ background: 'rgba(255,68,68,0.12)', color: '#ff6b6b', marginBottom: '1rem' }}>
          {error}
        </div>
      )}
      {saved && (
        <div className="save-success" style={{ marginBottom: '1rem' }}>
          Saved — restart the server to apply changes.
        </div>
      )}
      {ENV_GROUPS.map(group => (
        <div key={group.label} style={{ marginBottom: '2rem' }}>
          <h3 style={{
            fontSize: '0.78rem', textTransform: 'uppercase', letterSpacing: '0.08em',
            color: 'var(--accent)', marginBottom: '0.75rem',
            borderBottom: '1px solid rgba(91,146,121,0.2)', paddingBottom: '0.4rem',
          }}>
            {group.label}
          </h3>
          {group.fields.map(field => (
            <div key={field.key} className="report-field">
              <label style={{ fontSize: '0.82rem', color: 'var(--muted)', minWidth: '220px' }}>
                {field.label}
              </label>
              {field.type === 'select' ? (
                <select
                  className="report-input"
                  value={getValue(field.key)}
                  onChange={e => handleChange(field.key, e.target.value)}
                >
                  {(field.options ?? []).map(o => (
                    <option key={o} value={o}>{o || '— auto —'}</option>
                  ))}
                </select>
              ) : (
                <input
                  className="report-input"
                  type={field.type}
                  value={getValue(field.key)}
                  placeholder={getPlaceholder(field.key, field.type)}
                  onChange={e => handleChange(field.key, e.target.value)}
                  autoComplete="off"
                />
              )}
            </div>
          ))}
        </div>
      ))}
      <button className="run-btn" onClick={handleSave} disabled={saving}>
        {saving ? 'Saving…' : 'Save to .env'}
      </button>
    </div>
  )
}

// ── Price Alerts Tab ──────────────────────────────────────────────────────────

function PriceAlertsTab() {
  const { t } = useTranslation()
  const [alerts, setAlerts] = useState<PriceAlert[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')
  const [symbol, setSymbol] = useState('')
  const [target, setTarget] = useState('')
  const [condition, setCondition] = useState<'above' | 'below'>('above')
  const [note, setNote] = useState('')
  const [adding, setAdding] = useState(false)

  const reload = useCallback(() => {
    apiFetch<PriceAlert[]>('/price-alerts')
      .then(setAlerts)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { reload() }, [reload])

  // Auto-refresh every 30 seconds
  useEffect(() => {
    const id = setInterval(() => {
      apiFetch<PriceAlert[]>('/price-alerts')
        .then(setAlerts)
        .catch(() => {/* silently ignore refresh errors */})
    }, 30_000)
    return () => clearInterval(id)
  }, [])

  const add = useCallback(async () => {
    if (!symbol.trim() || !target.trim()) return
    const targetNum = parseFloat(target)
    if (isNaN(targetNum) || targetNum <= 0) {
      setError(t('invalid_target_price'))
      return
    }
    setAdding(true)
    setError('')
    try {
      await apiFetch<{ id: number }>('/price-alerts', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ symbol: symbol.trim().toUpperCase(), target: targetNum, condition, note: note.trim() }),
      })
      setSymbol('')
      setTarget('')
      setCondition('above')
      setNote('')
      setSuccess(t('price_alert_added'))
      setTimeout(() => setSuccess(''), 2000)
      reload()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : t('add_failed'))
    } finally {
      setAdding(false)
    }
  }, [symbol, target, condition, note, reload, t])

  const remove = useCallback(async (id: number) => {
    try {
      await apiFetch<null>(`/price-alerts/${id}`, { method: 'DELETE' })
      setAlerts((prev) => prev.filter((a) => a.ID !== id))
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : t('delete_failed'))
    }
  }, [t])

  const fmtPrice = (n: number) => {
    if (n >= 1000) return '$' + n.toLocaleString()
    if (n >= 1) return '$' + n.toFixed(2)
    return '$' + n.toPrecision(4)
  }

  if (loading) return <p className="loading">{t('loading')}</p>

  return (
    <>
      <p className="section-title">{t('price_alerts')}</p>

      {error && <p className="error-msg" style={{ marginBottom: 12 }}>{error}</p>}
      {success && <p style={{ color: 'var(--green)', marginBottom: 12, fontSize: '0.85rem' }}>{success}</p>}

      <div className="add-symbol-form">
        <input
          className="symbol-input"
          placeholder={t('symbol_placeholder_nvda')}
          value={symbol}
          onChange={(e) => setSymbol(e.target.value.toUpperCase())}
          onKeyDown={(e) => e.key === 'Enter' && add()}
        />
        <select
          className="chart-select"
          value={condition}
          onChange={(e) => setCondition(e.target.value as 'above' | 'below')}
        >
          <option value="above">{t('condition_above')}</option>
          <option value="below">{t('condition_below')}</option>
        </select>
        <input
          className="symbol-input"
          type="number"
          placeholder={t('target_price')}
          value={target}
          onChange={(e) => setTarget(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && add()}
          step="any"
          min="0"
        />
        <input
          className="symbol-input"
          placeholder={t('note_optional')}
          value={note}
          onChange={(e) => setNote(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && add()}
        />
        <button className="run-btn" onClick={add} disabled={adding || !symbol.trim() || !target.trim()}>
          {adding ? '...' : t('add')}
        </button>
      </div>

      <p className="section-title" style={{ marginTop: 24 }}>{t('registered_alerts')}</p>

      {alerts.length === 0 && <p className="loading">{t('no_price_alerts')}</p>}

      {alerts.map((alert) => (
        <div key={alert.ID} className="item">
          <div>
            <div className="item-name">
              <span className={`badge ${alert.Triggered ? 'badge-stock' : 'badge-crypto'}`}>
                {alert.Triggered ? t('triggered') : t('active')}
              </span>
              {alert.Symbol}
            </div>
            <div className="item-meta">
              {alert.Condition === 'above' ? '≥' : '≤'} {fmtPrice(alert.Target)}
              {alert.Note && ` — ${alert.Note}`}
            </div>
            <div className="item-meta" style={{ fontSize: '0.7rem' }}>
              {new Date(alert.CreatedAt).toLocaleString()}
              {alert.TriggeredAt && ` → ${new Date(alert.TriggeredAt).toLocaleString()}`}
            </div>
          </div>
          <button className="remove-btn" onClick={() => remove(alert.ID)} title={t('delete')}>✕</button>
        </div>
      ))}
    </>
  )
}

// ── root ──────────────────────────────────────────────────────────────────────

const CONFIG_TABS: Tab[] = ['symbols', 'rules', 'alert', 'price-alerts', 'report', 'status', 'settings']

const TAB_KEYS: Record<Tab, string> = {
  symbols: 'symbols',
  rules: 'rules',
  status: 'status',
  chart: 'chart',
  backtest: 'backtest',
  paper: 'paper',
  report: 'report',
  history: 'history',
  alert: 'alert',
  performance: 'performance',
  analysis: 'analysis',
  settings: 'settings',
  'price-alerts': 'price_alerts',
  calendar: 'calendar',
}

// ── Calendar Tab ─────────────────────────────────────────────────────────────

interface EconomicEvent {
  ID: number
  EventTime: string
  Country: string
  Event: string
  Impact: string
  Actual: string
  Forecast: string
  Previous: string
  Unit: string
}

function calendarDateHeader(dateKey: string, t: (key: string) => string): string {
  const today = new Date()
  const todayStr = today.toISOString().slice(0, 10)
  const tomorrowStr = new Date(today.getTime() + 86400000).toISOString().slice(0, 10)
  const yesterdayStr = new Date(today.getTime() - 86400000).toISOString().slice(0, 10)
  const label = new Date(dateKey + 'T00:00:00').toLocaleDateString(undefined, {
    weekday: 'short', month: 'short', day: 'numeric',
  })
  if (dateKey === todayStr) return `${t('calendar_today')} — ${label}`
  if (dateKey === tomorrowStr) return `${t('calendar_tomorrow')} — ${label}`
  if (dateKey === yesterdayStr) return `${t('calendar_yesterday')} — ${label}`
  return label
}

function CalendarTab() {
  const { t } = useTranslation()
  const [events, setEvents] = useState<EconomicEvent[]>([])
  const [loading, setLoading] = useState(true)
  const [fetchError, setFetchError] = useState(false)

  const fetchEvents = useCallback(() => {
    setFetchError(false)
    const now = new Date()
    const from = new Date(now.getTime() - 7 * 86400000).toISOString().slice(0, 10)
    const to = new Date(now.getTime() + 7 * 86400000).toISOString().slice(0, 10)
    apiFetch<EconomicEvent[]>(`/calendar?from=${from}&to=${to}`)
      .then((data) => setEvents(data ?? []))
      .catch(() => { setEvents([]); setFetchError(true) })
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    fetchEvents()
    const id = setInterval(fetchEvents, 5 * 60 * 1000)
    return () => clearInterval(id)
  }, [fetchEvents])

  if (loading) return <p className="loading">{t('loading')}</p>

  if (fetchError) {
    return (
      <div className="calendar-error">
        <p>⚠️ {t('calendar_load_error')}</p>
        <button className="calendar-retry-btn" onClick={() => { setLoading(true); fetchEvents() }}>
          {t('calendar_retry')}
        </button>
      </div>
    )
  }

  if (events.length === 0) {
    return (
      <div className="calendar-empty">
        <p className="calendar-empty-title">{t('calendar_no_data')}</p>
        <p className="calendar-empty-desc">{t('calendar_api_key_required')}</p>
        <ul className="calendar-empty-list">
          <li>FMP API Key — <a href="https://financialmodelingprep.com/developer/docs" target="_blank" rel="noreferrer">financialmodelingprep.com</a></li>
          <li>Finnhub API Key — <a href="https://finnhub.io/pricing" target="_blank" rel="noreferrer">finnhub.io</a></li>
        </ul>
        <p className="calendar-empty-desc">{t('calendar_restart_hint')}</p>
      </div>
    )
  }

  const grouped: Record<string, EconomicEvent[]> = {}
  for (const ev of events) {
    const dateKey = ev.EventTime.slice(0, 10)
    ;(grouped[dateKey] ??= []).push(ev)
  }

  const now = new Date()

  return (
    <>
      <p className="section-title">{t('calendar')}</p>
      {Object.entries(grouped)
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([date, items]) => (
          <div key={date}>
            <div className="calendar-date-header">{calendarDateHeader(date, t)}</div>
            {items.map((ev) => {
              const evTime = new Date(ev.EventTime)
              const timeStr = evTime.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
              const isPast = evTime < now
              const impactClass =
                ev.Impact === 'high' ? 'calendar-impact-high' :
                ev.Impact === 'medium' ? 'calendar-impact-medium' : 'calendar-impact-low'

              return (
                <div key={ev.ID} className="calendar-event">
                  <span className="calendar-time">{timeStr}</span>
                  <span className={impactClass} aria-label={`${t('calendar_impact_label')}: ${ev.Impact.toUpperCase()}`}>{ev.Impact.toUpperCase()}</span>
                  <span style={{ flex: 1 }}>{ev.Event} ({ev.Country})</span>
                  {ev.Actual ? (
                    <span className="calendar-actual">{ev.Actual}{ev.Unit}</span>
                  ) : isPast ? (
                    <span className="calendar-forecast">{t('calendar_released')}</span>
                  ) : (
                    <span className="calendar-forecast">{t('calendar_forecast_prefix')} {ev.Forecast}{ev.Unit}</span>
                  )}
                  <span className="calendar-forecast">{t('calendar_prev_prefix')} {ev.Previous}{ev.Unit}</span>
                </div>
              )
            })}
          </div>
        ))}
    </>
  )
}

export function App() {
  const { t } = useTranslation()
  const [tab, setTab] = useState<Tab>('chart')
  const [menuOpen, setMenuOpen] = useState(false)
  const [showOnboarding, setShowOnboarding] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)
  const [wsConnected, setWsConnected] = useState(false)
  const [liveSignal, setLiveSignal] = useState<SignalBar | null>(null)
  const wsRef = useRef<WebSocket | null>(null)

  // WebSocket auto-reconnecting connection
  useEffect(() => {
    let ws: WebSocket
    let reconnectTimer: ReturnType<typeof setTimeout>

    const connect = () => {
      const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      ws = new WebSocket(`${proto}//${window.location.host}/ws`)
      wsRef.current = ws

      ws.onopen = () => setWsConnected(true)

      ws.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data)
          if (msg.type === 'signal' && msg.payload) {
            setLiveSignal(msg.payload as SignalBar)
            setTimeout(() => setLiveSignal(null), 6000)
          }
        } catch {}
      }

      ws.onclose = () => {
        setWsConnected(false)
        reconnectTimer = setTimeout(connect, 3000)
      }

      ws.onerror = () => ws.close()
    }

    connect()
    return () => {
      clearTimeout(reconnectTimer)
      ws?.close()
    }
  }, [])

  // Show onboarding modal on first run (no localStorage key set)
  useEffect(() => {
    const done = localStorage.getItem(ONBOARDING_DONE_KEY)
    if (!done) setShowOnboarding(true)
  }, [])

  const isConfigTab = CONFIG_TABS.includes(tab)

  // Close menu on outside click
  useEffect(() => {
    if (!menuOpen) return
    const handler = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setMenuOpen(false)
      }
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [menuOpen])

  const goConfig = (t: Tab) => {
    setTab(t)
    setMenuOpen(false)
  }

  const handleGoToSettings = () => setTab('settings')

  return (
    <div className="container">
      <header className="header">
        <div className="header-top">
          <div>
            <h1><span className="brand">Chart</span> Analyzer
              <span className={`ws-indicator ${wsConnected ? 'ws-live' : 'ws-offline'}`}>
                {wsConnected ? '● LIVE' : '○ --'}
              </span>
            </h1>
            <p className="header-sub">{t('header_sub')}</p>
          </div>
          <div ref={menuRef} style={{ position: 'relative' }}>
            <button
              className={`hamburger-btn${isConfigTab ? ' config-active' : ''}`}
              onClick={() => setMenuOpen(o => !o)}
              title="Config"
            >
              ☰
            </button>
            {menuOpen && (
              <div className="config-menu">
                {(['symbols', 'rules', 'alert', 'report', 'status', 'settings'] as const).map(tabKey => (
                  <button
                    key={tabKey}
                    className={`config-menu-item${tab === tabKey ? ' active' : ''}`}
                    onClick={() => goConfig(tabKey)}
                  >
                    {t(TAB_KEYS[tabKey])}
                  </button>
                ))}
                <div style={{ borderTop: '1px solid rgba(91,146,121,0.2)', marginTop: '8px', paddingTop: '8px' }}>
                  <div style={{ fontSize: '0.7rem', color: 'var(--muted)', marginBottom: '6px' }}>{t('language')}</div>
                  {(['en', 'ko', 'ja'] as const).map(lang => (
                    <button
                      key={lang}
                      onClick={() => { i18n.changeLanguage(lang); localStorage.setItem('language', lang) }}
                      style={{
                        display: 'block',
                        width: '100%',
                        textAlign: 'left' as const,
                        padding: '5px 8px',
                        background: i18n.language === lang ? 'rgba(91,146,121,0.2)' : 'transparent',
                        border: 'none',
                        color: 'var(--text)',
                        fontSize: '0.82rem',
                        cursor: 'pointer',
                        borderRadius: '4px',
                      }}
                    >
                      {lang === 'en' ? 'English' : lang === 'ko' ? '한국어' : '日本語'}
                    </button>
                  ))}
                </div>
              </div>
            )}
          </div>
        </div>
        <nav className="tabs">
          <button className={`tab-btn${tab === 'chart' ? ' active' : ''}`} onClick={() => setTab('chart')}>{t('chart')}</button>
          <button className={`tab-btn${tab === 'analysis' ? ' active' : ''}`} onClick={() => setTab('analysis')}>{t('analysis')}</button>
          <button className={`tab-btn${tab === 'backtest' ? ' active' : ''}`} onClick={() => setTab('backtest')}>{t('backtest')}</button>
          <button className={`tab-btn${tab === 'performance' ? ' active' : ''}`} onClick={() => setTab('performance')}>{t('performance')}</button>
          <button className={`tab-btn${tab === 'paper' ? ' active' : ''}`} onClick={() => setTab('paper')}>{t('paper')}</button>
          <button className={`tab-btn${tab === 'history' ? ' active' : ''}`} onClick={() => setTab('history')}>{t('history')}</button>
          <button className={`tab-btn${tab === 'calendar' ? ' active' : ''}`} onClick={() => setTab('calendar')}>{t('calendar')}</button>
        </nav>
      </header>
      <main>
        {tab === 'chart' && <ChartTab />}
        {tab === 'analysis' && <AnalysisTab />}
        {tab === 'backtest' && <BacktestTab />}
        {tab === 'performance' && <PerformanceTab />}
        {tab === 'paper' && <PaperTab />}
        {tab === 'history' && <HistoryTab />}
        {tab === 'symbols' && <SymbolsTab />}
        {tab === 'rules' && <RulesTab />}
        {tab === 'alert' && <AlertTab />}
        {tab === 'report' && <ReportTab />}
        {tab === 'status' && <StatusTab />}
        {tab === 'settings' && <SettingsTab />}
        {tab === 'price-alerts' && <PriceAlertsTab />}
        {tab === 'calendar' && <CalendarTab />}
      </main>
      {showOnboarding && (
        <OnboardingModal
          onClose={() => setShowOnboarding(false)}
          onGoToSettings={handleGoToSettings}
        />
      )}
      {liveSignal && (
        <div className="ws-toast" onClick={() => setLiveSignal(null)}>
          <span className="ws-toast-dir" data-dir={liveSignal.direction}>{liveSignal.direction}</span>
          <strong>{liveSignal.symbol}</strong>
          <span>{liveSignal.rule}</span>
          <span className="ws-toast-score">{liveSignal.score?.toFixed(1)}</span>
          <span className="ws-toast-close">×</span>
        </div>
      )}
    </div>
  )
}
