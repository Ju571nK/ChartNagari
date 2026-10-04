import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { IChartApi, ISeriesApi } from 'lightweight-charts'
import { forexGet, isMetal, isUSDPair, visibleSession, type DxyData, type ForexSession, type StrengthCurrency, type ForexStatus } from './forex'
import { DEMO_STATIC } from './demoApi'

export interface SourceMetadata {
  data_provider?: string
  provider_symbol?: string
  source_identity?: string
  data_proxy?: boolean
  suspended?: boolean
  suspension_reason?: string
}

export function SourceNote({ source }: { source: SourceMetadata }) {
  const { t } = useTranslation()
  const origin = source.data_provider
    ? `${source.data_provider.toUpperCase()}${source.provider_symbol ? ` · ${source.provider_symbol}` : ''}`
    : t('forex.sourceUnknown')
  return <span className="forex-source-note">
    {origin}{source.data_proxy && ` · ${t('forex.futuresProxy')}`}
    {source.suspended && <strong> · {t('forex.suspended')}: {t(`forex.suspension_${source.suspension_reason}`, { defaultValue: t('forex.suspension_source_mismatch') })}</strong>}
  </span>
}

function useForexData<T>(path: string | null, revision = 0) {
  const [state, setState] = useState<{ path: string; revision: number; data?: T; error?: string }>({ path: '', revision: -1 })
  useEffect(() => {
    if (!path) return
    let controller: AbortController | null = null
    const refresh = () => {
      controller?.abort()
      const request = new AbortController()
      controller = request
      void forexGet<T>(path, request.signal).then(data => {
      const value = data as Record<string, unknown> | null
      if (!value || (path.startsWith('strength') && !Array.isArray(value.currencies)) ||
        (path.startsWith('dxy') && !Array.isArray(value.bars)) ||
        (path === 'status' && (typeof value.provider !== 'string' || typeof value.volume_quality !== 'string'))) {
        throw new Error('Invalid FX data response')
      }
      if (!request.signal.aborted) setState({ path, revision, data })
    })
      .catch((error: Error) => { if (!request.signal.aborted) setState({ path, revision, error: error.message }) })
    }
    refresh()
    const timer = window.setInterval(refresh, 30_000)
    return () => { controller?.abort(); window.clearInterval(timer) }
  }, [path, revision])
  return path && state.path === path && state.revision === revision ? state : { path: path ?? '', revision }
}

export function MetalSourceNote({ symbol }: { symbol: string }) {
  const { t } = useTranslation()
  const state = useForexData<ForexStatus>(isMetal(symbol) ? 'status' : null)
  if (!isMetal(symbol)) return null
  if (state.error) return <p className="forex-proxy" role="alert">{t('forex.providerStatusError')}: {state.error}</p>
  if (!state.data) return <p role="status">{t('forex.providerLoading')}</p>
  return state.data.provider === 'yahoo' ? <p className="forex-proxy">{t('forex.metalProxy')}</p> : null
}

export function StrengthMeter({ compact = false, revision = 0 }: { compact?: boolean; revision?: number }) {
  const { t } = useTranslation()
  const state = useForexData<{ currencies: StrengthCurrency[] }>('strength?tf=4H&lookback=20', revision)
  const ranked = useMemo(() => [...(Array.isArray(state.data?.currencies) ? state.data.currencies : [])].sort((a, b) => {
    if (a.coverage !== b.coverage) return a.coverage === 'ok' ? -1 : 1
    return (b.value ?? -Infinity) - (a.value ?? -Infinity)
  }), [state.data])
  return <section className={`forex-panel${compact ? ' forex-panel-compact' : ''}`} aria-label={t('forex.strength')}>
    <h3>{t('forex.strength')}{DEMO_STATIC && <small className="demo-sample"> · {t('forex.demoSample')}</small>}</h3>
    {state.error && <p role="alert">{t('forex.loadError')}: {state.error}</p>}
    {!state.error && !state.data && <p role="status">{t('loading')}</p>}
    {state.data && ranked.length === 0 && <p>{t('forex.noStrength')}</p>}
    {ranked.map(item => {
      const low = item.coverage === 'low' || item.pairs < 2 || item.value === null || !Number.isFinite(item.value)
      return <div className={`strength-row${low ? ' strength-low' : ''}`} key={item.currency}>
        <span>{item.currency}</span>
        <div className="strength-track"><div style={{ width: low ? 0 : `${Math.min(100, Math.abs(item.value!))}%`, marginLeft: !low && item.value! < 0 ? 'auto' : undefined }} /></div>
        <strong>{low ? '—' : item.value!.toFixed(1)}</strong>
        {low && <small title={t('forex.lowCoverage')}>{t('forex.lowCoverageShort')}</small>}
      </div>
    })}
    {ranked.some(item => item.coverage === 'low' || item.pairs < 2) && <p className="forex-note">{t('forex.lowCoverage')}</p>}
  </section>
}

export function DxyPanel({ symbol, revision = 0 }: { symbol: string; revision?: number }) {
  const { t } = useTranslation()
  const state = useForexData<DxyData>(isUSDPair(symbol) ? `dxy?symbol=${encodeURIComponent(symbol)}` : null, revision)
  if (!isUSDPair(symbol)) return null
  const bars = state.data?.bars ?? []
  const values = bars.map(bar => bar.close).filter(Number.isFinite)
  const lo = Math.min(...values), hi = Math.max(...values)
  const points = values.map((value, index) => `${(index / Math.max(values.length - 1, 1)) * 100},${35 - ((value - lo) / (hi - lo || 1)) * 30}`).join(' ')
  return <section className="forex-panel" aria-label={t('forex.dxy')}>
    <h3>{t('forex.dxy')}{DEMO_STATIC && <small className="demo-sample"> · {t('forex.demoSample')}</small>}</h3>
    {state.error && <p role="alert">{t('forex.loadError')}: {state.error}</p>}
    {!state.error && !state.data && <p role="status">{t('loading')}</p>}
    {state.data?.state === 'disabled' && <p>{t('forex.dxyDisabled')}</p>}
    {state.data && state.data.state !== 'disabled' && <>
      {values.length >= 2 ? <svg className="dxy-sparkline" viewBox="0 0 100 40" preserveAspectRatio="none" role="img" aria-label={t('forex.dxyChart')}><polyline points={points} /></svg> : <p>{t('forex.noDxy')}</p>}
      <div className="dxy-correlations"><span>{t('forex.correlation20')}: {state.data.correlation_20 == null ? '—' : state.data.correlation_20.toFixed(2)}</span><span>{t('forex.correlation60')}: {state.data.correlation_60 == null ? '—' : state.data.correlation_60.toFixed(2)}</span></div>
    </>}
  </section>
}

export type BoxPosition = { name: string; left: number; width: number; top: number; height: number; highY?: number; lowY?: number }
export function interpolatedTimeCoordinate(times: number[], target: number, coordinate: (time: number) => number | null): number | null {
  if (times.length < 2) return times.length ? coordinate(times[0]) : null
  let right = times.findIndex(time => time >= target)
  if (right < 0) right = times.length - 1
  if (right === 0) right = 1
  const before = times[right - 1], after = times[right]
  const x1 = coordinate(before), x2 = coordinate(after)
  if (x1 == null || x2 == null) return null
  return x1 + (target - before) / (after - before) * (x2 - x1)
}
export function sessionPositions(sessions: ForexSession[], x: (time: number) => number | null, y: (price: number) => number | null): BoxPosition[] {
  return sessions.flatMap(session => {
    const x1 = x(session.start), x2 = x(session.end), y1 = y(session.high), y2 = y(session.low)
    if ([x1, x2, y1, y2].some(v => v == null || !Number.isFinite(v))) return []
    const left = Math.min(x1!, x2!), top = Math.min(y1!, y2!)
    return [{ name: session.name, left, width: Math.abs(x2! - x1!), top, height: Math.abs(y2! - y1!),
      highY: session.asian_high == null ? undefined : y(session.asian_high) ?? undefined,
      lowY: session.asian_low == null ? undefined : y(session.asian_low) ?? undefined }]
  })
}

export function SessionOverlay({ symbol, timeframe, bars, chart, series, enabled, revision = 0 }: {
  symbol: string; timeframe: string; bars: { time: number }[]; chart: IChartApi | null; series: ISeriesApi<'Candlestick'> | null; enabled: boolean; revision?: number
}) {
  const { t } = useTranslation()
  const from = bars[0]?.time, to = bars[bars.length - 1]?.time
  const path = enabled && visibleSession(timeframe) && from != null && to != null && to > from
    ? `sessions?symbol=${encodeURIComponent(symbol)}&tf=${timeframe}&from=${from}&to=${to}` : null
  const [sessionState, setSessionState] = useState<{ path: string; data?: { sessions: ForexSession[] }; error?: string }>({ path: '' })
  useEffect(() => {
    if (!path || from == null || to == null) return
    const controller = new AbortController()
    const spans: Array<[number, number]> = []
    for (let cursor = from; cursor < to; cursor += 30 * 86400) spans.push([cursor, Math.min(to, cursor + 30 * 86400)])
    Promise.all(spans.map(([a, b]) => forexGet<{ sessions: ForexSession[] }>(`sessions?symbol=${encodeURIComponent(symbol)}&tf=${timeframe}&from=${a}&to=${b}`, controller.signal)))
      .then(parts => {
        if (controller.signal.aborted) return
        if (parts.some(part => !Array.isArray(part.sessions))) throw new Error('Invalid session response')
        const unique = new Map<string, ForexSession>()
        parts.flatMap(part => part.sessions).forEach(session => unique.set(`${session.name}-${session.start}`, session))
        setSessionState({ path, data: { sessions: [...unique.values()] } })
      })
      .catch((error: Error) => { if (!controller.signal.aborted) setSessionState({ path, error: error.message }) })
    return () => controller.abort()
  }, [path, symbol, timeframe, from, to, revision])
  const state = path && sessionState.path === path ? sessionState : { path: path ?? '' }
  const [positions, setPositions] = useState<BoxPosition[]>([])
  useEffect(() => {
    if (!chart || !series || !Array.isArray(state.data?.sessions) || !path) { setPositions([]); return }
    let frame = 0
    let previous = ''
    const times = bars.map(bar => bar.time)
    const update = () => {
      const next = sessionPositions(state.data!.sessions, time => interpolatedTimeCoordinate(times, time, t => chart.timeScale().timeToCoordinate(t as never)), price => series.priceToCoordinate(price))
      const signature = JSON.stringify(next)
      if (signature !== previous) { previous = signature; setPositions(next) }
      frame = requestAnimationFrame(update)
    }
    frame = requestAnimationFrame(update)
    return () => cancelAnimationFrame(frame)
  }, [chart, series, state.data, path, bars])
  if (!enabled || !visibleSession(timeframe)) return null
  return <>
    {state.error && <div className="session-error" role="alert">{t('forex.sessionsError')}: {state.error}</div>}
    <div className="session-overlay" aria-label={t('forex.sessions')}>
      {positions.map((box, index) => <div key={`${box.name}-${index}`} className={`session-box session-${box.name}`} style={{ left: box.left, width: box.width, top: box.top, height: Math.max(box.height, 2) }} title={t(`forex.session_${box.name}`)}>
        <span>{t(`forex.session_${box.name}`)}</span>
        {box.highY != null && <i className="asian-range-line" style={{ top: box.highY - box.top }} />}
        {box.lowY != null && <i className="asian-range-line" style={{ top: box.lowY - box.top }} />}
      </div>)}
    </div>
  </>
}
