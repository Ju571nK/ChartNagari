// demoApi.ts — static-demo API shim for the zero-install GitHub Pages build.
//
// When VITE_DEMO_STATIC === 'true', installDemoApi() patches window.fetch so
// every '/api/...' request resolves from canned JSON instead of a Go backend.
// The fixtures in web/public/demo/scan-{TF}.json are captured verbatim from the
// real rule engine (see internal/api/demo_capture_test.go), so the candles and
// signals shown are authentic detector output — not hand-faked data.
//
// The shim intercepts at the window.fetch layer so it transparently covers both
// App.tsx's apiFetch() wrapper and OnboardingModal.tsx's direct fetch() calls
// without touching either.

import { demoDxy, demoFxBars, demoFxSessions, demoSessionsFromBars } from './demoFx'
import { instrument } from './forex'

const DEMO_SYMBOL = 'DEMO_BTC'
const DEMO_FX = ['EURUSD', 'GBPUSD', 'USDJPY', 'USDCHF', 'AUDUSD', 'USDCAD', 'NZDUSD', 'XAUUSD'] as const
const demoSymbols = [
  ...['EURUSD', 'GBPUSD', 'USDJPY', 'USDCHF', 'AUDUSD', 'USDCAD', 'NZDUSD', 'XAUUSD'].map(symbol => ({ symbol, enabled: true, type: 'forex', exchange: 'FX' })),
  { symbol: DEMO_SYMBOL, enabled: true, type: 'crypto', exchange: 'binance' },
]

interface DemoBar {
  time: number
  open: number
  high: number
  low: number
  close: number
  volume: number
}

interface DemoSignal {
  symbol?: string
  timeframe?: string
  rule: string
  direction: string
  score: number
  message?: string
  zone_low?: number
  zone_high?: number
}

interface DemoScan {
  symbol: string
  timeframe: string
  bars: DemoBar[]
  signals: DemoSignal[]
}

// Captured fixture timeframes — must stay in sync with the capture loop in
// internal/api/demo_capture_test.go.
export const DEMO_TIMEFRAMES = ['1W', '1D', '4H', '1H'] as const

// Cache for loaded fixtures, keyed by timeframe.
const scanCache = new Map<string, Promise<DemoScan>>()

// The most recent OHLCV timeframe requested. /signals carries no timeframe
// param, so this tracks which fixture to align markers against.
let currentTf = '1D'

function loadScan(tf: string, realFetch: typeof fetch): Promise<DemoScan> {
  const key = (DEMO_TIMEFRAMES as readonly string[]).includes(tf) ? tf : '1D'
  let p = scanCache.get(key)
  if (!p) {
    const url = import.meta.env.BASE_URL + 'demo/scan-' + key + '.json'
    p = realFetch(url).then((r) => {
      // Without this, a 404 (wrong BASE_PATH, renamed fixture) would parse the
      // Pages HTML error page and fail with a useless JSON syntax error.
      if (!r.ok) throw new Error(`fixture ${url}: HTTP ${r.status}`)
      return r.json() as Promise<DemoScan>
    })
    // Evict on failure so a transient fetch error doesn't negative-cache the
    // timeframe for the whole session (demo would stay blank until reload).
    p.catch(() => scanCache.delete(key))
    scanCache.set(key, p)
  }
  return p
}

function jsonResponse(data: unknown, status = 200): Response {
  return new Response(JSON.stringify(data), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

const noContent = () => new Response(null, { status: 204 })

// Distribute MTF signals across the visible bars so markers don't all stack on
// the final candle (every demo signal carries created_at ≈ now). Placement is
// cosmetic; each signal is snapped to a local low (LONG) or high (SHORT) near an
// evenly-spaced anchor in the latter ~55% of the series.
function placeSignals(bars: DemoBar[], signals: DemoSignal[]): unknown[] {
  const n = bars.length
  if (n === 0) return []
  const dirSignals = signals.filter((s) => s.direction === 'LONG' || s.direction === 'SHORT')
  const k = dirSignals.length || 1
  return dirSignals.map((s, i) => {
    const frac = 0.45 + ((i + 0.5) / k) * 0.5 // 0.45 → 0.95
    let idx = Math.min(n - 1, Math.max(0, Math.round(frac * (n - 1))))
    const lo = Math.max(0, idx - 4)
    const hi = Math.min(n - 1, idx + 4)
    let best = idx
    for (let j = lo; j <= hi; j++) {
      if (s.direction === 'LONG') {
        if (bars[j].low < bars[best].low) best = j
      } else {
        if (bars[j].high > bars[best].high) best = j
      }
    }
    idx = best
    return {
      symbol: DEMO_SYMBOL,
      timeframe: s.timeframe ?? currentTf,
      time: bars[idx].time,
      direction: s.direction,
      rule: s.rule,
      score: s.score,
      message: s.message ?? '',
      ai_interpretation: '',
      zone_low: s.zone_low,
      zone_high: s.zone_high,
    }
  })
}

function urlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.href
  return input.url
}

async function route(path: string, search: URLSearchParams, realFetch: typeof fetch): Promise<Response> {
  // /demo/scan?symbol=&timeframe= → full captured scan (onboarding reads .signals)
  if (path === '/demo/scan') {
    const tf = search.get('timeframe') || '1D'
    const scan = await loadScan(tf, realFetch)
    return jsonResponse(scan)
  }

  // /ohlcv/{symbol}/{tf} → captured candles (remember tf for marker alignment)
  const ohlcv = path.match(/^\/ohlcv\/([^/]+)\/([^/]+)$/)
  if (ohlcv) {
    const sym = decodeURIComponent(ohlcv[1])
    const tf = ohlcv[2]
    if (sym === DEMO_SYMBOL) {
      const scan = await loadScan(tf, realFetch)
      currentTf = tf
      return jsonResponse(scan.bars)
    }
    if (DEMO_FX.some(item => item === sym)) { currentTf = tf; return jsonResponse(demoFxBars(sym, tf)) }
    return jsonResponse([]) // ^VIX overlay etc. — no data in demo
  }

  // /signals?symbol=DEMO_BTC → MTF signals placed onto the current timeframe's
  // bars. Bars come from the same cached fixture as /ohlcv (no shared mutable
  // bar state). Caveat: currentTf is still last-write-wins, so overlapping
  // timeframe switches can place a stale chain's markers on the newer TF's
  // bars — cosmetic only, and the newer chain's setSignals lands last anyway.
  if (path === '/signals') {
    if (DEMO_FX.some(item => item === search.get('symbol'))) {
      const sym = search.get('symbol')!
      const bars = demoFxBars(sym, currentTf)
      const sample = bars[Math.floor(bars.length * 0.7)]
      const providerSymbol = sym === 'XAUUSD' ? 'GC=F' : `${sym}=X`
      return jsonResponse(sample ? [{ symbol: sym, timeframe: currentTf, time: sample.time, direction: 'LONG', rule: 'ict_liquidity_sweep', score: 7.2, message: 'Demo sample', message_key: 'forex.demoSignal', ai_interpretation: '', zone_low: sample.low, zone_high: sample.high, entry_price: sample.close, tp: sample.close + 30 * instrument(sym).pip, sl: sample.close - 15 * instrument(sym).pip, tp_pips: 30, sl_pips: 15, data_provider: 'yahoo', provider_symbol: providerSymbol, source_identity: `yahoo|${providerSymbol}|${sym === 'XAUUSD' ? 'proxy' : 'spot'}`, data_proxy: sym === 'XAUUSD', volume_unconfirmed: true }] : [])
    }
    if (search.get('symbol') === DEMO_SYMBOL) {
      const scan = await loadScan(currentTf, realFetch)
      return jsonResponse(placeSignals(scan.bars, scan.signals))
    }
    return jsonResponse([])
  }

  // EURUSD leads the demo watchlist, so the chart opens on FX without setup.
  if (path === '/symbols') {
    return jsonResponse(demoSymbols)
  }

  if (path === '/forex/status') return jsonResponse({ provider: 'yahoo', configured_provider: 'auto', volume_quality: 'none', state: 'ready', message: 'Demo sample' })
  if (path === '/forex/strength') {
    const lookback = Number(search.get('lookback') || 20)
    const changes = new Map<string, number[]>()
    for (const pair of DEMO_FX) {
      const bars = demoFxBars(pair, search.get('tf') || '4H')
      if (bars.length <= lookback) continue
      const change = (bars.at(-1)!.close / bars.at(-1 - lookback)!.close - 1) * 100
      for (const [currency, value] of [[pair.slice(0, 3), change], [pair.slice(3), -change]] as const) {
        changes.set(currency, [...(changes.get(currency) ?? []), value])
      }
    }
    const averages = [...changes].map(([currency, values]) => ({ currency, pairs: values.length, average: values.reduce((sum, v) => sum + v, 0) / values.length }))
    const max = Math.max(0, ...averages.filter(item => item.pairs >= 2).map(item => Math.abs(item.average)))
    return jsonResponse({ currencies: averages.map(item => ({ currency: item.currency, pairs: item.pairs, coverage: item.pairs >= 2 ? 'ok' : 'low', value: item.pairs >= 2 ? (max ? item.average / max * 100 : 0) : null })) })
  }
  if (path === '/forex/dxy') {
    const usdBase = search.get('symbol')?.startsWith('USD')
    return jsonResponse({ bars: demoDxy(), correlation_20: usdBase ? 0.72 : -0.78, correlation_60: usdBase ? 0.63 : -0.66 })
  }
  if (path === '/forex/sessions') {
    const sym = search.get('symbol')
    if (sym && DEMO_FX.some(item => item === sym)) return jsonResponse({ sessions: demoFxSessions(sym, Number(search.get('from')), Number(search.get('to'))) })
    if (sym === DEMO_SYMBOL) {
      const scan = await loadScan(search.get('tf') || '1H', realFetch)
      return jsonResponse({ sessions: demoSessionsFromBars(scan.bars, Number(search.get('from')), Number(search.get('to'))) })
    }
    return jsonResponse({ sessions: [] })
  }
  if (path === '/forex/test-connection') return jsonResponse({ ok: true, message: 'Demo sample', provider: 'yahoo' })

  // Object-shaped endpoints touched on load.
  if (path === '/settings/config') return jsonResponse({ FOREX_PROVIDER: 'auto', OANDA_TOKEN: '', OANDA_ENVIRONMENT: 'practice' })
  if (path === '/env/config') return jsonResponse({})
  if (path === '/status') return jsonResponse({ phase: 'demo', running: false })
  if (path === '/vix/current') return noContent()
  if (path.startsWith('/wyckoff/')) return jsonResponse({ events: [] })
  if (path.startsWith('/profiles/')) return jsonResponse({})

  // Object-shaped endpoints whose consumers dereference fields — an empty
  // array here would crash the tab (ExecutionTab: config.plugins.map,
  // MyTradesTab: rollup.rows.length). Match each consumer's expected shape.
  if (path === '/execution/config') {
    return jsonResponse({
      version: 0,
      enabled: false,
      killed_at: '',
      plugins: [],
      max_dispatched: 0,
      dedup_window: '',
      symbol_map: {},
    })
  }
  if (path === '/backtest/readiness') return new Response('Backtesting requires a local server', { status: 503 })
  if (path.startsWith('/execution')) return jsonResponse({})
  if (path === '/marks/rollup') return jsonResponse({ by: '', since: '', rows: [] })
  // apiFetch() maps 204 → null; PaperTab's summary render is null-guarded.
  if (path === '/paper/summary') return noContent()

  // Unknown endpoints fail visibly rather than pretending a valid empty response.
  return jsonResponse({ error: 'Demo endpoint unavailable' }, 404)
}

export function installDemoApi(): void {
  const realFetch = window.fetch.bind(window)

  window.fetch = async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const raw = urlOf(input)
    let origin: string
    let pathname: string
    let search: URLSearchParams
    try {
      const u = new URL(raw, window.location.origin)
      origin = u.origin
      pathname = u.pathname
      search = u.searchParams
    } catch {
      return realFetch(input, init)
    }

    // Only intercept same-origin /api requests. Absolute cross-origin URLs
    // whose path happens to start with /api/ must reach their real host.
    if (origin !== window.location.origin || !pathname.startsWith('/api/')) {
      return realFetch(input, init)
    }

    const method = (init?.method || (input instanceof Request ? input.method : 'GET')).toUpperCase()
    if (method !== 'GET') {
      if (pathname === '/api/forex/test-connection') return jsonResponse({ ok: true, message: 'Demo sample', provider: 'yahoo' })
      // The onboarding scan POST expects JSON on 2xx but treats 503 as the
      // graceful "LLM unavailable — scan still completes" path. Use that.
      if (pathname === '/api/analysis/full') {
        return new Response('demo mode: AI interpretation unavailable', { status: 503 })
      }
      // Other mutations (PUT/POST/DELETE) are no-ops in the read-only demo.
      return noContent()
    }

    try {
      return await route(pathname.replace(/^\/api/, ''), search, realFetch)
    } catch (err) {
      // Surface as a 503 rather than a silent wrong-shape []: apiFetch()
      // throws on !ok, so consumers hit their existing error paths and the
      // failure is visible in DevTools instead of rendering a blank demo.
      console.warn('[demo] fixture route failed:', pathname, err)
      return new Response('demo fixture unavailable', { status: 503 })
    }
  }
}

export const DEMO_STATIC = import.meta.env.VITE_DEMO_STATIC === 'true'
