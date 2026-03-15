import { useState, useEffect, useCallback, useRef } from 'react'
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

type Tab = 'symbols' | 'rules' | 'status' | 'chart' | 'backtest' | 'paper' | 'report' | 'history' | 'alert' | 'performance'

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

const RULE_DESC: Record<string, string> = {
  rsi_overbought_oversold:     'RSI 과매수/과매도 구간 신호',
  rsi_divergence:              'RSI 다이버전스 — 추세 전환 조기 감지',
  ema_cross:                   'EMA 크로스 — 단기/장기 방향 전환',
  support_resistance_breakout: '지지·저항 돌파 — 레벨 브레이크아웃',
  fibonacci_confluence:        '피보나치 수렴 구간 — 반전 가능 지점',
  volume_spike:                '거래량 급증 — 세력 개입 감지',
  ict_order_block:             'Order Block — 기관 매수/매도 구간',
  ict_fair_value_gap:          'Fair Value Gap — 미체결 가격 공백',
  ict_liquidity_sweep:         'Liquidity Sweep — 스톱 헌팅 후 반전',
  ict_breaker_block:           'Breaker Block — 무효화된 OB 반전 구간',
  ict_kill_zone:               'Kill Zone — 런던/뉴욕 세션 주요 시간대',
  wyckoff_accumulation:        'Wyckoff 축적 — 세력 매집 국면',
  wyckoff_distribution:        'Wyckoff 분배 — 세력 매도 국면',
  wyckoff_spring:              'Spring — 저점 이탈 후 급반등',
  wyckoff_upthrust:            'Upthrust — 고점 돌파 후 급락',
  wyckoff_volume_anomaly:      '비정상 거래량 — 이상 세력 개입 신호',
  smc_bos:                     'BOS — Break of Structure (추세 지속)',
  smc_choch:                   'CHoCH — Change of Character (추세 전환)',
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
  const [symbols, setSymbols] = useState<SymbolItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [newSymbol, setNewSymbol] = useState('')
  const [newType, setNewType] = useState<'crypto' | 'stock'>('stock')
  const [newExchange, setNewExchange] = useState('')
  const [adding, setAdding] = useState(false)

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
      setError(e instanceof Error ? e.message : '알 수 없는 오류')
    }
  }, [])

  const remove = useCallback(async (sym: SymbolItem) => {
    if (!confirm(`${sym.symbol}을 삭제할까요?`)) return
    try {
      await apiFetch<null>(`/symbols/${encodeURIComponent(sym.symbol)}`, { method: 'DELETE' })
      setSymbols((prev) => prev.filter((s) => s.symbol !== sym.symbol))
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : '삭제 실패')
    }
  }, [])

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
      setError(e instanceof Error ? e.message : '추가 실패')
    } finally {
      setAdding(false)
    }
  }, [newSymbol, newType, newExchange, reload])

  if (loading) return <p className="loading">로딩 중...</p>
  if (error) return <p className="error-msg">오류: {error}</p>

  return (
    <>
      <p className="section-title">종목 관리</p>
      {symbols.map((sym) => (
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
            <button className="remove-btn" onClick={() => remove(sym)} title="삭제">✕</button>
          </div>
        </div>
      ))}
      {symbols.length === 0 && <p className="loading">등록된 종목 없음</p>}

      {/* 종목 추가 폼 */}
      <p className="section-title" style={{ marginTop: 24 }}>종목 추가</p>
      <div className="add-symbol-form">
        <select
          className="chart-select"
          value={newType}
          onChange={(e) => setNewType(e.target.value as 'crypto' | 'stock')}
        >
          <option value="stock">주식</option>
          <option value="crypto">코인</option>
        </select>
        <input
          className="symbol-input"
          placeholder="심볼 (예: NVDA)"
          value={newSymbol}
          onChange={(e) => setNewSymbol(e.target.value.toUpperCase())}
          onKeyDown={(e) => e.key === 'Enter' && add()}
        />
        <input
          className="symbol-input"
          placeholder="거래소 (예: nasdaq)"
          value={newExchange}
          onChange={(e) => setNewExchange(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && add()}
        />
        <button className="run-btn" onClick={add} disabled={adding || !newSymbol.trim()}>
          {adding ? '...' : '추가'}
        </button>
      </div>
      <p className="item-meta" style={{ marginTop: 8 }}>
        ⚠️ 추가 후 서버 재시작 시 데이터 수집이 시작됩니다
      </p>
    </>
  )
}

function RulesTab() {
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
      setError(e instanceof Error ? e.message : '알 수 없는 오류')
    }
  }, [])

  if (loading) return <p className="loading">로딩 중...</p>
  if (error) return <p className="error-msg">오류: {error}</p>

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
                {RULE_DESC[rule.name] && (
                  <div className="item-meta">{RULE_DESC[rule.name]}</div>
                )}
              </div>
              <Toggle checked={rule.enabled} onChange={(v) => toggle(rule, v)} />
            </div>
          ))}
        </div>
      ))}
      {rules.length === 0 && <p className="loading">등록된 룰 없음</p>}
    </>
  )
}

function fmtUptime(sec: number | undefined): string {
  if (!sec || isNaN(sec)) return '계산 중...'
  if (sec < 60) return `${sec}초`
  if (sec < 3600) return `${Math.floor(sec / 60)}분`
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  return `${h}시간 ${m}분`
}

function fmtRelTime(unix: number): string {
  if (!unix) return '신호 없음'
  const diff = Math.floor(Date.now() / 1000 - unix)
  if (diff < 60) return `${diff}초 전`
  if (diff < 3600) return `${Math.floor(diff / 60)}분 전`
  if (diff < 86400) return `${Math.floor(diff / 3600)}시간 전`
  return `${Math.floor(diff / 86400)}일 전`
}

function StatusTab() {
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

  if (loading) return <p className="loading">로딩 중...</p>
  if (error) return <p className="error-msg">오류: {error}</p>
  if (!status) return null

  return (
    <>
      {/* 파이프라인 활성 배너 */}
      <div className="status-banner">
        <span className="status-dot" />
        <span>파이프라인 실행 중</span>
        <span className="status-uptime">가동 시간 {fmtUptime(status.uptime_sec)}</span>
      </div>

      {/* 데이터 소스 */}
      <p className="section-title">데이터 소스</p>
      <div className="source-list">
        {(status.data_sources ?? []).map((src) => (
          <div key={src} className="source-item">
            <span className="source-dot">✅</span>
            <span>{src}</span>
          </div>
        ))}
      </div>

      {/* 분석 현황 */}
      <p className="section-title">분석 현황</p>
      <div className="status-grid">
        <div className="stat-card">
          <div className="stat-value">{status.symbols}</div>
          <div className="stat-label">모니터링 종목</div>
        </div>
        <div className="stat-card">
          <div className="stat-value">{status.rules}</div>
          <div className="stat-label">활성 룰</div>
        </div>
        <div className="stat-card" style={{ gridColumn: 'span 2' }}>
          <div className="stat-value" style={{ fontSize: '1.2rem' }}>
            {fmtRelTime(status.last_signal_unix)}
          </div>
          <div className="stat-label">마지막 신호 감지</div>
        </div>
      </div>

      <p className="phase-info" style={{ marginTop: 16 }}>{status.phase}</p>
    </>
  )
}

// ── Chart Tab ─────────────────────────────────────────────────────────────────

const TFS = ['1H', '4H', '1D', '1W'] as const
type TF = (typeof TFS)[number]

function ChartTab() {
  const [symbol, setSymbol] = useState('')
  const [symbols, setSymbols] = useState<string[]>([])
  const [tf, setTf] = useState<TF>('1H')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [signals, setSignals] = useState<SignalBar[]>([])
  const containerRef = useRef<HTMLDivElement>(null)
  const chartRef = useRef<IChartApi | null>(null)
  const seriesRef = useRef<ISeriesApi<'Candlestick'> | null>(null)
  const volRef = useRef<ISeriesApi<'Histogram'> | null>(null)

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

  return (
    <>
      <div className="chart-controls">
        <select
          className="chart-select"
          value={symbol}
          onChange={(e) => setSymbol(e.target.value)}
        >
          {symbols.length === 0 && <option value="">종목 없음</option>}
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
      </div>
      {loading && <p className="loading">차트 로딩 중...</p>}
      {error && <p className="error-msg">데이터 없음 — {error}</p>}
      <div ref={containerRef} className="chart-area" />
      {signals.some((s) => s.ai_interpretation) && (
        <>
          <p className="section-title" style={{ marginTop: 20 }}>AI 해석</p>
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
  entry_time: number
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
      setError(e instanceof Error ? e.message : '알 수 없는 오류')
    } finally {
      setLoading(false)
    }
  }, [symbol, tf, ruleFilter, tpMult, slMult])

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
      setError(e instanceof Error ? e.message : '알 수 없는 오류')
    } finally {
      setRulesLoading(false)
    }
  }, [symbol, tf, tpMult, slMult])

  return (
    <>
      <p className="section-title">백테스트 설정</p>
      <div className="backtest-controls">
        <select
          className="chart-select"
          value={symbol}
          onChange={(e) => setSymbol(e.target.value)}
          disabled={loading}
        >
          {symbols.length === 0 && <option value="">종목 없음</option>}
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
          <option value="">전체 룰</option>
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
          {loading ? '계산 중...' : '실행'}
        </button>
        <button
          className="run-btn"
          onClick={runPerRule}
          disabled={rulesLoading || !symbol}
          style={{ background: 'rgba(91,146,121,0.12)', color: 'var(--green)', border: '1px solid rgba(91,146,121,0.4)' }}
        >
          {rulesLoading ? '분석 중...' : '룰별 분석'}
        </button>
      </div>

      {error && <p className="error-msg">오류: {error}</p>}

      {result && !loading && (
        <>
          <p className="section-title">
            결과 — {result.symbol} {result.timeframe} ({result.bars} 바, {result.trades} 거래) | TP×{tpMult} SL×{slMult}
          </p>

          <div className="backtest-stats">
            <div className="stat-card">
              <div className="stat-value" style={{ fontSize: '1.4rem' }}>
                {(result.stats.win_rate * 100).toFixed(1)}%
              </div>
              <div className="stat-label">승률</div>
            </div>
            <div className="stat-card">
              <div className="stat-value" style={{ fontSize: '1.4rem' }}>
                {fmt2(result.stats.avg_rr)}
              </div>
              <div className="stat-label">평균 손익비</div>
            </div>
            <div className="stat-card">
              <div className="stat-value" style={{ fontSize: '1.4rem' }}>
                {fmt2(result.stats.profit_factor)}
              </div>
              <div className="stat-label">수익 팩터</div>
            </div>
            <div className="stat-card">
              <div className="stat-value" style={{ fontSize: '1.4rem', color: 'var(--muted)' }}>
                {(result.stats.max_drawdown * 100).toFixed(1)}%
              </div>
              <div className="stat-label">최대낙폭</div>
            </div>
            <div className="stat-card">
              <div className="stat-value" style={{ fontSize: '1.4rem' }}>
                {fmt2(result.stats.sharpe)}
              </div>
              <div className="stat-label">샤프비율</div>
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
              <div className="stat-label">누적 수익률</div>
            </div>
          </div>

          {result.outcomes && result.outcomes.length > 0 && (
            <>
              <p className="section-title" style={{ marginTop: 24 }}>거래 목록</p>
              <div className="backtest-table-wrap">
                <table className="backtest-table">
                  <thead>
                    <tr>
                      <th>진입 시간</th>
                      <th>방향</th>
                      <th>룰</th>
                      <th>진입가</th>
                      <th>청산가</th>
                      <th>바</th>
                      <th>수익률</th>
                    </tr>
                  </thead>
                  <tbody>
                    {result.outcomes.map((o, i) => (
                      <tr key={i} className={o.win ? 'outcome-win' : 'outcome-loss'}>
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
            <p className="loading">데이터 없음 — 수집된 OHLCV 바가 충분하지 않거나 신호가 발생하지 않았습니다.</p>
          )}
        </>
      )}

      {ruleStats !== null && (
        <>
          <p className="section-title" style={{ marginTop: 24 }}>
            룰별 성과 분석 — {symbol} {tf} | TP×{tpMult} SL×{slMult}
          </p>
          {ruleStats.length === 0 ? (
            <p className="loading">거래 데이터 없음 — OHLCV 수집 기간을 늘려주세요.</p>
          ) : (
            <>
              <div className="backtest-table-wrap">
                <table className="backtest-table">
                  <thead>
                    <tr>
                      <th>룰</th><th>거래 수</th><th>승률</th><th>평균 RR</th><th>수익 팩터</th><th>누적 수익</th><th>Export</th>
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
                🟢 승률 ≥ 45% 달성 &nbsp;|&nbsp; 🔴 미달 — 해당 룰 OFF 또는 스코어 임계값 상향 고려
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

  if (loading) return <p className="loading">로딩 중...</p>

  return (
    <>
      <p className="section-title">페이퍼 트레이딩 — 실시간 가상 포트폴리오</p>

      {summary && (
        <div className="backtest-stats">
          <div className="stat-card">
            <div className="stat-value" style={{ fontSize: '1.4rem' }}>{summary.open_positions}</div>
            <div className="stat-label">오픈 포지션</div>
          </div>
          <div className="stat-card">
            <div className="stat-value" style={{ fontSize: '1.4rem' }}>{summary.closed_trades}</div>
            <div className="stat-label">총 거래</div>
          </div>
          <div className="stat-card">
            <div className="stat-value" style={{ fontSize: '1.4rem' }}>{(summary.win_rate * 100).toFixed(1)}%</div>
            <div className="stat-label">승률</div>
          </div>
          <div className="stat-card">
            <div
              className="stat-value"
              style={{ fontSize: '1.4rem', color: summary.total_pnl_pct >= 0 ? 'var(--mint)' : 'var(--muted)' }}
            >
              {summary.total_pnl_pct >= 0 ? '+' : ''}{summary.total_pnl_pct.toFixed(2)}%
            </div>
            <div className="stat-label">누적 손익</div>
          </div>
          <div className="stat-card">
            <div className="stat-value" style={{ fontSize: '1.4rem', color: 'var(--mint)' }}>
              +{summary.avg_win_pct.toFixed(2)}%
            </div>
            <div className="stat-label">평균 수익</div>
          </div>
          <div className="stat-card">
            <div className="stat-value" style={{ fontSize: '1.4rem', color: 'var(--muted)' }}>
              {summary.avg_loss_pct.toFixed(2)}%
            </div>
            <div className="stat-label">평균 손실</div>
          </div>
        </div>
      )}

      {positions.length > 0 && (
        <>
          <p className="section-title" style={{ marginTop: 24 }}>오픈 포지션 ({positions.length})</p>
          <div className="backtest-table-wrap">
            <table className="backtest-table">
              <thead>
                <tr><th>종목</th><th>방향</th><th>룰</th><th>진입가</th><th>TP</th><th>SL</th><th>진입 시간</th></tr>
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
        <p className="loading" style={{ marginTop: 16 }}>오픈 포지션 없음 — 신호 발생 시 자동으로 포지션이 생성됩니다.</p>
      )}

      {history.length > 0 && (
        <>
          <p className="section-title" style={{ marginTop: 24 }}>청산 히스토리</p>
          <div className="backtest-table-wrap">
            <table className="backtest-table">
              <thead>
                <tr><th>종목</th><th>방향</th><th>진입가</th><th>청산가</th><th>결과</th><th>손익</th></tr>
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
      setError(e instanceof Error ? e.message : '저장 실패')
    } finally {
      setSaving(false)
    }
  }, [config])

  if (!config && !error) return <p className="loading">로딩 중...</p>
  if (error && !config) return <p className="error-msg">오류: {error}</p>
  if (!config) return null

  return (
    <>
      <p className="section-title">일일 리포트 설정</p>

      <div className="report-field">
        <span className="field-label">리포트 활성화</span>
        <Toggle checked={config.enabled} onChange={(v) => setConfig({ ...config, enabled: v })} />
      </div>

      <div className="report-field">
        <span className="field-label">발송 시간 (KST)</span>
        <input
          className="report-input"
          placeholder="09:00"
          value={config.time}
          onChange={(e) => setConfig({ ...config, time: e.target.value })}
        />
      </div>

      <div className="report-field">
        <span className="field-label">시간대</span>
        <input
          className="report-input"
          value={config.timezone}
          readOnly
          style={{ opacity: 0.6, cursor: 'default' }}
        />
      </div>

      <div className="report-field">
        <span className="field-label">AI 최소 스코어</span>
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
        <span className="field-label">신호 없는 날 스킵</span>
        <Toggle checked={config.only_if_signals} onChange={(v) => setConfig({ ...config, only_if_signals: v })} />
      </div>

      <div className="report-field">
        <span className="field-label">축약 모드</span>
        <Toggle checked={config.compact} onChange={(v) => setConfig({ ...config, compact: v })} />
      </div>

      <div style={{ marginTop: 24, display: 'flex', alignItems: 'center', gap: 12 }}>
        <button className="run-btn" onClick={save} disabled={saving}>
          {saving ? '저장 중...' : '저장'}
        </button>
        {saved && <span className="save-success">✓ 저장됨</span>}
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
      setError(e instanceof Error ? e.message : '저장 실패')
    } finally {
      setSaving(false)
    }
  }, [config])

  if (!config && !error) return <p className="loading">로딩 중...</p>
  if (error && !config) return <p className="error-msg">오류: {error}</p>
  if (!config) return null

  return (
    <>
      <p className="section-title">알림 설정</p>

      <div className="report-field">
        <span className="field-label">신호 스코어 임계값</span>
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
        이 점수 이상 신호만 Telegram으로 발송됩니다 (현재 기본값: 12.0)
      </p>

      <div className="report-field">
        <span className="field-label">중복 방지 쿨다운 (시간)</span>
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
        같은 종목+룰 알림 최소 간격
      </p>

      <div className="report-field">
        <span className="field-label">MTF 합의 필터</span>
        <select
          className="report-input"
          value={config.mtf_consensus_min}
          onChange={(e) => setConfig({ ...config, mtf_consensus_min: parseInt(e.target.value) })}
        >
          <option value={1}>비활성 (단일 TF 신호도 알림)</option>
          <option value={2}>2개 이상 TF 합의 (권장)</option>
          <option value={3}>3개 이상 TF 합의</option>
          <option value={4}>4개 TF 전체 합의</option>
        </select>
      </div>
      <p className="item-meta" style={{ marginBottom: 12 }}>
        여러 타임프레임에서 동시에 같은 방향 신호가 나올 때만 알림 발송 → 역추세 포지션 감소
      </p>

      <p className="section-title" style={{ marginTop: 20 }}>TP/SL 배율 설정</p>
      <p className="item-meta">TP = 진입가 ± ATR × 배율. 낮을수록 빠른 청산, 높을수록 큰 목표가</p>

      <div className="report-field">
        <span className="field-label">코인 TP 배율</span>
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
        <span className="field-label">코인 SL 배율</span>
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
        <span className="field-label">주식 TP 배율</span>
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
        <span className="field-label">주식 SL 배율</span>
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
          {saving ? '저장 중...' : '저장'}
        </button>
        {saved && <span className="save-success">✓ 저장됨</span>}
        {error && <span className="error-msg" style={{ padding: '4px 8px' }}>{error}</span>}
      </div>
    </>
  )
}

// ── History Tab ───────────────────────────────────────────────────────────────

function HistoryTab() {
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
          <option value="ALL">전체 종목</option>
          {symbols.map((s) => <option key={s} value={s}>{s}</option>)}
        </select>
        <select className="chart-select" value={direction} onChange={(e) => setDirection(e.target.value)}>
          <option value="ALL">전체 방향</option>
          <option value="LONG">LONG</option>
          <option value="SHORT">SHORT</option>
        </select>
        <select className="chart-select" value={limit} onChange={(e) => setLimit(Number(e.target.value))}>
          <option value={50}>50건</option>
          <option value={100}>100건</option>
          <option value={200}>200건</option>
        </select>
      </div>
      {loading && <p className="loading">로딩 중...</p>}
      {error && <p className="error-msg">오류: {error}</p>}
      {!loading && signals.length === 0 && (
        <p className="loading">신호 없음 — 수집 기간이 짧거나 필터 조건을 확인하세요.</p>
      )}
      {signals.length > 0 && (
        <div className="backtest-table-wrap">
          <table className="backtest-table">
            <thead>
              <tr>
                <th>시간</th>
                <th>종목</th>
                <th>TF</th>
                <th>방향</th>
                <th>룰</th>
                <th>스코어</th>
                <th>AI 해석</th>
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
      setError(e instanceof Error ? e.message : '오류 발생')
    } finally {
      setLoading(false)
    }
  }, [selected, tf, tpMult, slMult])

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
      <p className="section-title">심볼 선택</p>
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
          {symbols.length === selected.length ? '전체 해제' : '전체 선택'}
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
          {loading ? '분석 중...' : `${selected.length}개 심볼 분석`}
        </button>
      </div>

      {error && <p className="error-msg">오류: {error}</p>}

      {stats !== null && !loading && (
        <>
          <p className="section-title" style={{ marginTop: 24 }}>
            룰 성과 순위 — {tf} | TP×{tpMult} SL×{slMult} | {selected.length}개 심볼 집계
          </p>
          {stats.length === 0 ? (
            <p className="loading">데이터 없음 — OHLCV 수집 기간을 늘리거나 다른 TF를 선택하세요.</p>
          ) : (
            <>
              <div className="backtest-table-wrap">
                <table className="backtest-table">
                  <thead>
                    <tr>
                      <th>순위</th>
                      <th>룰</th>
                      <th>평균 승률</th>
                      <th>평균 RR</th>
                      <th>수익 팩터</th>
                      <th>총 거래</th>
                      <th>심볼 수</th>
                      <th style={{ minWidth: 140 }}>TradingView 내보내기</th>
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
                              {exporting === s.rule ? '...' : '📥 TV로 내보내기'}
                            </button>
                          ) : (
                            <span style={{ color: 'var(--muted)', fontSize: '0.75rem' }}>미지원</span>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <p className="item-meta" style={{ marginTop: 8 }}>
                🟢 승률 ≥ 45% | 1~3위 강조 | 승률 높은 룰만 TradingView에서 활용 권장
              </p>
            </>
          )}
        </>
      )}
    </>
  )
}

// ── root ──────────────────────────────────────────────────────────────────────

export function App() {
  const [tab, setTab] = useState<Tab>('symbols')

  return (
    <div className="container">
      <header className="header">
        <h1><span className="brand">Chart</span> Analyzer</h1>
        <p className="header-sub">ICT · Wyckoff · General TA — MTF 신호 분석 플랫폼</p>
        <nav className="tabs">
          <button className={`tab-btn${tab === 'symbols' ? ' active' : ''}`} onClick={() => setTab('symbols')}>
            종목
          </button>
          <button className={`tab-btn${tab === 'rules' ? ' active' : ''}`} onClick={() => setTab('rules')}>
            룰
          </button>
          <button className={`tab-btn${tab === 'status' ? ' active' : ''}`} onClick={() => setTab('status')}>
            상태
          </button>
          <button className={`tab-btn${tab === 'chart' ? ' active' : ''}`} onClick={() => setTab('chart')}>
            차트
          </button>
          <button className={`tab-btn${tab === 'backtest' ? ' active' : ''}`} onClick={() => setTab('backtest')}>
            백테스트
          </button>
          <button className={`tab-btn${tab === 'paper' ? ' active' : ''}`} onClick={() => setTab('paper')}>
            페이퍼
          </button>
          <button className={`tab-btn${tab === 'report' ? ' active' : ''}`} onClick={() => setTab('report')}>
            리포트
          </button>
          <button className={`tab-btn${tab === 'history' ? ' active' : ''}`} onClick={() => setTab('history')}>
            히스토리
          </button>
          <button className={`tab-btn${tab === 'alert' ? ' active' : ''}`} onClick={() => setTab('alert')}>
            알림
          </button>
          <button className={`tab-btn${tab === 'performance' ? ' active' : ''}`} onClick={() => setTab('performance')}>
            성과
          </button>
        </nav>
      </header>
      <main>
        {tab === 'symbols' && <SymbolsTab />}
        {tab === 'rules' && <RulesTab />}
        {tab === 'status' && <StatusTab />}
        {tab === 'chart' && <ChartTab />}
        {tab === 'backtest' && <BacktestTab />}
        {tab === 'paper' && <PaperTab />}
        {tab === 'report' && <ReportTab />}
        {tab === 'history' && <HistoryTab />}
        {tab === 'alert' && <AlertTab />}
        {tab === 'performance' && <PerformanceTab />}
      </main>
    </div>
  )
}
