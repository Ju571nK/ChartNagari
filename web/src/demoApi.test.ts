import { describe, it, expect, vi, afterEach } from 'vitest'
// Real shipped fixtures (resolveJsonModule) — validated in the last test.
import shipped1W from '../public/demo/scan-1W.json'
import shipped1D from '../public/demo/scan-1D.json'
import shipped4H from '../public/demo/scan-4H.json'
import shipped1H from '../public/demo/scan-1H.json'

// demoApi keeps module-level state (fixture cache, currentTf/currentBars), so
// every test gets a fresh copy via vi.resetModules() + dynamic import.

interface Bar {
  time: number
  open: number
  high: number
  low: number
  close: number
  volume: number
}

interface Sig {
  timeframe?: string
  rule: string
  direction: string
  score: number
  message?: string
  zone_low?: number
  zone_high?: number
}

interface Scan {
  symbol: string
  timeframe: string
  bars: Bar[]
  signals: Sig[]
}

function bar(time: number, low = 10, high = 20): Bar {
  return { time, open: 15, high, low, close: 15, volume: 100 }
}

function scan(tf: string, bars: Bar[], signals: Sig[]): Scan {
  return { symbol: 'DEMO_BTC', timeframe: tf, bars, signals }
}

// 11 flat bars (times 1000..1010) — snap logic keeps the anchor index on ties.
const flatBars = Array.from({ length: 11 }, (_, i) => bar(1000 + i))

const ORIGIN = window.location.origin

// Installs a fresh demoApi over a mocked "real" fetch that serves fixtures
// for /demo/scan-{TF}.json and records every passthrough call.
async function install(fixtures: Record<string, Scan>, opts?: { failFixtures?: boolean }) {
  vi.resetModules()
  const realFetch = vi.fn(async (input: RequestInfo | URL) => {
    const url = input instanceof Request ? input.url : String(input)
    const m = url.match(/demo\/scan-([^./]+)\.json$/)
    if (m) {
      if (opts?.failFixtures) throw new Error('fixture load failed')
      const fx = fixtures[m[1]]
      if (fx) return new Response(JSON.stringify(fx))
      throw new Error('no fixture for ' + m[1])
    }
    return new Response(JSON.stringify({ passthrough: true }))
  })
  window.fetch = realFetch as unknown as typeof fetch
  const mod = await import('./demoApi')
  mod.installDemoApi()
  return { realFetch, mod }
}

afterEach(() => {
  vi.unstubAllEnvs()
  vi.restoreAllMocks()
})

describe('route: core endpoints', () => {
  // ── 1. /symbols → single enabled DEMO_BTC ────────────────────────────────
  it('serves a single enabled DEMO_BTC from /api/symbols', async () => {
    await install({ '1D': scan('1D', flatBars, []) })
    const res = await window.fetch('/api/symbols')
    expect(res.status).toBe(200)
    expect(await res.json()).toEqual([
      { symbol: 'DEMO_BTC', enabled: true, type: 'crypto', exchange: 'binance' },
    ])
  })

  // ── 2. /demo/scan honors the timeframe query param ───────────────────────
  it('serves the full captured scan for the requested timeframe', async () => {
    const fx4h = scan('4H', flatBars, [{ rule: 'r', direction: 'LONG', score: 80 }])
    const { realFetch } = await install({ '4H': fx4h })
    const res = await window.fetch('/api/demo/scan?symbol=DEMO_BTC&timeframe=4H')
    expect(await res.json()).toEqual(fx4h)
    expect(realFetch).toHaveBeenCalledWith('/demo/scan-4H.json')
  })

  // ── 3. /demo/scan without timeframe defaults to 1D ───────────────────────
  it('defaults /demo/scan to the 1D fixture when timeframe is missing', async () => {
    const fx = scan('1D', flatBars, [])
    const { realFetch } = await install({ '1D': fx })
    const res = await window.fetch('/api/demo/scan')
    expect(await res.json()).toEqual(fx)
    expect(realFetch).toHaveBeenCalledWith('/demo/scan-1D.json')
  })

  // ── 4. loadScan whitelist: unknown TF falls back to 1D ───────────────────
  it('falls back to the 1D fixture for a timeframe outside the whitelist', async () => {
    const { realFetch } = await install({ '1D': scan('1D', flatBars, []) })
    const res = await window.fetch('/api/demo/scan?timeframe=15m')
    expect(res.status).toBe(200)
    expect(realFetch).toHaveBeenCalledWith('/demo/scan-1D.json')
    expect(realFetch).not.toHaveBeenCalledWith('/demo/scan-15m.json')
  })

  // ── 5. loadScan caches fixtures per timeframe ────────────────────────────
  it('fetches each fixture only once (cache hit on repeat requests)', async () => {
    const { realFetch } = await install({ '1D': scan('1D', flatBars, []) })
    await window.fetch('/api/demo/scan?timeframe=1D')
    await window.fetch('/api/demo/scan?timeframe=1D')
    await window.fetch('/api/ohlcv/DEMO_BTC/1D')
    expect(realFetch).toHaveBeenCalledTimes(1)
  })

  // ── 6. /ohlcv/DEMO_BTC/{tf} → captured candles ───────────────────────────
  it('serves captured bars from /api/ohlcv for the demo symbol', async () => {
    const { realFetch } = await install({ '1H': scan('1H', flatBars, []) })
    const res = await window.fetch('/api/ohlcv/DEMO_BTC/1H')
    expect(await res.json()).toEqual(flatBars)
    expect(realFetch).toHaveBeenCalledWith('/demo/scan-1H.json')
  })

  // ── 7. /ohlcv for any other symbol → empty, no fixture load ──────────────
  it('returns an empty array for non-demo ohlcv symbols without loading fixtures', async () => {
    const { realFetch } = await install({ '1D': scan('1D', flatBars, []) })
    const res = await window.fetch('/api/ohlcv/AAPL/1D')
    expect(await res.json()).toEqual([])
    expect(realFetch).not.toHaveBeenCalled()
  })

  // ── 8. /signals for another symbol → empty ───────────────────────────────
  it('returns an empty array from /api/signals for non-demo symbols', async () => {
    const { realFetch } = await install({ '1D': scan('1D', flatBars, []) })
    const res = await window.fetch('/api/signals?symbol=AAPL')
    expect(await res.json()).toEqual([])
    expect(realFetch).not.toHaveBeenCalled()
  })
})

describe('placeSignals via /api/signals', () => {
  // ── 9. LONG snaps to the local low near the anchor ───────────────────────
  it('snaps a LONG marker to the local low near its anchor', async () => {
    // k=1 → anchor idx 7 (round(0.7*10)), window 3..10; the lowest low is idx 4.
    const bars = flatBars.map((b, i) => (i === 4 ? bar(b.time, 1) : b))
    const sig: Sig = { rule: 'ob', direction: 'LONG', score: 90, message: 'm', zone_low: 1, zone_high: 2 }
    await install({ '1D': scan('1D', bars, [sig]) })
    await window.fetch('/api/ohlcv/DEMO_BTC/1D')
    const placed = await (await window.fetch('/api/signals?symbol=DEMO_BTC')).json()
    expect(placed).toEqual([
      {
        symbol: 'DEMO_BTC',
        timeframe: '1D',
        time: 1004,
        direction: 'LONG',
        rule: 'ob',
        score: 90,
        message: 'm',
        ai_interpretation: '',
        zone_low: 1,
        zone_high: 2,
      },
    ])
  })

  // ── 10. SHORT snaps to the local high near the anchor ────────────────────
  it('snaps a SHORT marker to the local high near its anchor', async () => {
    const bars = flatBars.map((b, i) => (i === 9 ? bar(b.time, 10, 99) : b))
    await install({ '1D': scan('1D', bars, [{ rule: 'fvg', direction: 'SHORT', score: 70 }]) })
    await window.fetch('/api/ohlcv/DEMO_BTC/1D')
    const placed = await (await window.fetch('/api/signals?symbol=DEMO_BTC')).json()
    expect(placed).toHaveLength(1)
    expect(placed[0].time).toBe(1009)
    expect(placed[0].direction).toBe('SHORT')
    expect(placed[0].message).toBe('') // message ?? '' default
  })

  // ── 11. distribution + non-directional filter ────────────────────────────
  it('spreads directional signals across later bars and drops non-directional ones', async () => {
    const sigs: Sig[] = [
      { rule: 'a', direction: 'LONG', score: 1, timeframe: '1W' },
      { rule: 'b', direction: 'SHORT', score: 2 },
      { rule: 'c', direction: 'NEUTRAL', score: 3 },
    ]
    await install({ '1D': scan('1D', flatBars, sigs) })
    await window.fetch('/api/ohlcv/DEMO_BTC/1D')
    const placed = await (await window.fetch('/api/signals?symbol=DEMO_BTC')).json()
    // k=2 → anchors at round(0.575*10)=6 and round(0.825*10)=8; flat bars keep them.
    expect(placed).toHaveLength(2)
    expect(placed.map((p: { time: number }) => p.time)).toEqual([1006, 1008])
    expect(placed[0].timeframe).toBe('1W') // explicit timeframe kept
    expect(placed[1].timeframe).toBe('1D') // missing timeframe falls back to currentTf
  })

  // ── 12. /signals without prior /ohlcv → bars derived from default 1D scan ─
  it('places markers from the default 1D fixture when /signals arrives first', async () => {
    await install({ '1D': scan('1D', flatBars, [{ rule: 'r', direction: 'LONG', score: 5 }]) })
    const placed = await (await window.fetch('/api/signals?symbol=DEMO_BTC')).json()
    // No shared mutable bar state: bars come straight from the cached fixture.
    expect(placed).toHaveLength(1)
    expect(placed[0].rule).toBe('r')
    expect(flatBars.map((b) => b.time)).toContain(placed[0].time)
  })

  // ── 12b. transient fixture failure is retried (no negative caching) ──────
  it('retries fixture load after a transient failure', async () => {
    let calls = 0
    vi.resetModules()
    const realFetch = vi.fn(async () => {
      calls++
      if (calls === 1) throw new Error('network')
      return new Response(JSON.stringify(scan('1D', flatBars, [])))
    })
    window.fetch = realFetch as unknown as typeof fetch
    const mod = await import('./demoApi')
    mod.installDemoApi()
    // First request fails → surfaced as a 503 (never a silent 200 []).
    expect((await window.fetch('/api/demo/scan?timeframe=1D')).status).toBe(503)
    // Rejected promise must be evicted: second request refetches and succeeds.
    const second = await (await window.fetch('/api/demo/scan?timeframe=1D')).json()
    expect(second.symbol).toBe('DEMO_BTC')
    expect(calls).toBe(2)
  })
})

describe('route: object endpoints and fallbacks', () => {
  // ── 13. object-shaped endpoints touched on load ──────────────────────────
  it('serves object-shaped endpoints and 204 for /vix/current', async () => {
    await install({ '1D': scan('1D', flatBars, []) })
    expect(await (await window.fetch('/api/settings/config')).json()).toEqual({})
    expect(await (await window.fetch('/api/env/config')).json()).toEqual({})
    expect(await (await window.fetch('/api/status')).json()).toEqual({ phase: 'demo', running: false })
    expect(await (await window.fetch('/api/wyckoff/DEMO_BTC/1D')).json()).toEqual({ events: [] })
    expect(await (await window.fetch('/api/profiles/DEMO_BTC')).json()).toEqual({})
    // Consumers dereference these shapes (ExecutionTab: config.plugins.map,
    // MyTradesTab: rollup.rows.length) — assert the crash-proof fields exist.
    const execConfig = await (await window.fetch('/api/execution/config')).json()
    expect(execConfig.plugins).toEqual([])
    expect(execConfig.enabled).toBe(false)
    expect(await (await window.fetch('/api/marks/rollup?by=rule')).json()).toEqual({ by: '', since: '', rows: [] })
    // apiFetch maps 204 → null; PaperTab's summary render is null-guarded.
    expect((await window.fetch('/api/paper/summary')).status).toBe(204)
    const vix = await window.fetch('/api/vix/current')
    expect(vix.status).toBe(204)
    expect(await vix.text()).toBe('')
  })

  // ── 13b. onboarding scan POST → graceful 503 (LLM-unavailable path) ──────
  it('returns 503 for POST /api/analysis/full so onboarding completes without AI', async () => {
    await install({ '1D': scan('1D', flatBars, []) })
    const res = await window.fetch('/api/analysis/full', { method: 'POST', body: '{}' })
    expect(res.status).toBe(503)
  })

  // ── 13c. cross-origin /api requests are NOT intercepted ──────────────────
  it('passes cross-origin /api URLs through to the real fetch', async () => {
    const { realFetch } = await install({ '1D': scan('1D', flatBars, []) })
    const res = await window.fetch('https://broker.example/api/orders')
    expect(realFetch).toHaveBeenCalledWith('https://broker.example/api/orders', undefined)
    expect((await res.json()).passthrough).toBe(true)
  })

  // ── 14. anything else → empty array ──────────────────────────────────────
  it('serves an empty array for unknown GET endpoints', async () => {
    await install({ '1D': scan('1D', flatBars, []) })
    const res = await window.fetch('/api/some/unknown/endpoint')
    expect(res.status).toBe(200)
    expect(await res.json()).toEqual([])
  })
})

describe('installDemoApi: interception behavior', () => {
  // ── 15. non-/api URLs (and unparseable ones) pass through ────────────────
  it('passes non-/api and unparseable URLs through to the real fetch', async () => {
    const { realFetch } = await install({ '1D': scan('1D', flatBars, []) })
    await window.fetch('/assets/logo.svg')
    expect(realFetch).toHaveBeenCalledWith('/assets/logo.svg', undefined)
    await window.fetch('http://') // new URL() throws → passthrough branch
    expect(realFetch).toHaveBeenCalledWith('http://', undefined)
  })

  // ── 16. mutations are read-only no-ops ───────────────────────────────────
  it('answers non-GET /api requests with 204 and never hits the backend', async () => {
    const { realFetch } = await install({ '1D': scan('1D', flatBars, []) })
    const res = await window.fetch('/api/symbols', { method: 'POST', body: '{}' })
    expect(res.status).toBe(204)
    const del = await window.fetch(new Request(ORIGIN + '/api/symbols/1', { method: 'DELETE' }))
    expect(del.status).toBe(204)
    expect(realFetch).not.toHaveBeenCalled()
  })

  // ── 17. urlOf handles string / URL / Request inputs ──────────────────────
  it('routes string, URL, and Request inputs identically', async () => {
    await install({ '1D': scan('1D', flatBars, []) })
    const expected = [{ symbol: 'DEMO_BTC', enabled: true, type: 'crypto', exchange: 'binance' }]
    expect(await (await window.fetch('/api/symbols')).json()).toEqual(expected)
    expect(await (await window.fetch(new URL('/api/symbols', ORIGIN))).json()).toEqual(expected)
    expect(await (await window.fetch(new Request(ORIGIN + '/api/symbols'))).json()).toEqual(expected)
  })

  // ── 18. route errors degrade to an empty array ───────────────────────────
  it('returns a diagnosable 503 when fixture loading fails', async () => {
    await install({}, { failFixtures: true })
    const res = await window.fetch('/api/demo/scan?timeframe=1D')
    // Not a silent 200 [] — consumers hit their error paths and the failure
    // is visible in DevTools instead of rendering a blank demo.
    expect(res.status).toBe(503)
  })

  // ── 19. DEMO_STATIC reflects VITE_DEMO_STATIC ────────────────────────────
  it('exports DEMO_STATIC=true only when VITE_DEMO_STATIC === "true"', async () => {
    vi.resetModules()
    expect((await import('./demoApi')).DEMO_STATIC).toBe(false)
    vi.stubEnv('VITE_DEMO_STATIC', 'true')
    vi.resetModules()
    expect((await import('./demoApi')).DEMO_STATIC).toBe(true)
  })
})

describe('shipped demo fixtures', () => {
  // ── 20. real public/demo/*.json match the DemoScan contract ──────────────
  it('every scan-{TF}.json fixture has DEMO_BTC bars and well-formed signals', () => {
    const shipped: Record<string, Scan> = {
      '1W': shipped1W,
      '1D': shipped1D,
      '4H': shipped4H,
      '1H': shipped1H,
    }
    for (const tf of ['1W', '1D', '4H', '1H']) {
      const fx = shipped[tf]
      expect(fx.symbol).toBe('DEMO_BTC')
      expect(fx.timeframe).toBe(tf)
      expect(fx.bars.length).toBeGreaterThan(0)
      for (const b of fx.bars) {
        expect(Number.isFinite(b.time)).toBe(true)
        expect(b.low).toBeLessThanOrEqual(b.high)
      }
      expect(fx.signals.length).toBeGreaterThan(0)
      for (const s of fx.signals) {
        expect(typeof s.rule).toBe('string')
        // NEUTRAL is valid detector output (e.g. ict_kill_zone); the chart
        // layer filters it out of markers, but fixtures may carry it.
        expect(['LONG', 'SHORT', 'NEUTRAL']).toContain(s.direction)
      }
    }
  })
})
