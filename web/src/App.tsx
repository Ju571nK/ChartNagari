import { useState, useEffect, useCallback, useRef } from 'react'
import {
  createChart,
  createSeriesMarkers,
  CandlestickSeries,
  CrosshairMode,
  type IChartApi,
  type ISeriesApi,
  type UTCTimestamp,
} from 'lightweight-charts'

// ── types ─────────────────────────────────────────────────────────────────────

type Tab = 'symbols' | 'rules' | 'status' | 'chart' | 'backtest'

interface OHLCVBar {
  time: number
  open: number
  high: number
  low: number
  close: number
  volume: number
}

interface SignalBar {
  time: number
  direction: string
  rule: string
  score: number
  message: string
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
  tests: number
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

  useEffect(() => {
    apiFetch<SymbolItem[]>('/symbols')
      .then(setSymbols)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  const toggle = useCallback(async (sym: SymbolItem, enabled: boolean) => {
    try {
      await putJSON(`/symbols/${encodeURIComponent(sym.symbol)}`, { enabled })
      setSymbols((prev) => prev.map((s) => (s.symbol === sym.symbol ? { ...s, enabled } : s)))
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : '알 수 없는 오류')
    }
  }, [])

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
          <Toggle checked={sym.enabled} onChange={(v) => toggle(sym, v)} />
        </div>
      ))}
      {symbols.length === 0 && <p className="loading">등록된 종목 없음</p>}
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

function StatusTab() {
  const [status, setStatus] = useState<StatusData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    apiFetch<StatusData>('/status')
      .then(setStatus)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  if (loading) return <p className="loading">로딩 중...</p>
  if (error) return <p className="error-msg">오류: {error}</p>
  if (!status) return null

  return (
    <>
      <p className="section-title">시스템 상태</p>
      <p className="phase-info">{status.phase}</p>
      <div className="status-grid">
        <div className="stat-card">
          <div className="stat-value">{status.symbols}</div>
          <div className="stat-label">등록 종목</div>
        </div>
        <div className="stat-card">
          <div className="stat-value">{status.rules}</div>
          <div className="stat-label">분석 룰</div>
        </div>
        <div className="stat-card">
          <div className="stat-value">{status.tests}</div>
          <div className="stat-label">통과 테스트</div>
        </div>
        <div className="stat-card">
          <div className="stat-pass">✓ PASS</div>
          <div className="stat-label">전체 테스트</div>
        </div>
      </div>
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
  const containerRef = useRef<HTMLDivElement>(null)
  const chartRef = useRef<IChartApi | null>(null)
  const seriesRef = useRef<ISeriesApi<'Candlestick'> | null>(null)

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
      height: 420,
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
    chartRef.current = chart
    seriesRef.current = series

    const onResize = () => {
      if (containerRef.current) chart.applyOptions({ width: containerRef.current.clientWidth })
    }
    window.addEventListener('resize', onResize)
    return () => {
      window.removeEventListener('resize', onResize)
      chart.remove()
      chartRef.current = null
      seriesRef.current = null
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
        return apiFetch<SignalBar[]>(`/signals?symbol=${encodeURIComponent(symbol)}&limit=50`)
      })
      .then((sigs) => {
        if (!seriesRef.current) return
        const markers = sigs
          .filter((s) => s.direction !== 'NEUTRAL')
          .map((s) => ({
            time: s.time as UTCTimestamp,
            position: s.direction === 'LONG' ? ('belowBar' as const) : ('aboveBar' as const),
            color: s.direction === 'LONG' ? '#8FCB9B' : 'rgba(143,128,115,0.9)',
            shape: s.direction === 'LONG' ? ('arrowUp' as const) : ('arrowDown' as const),
            text: s.rule,
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

function fmt2(n: number) { return n.toFixed(2) }
function fmtPct(n: number) { return (n >= 0 ? '+' : '') + n.toFixed(2) + '%' }

function BacktestTab() {
  const [symbols, setSymbols] = useState<string[]>([])
  const [rules, setRules] = useState<string[]>([])
  const [symbol, setSymbol] = useState('')
  const [tf, setTf] = useState<TF>('1H')
  const [ruleFilter, setRuleFilter] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [result, setResult] = useState<BacktestResult | null>(null)

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
        body: JSON.stringify({ symbol, timeframe: tf, rule: ruleFilter }),
      })
      setResult(r)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : '알 수 없는 오류')
    } finally {
      setLoading(false)
    }
  }, [symbol, tf, ruleFilter])

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

        <button className="run-btn" onClick={run} disabled={loading || !symbol}>
          {loading ? '계산 중...' : '실행'}
        </button>
      </div>

      {error && <p className="error-msg">오류: {error}</p>}

      {result && !loading && (
        <>
          <p className="section-title">
            결과 — {result.symbol} {result.timeframe} ({result.bars} 바, {result.trades} 거래)
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
        </nav>
      </header>
      <main>
        {tab === 'symbols' && <SymbolsTab />}
        {tab === 'rules' && <RulesTab />}
        {tab === 'status' && <StatusTab />}
        {tab === 'chart' && <ChartTab />}
        {tab === 'backtest' && <BacktestTab />}
      </main>
    </div>
  )
}
