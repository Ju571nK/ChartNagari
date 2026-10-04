import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useUXCopy } from './uxCopy'
import { forexGet, isForexSymbol, isMetal, type ForexStatus } from './forex'

export interface PriceBar {
  time: number
  open: number
  high: number
  low: number
  close: number
  volume: number
}

type DataState = {
  key: string
  phase: 'idle' | 'loading' | 'empty' | 'ready' | 'error'
  bars: PriceBar[]
  checkedAt?: number
  error?: string
}
const noBars: PriceBar[] = []

// The key guards the render before effect cleanup as well as late responses.
export function useMarketData(symbol: string, timeframe: string, revision = 0) {
  const key = JSON.stringify([symbol, timeframe, revision])
  const [state, setState] = useState<DataState>({ key: '', phase: 'idle', bars: noBars })
  useEffect(() => {
    const controller = new AbortController()
    if (!symbol) return () => controller.abort()
    setState({ key, phase: 'loading', bars: noBars })
    fetch(`/api/ohlcv/${encodeURIComponent(symbol)}/${encodeURIComponent(timeframe)}?limit=200`, { signal: controller.signal })
      .then(async response => {
        if (!response.ok) throw new Error(`HTTP ${response.status}: ${response.statusText}`)
        const bars: PriceBar[] = await response.json()
        if (!Array.isArray(bars)) throw new Error('Invalid price data response')
        if (!controller.signal.aborted) {
          setState({ key, phase: bars.length ? 'ready' : 'empty', bars, checkedAt: Date.now() })
        }
      })
      .catch((error: Error) => {
        if (!controller.signal.aborted) setState({ key, phase: 'error', bars: noBars, error: error.message })
      })
    return () => controller.abort()
  }, [key, symbol, timeframe])
  if (!symbol) return { key, phase: 'idle' as const, bars: noBars }
  return state.key === key ? state : { key, phase: 'loading' as const, bars: noBars }
}

export function MarketDataStatus({ data, symbol, timeframe, onRetry, onManage }: {
  data: DataState
  symbol: string
  timeframe: string
  onRetry: () => void
  onManage: () => void
}) {
  const { t, i18n } = useTranslation()
  const ux = useUXCopy()
  const formatTime = (value: number) => new Date(value).toLocaleString(i18n.language, { timeZoneName: 'short' })
  const latest = data.bars.length ? Math.max(...data.bars.map(bar => bar.time)) : null
  const [fx, setFx] = useState<{ status?: ForexStatus; error?: string }>({})
  useEffect(() => {
    if (!isForexSymbol(symbol)) { setFx({}); return }
    const controller = new AbortController()
    setFx({})
    forexGet<ForexStatus>('status', controller.signal).then(status => {
      if (typeof status?.provider !== 'string' || typeof status?.volume_quality !== 'string') throw new Error('Invalid FX status response')
      setFx({ status })
    })
      .catch((error: Error) => { if (!controller.signal.aborted) setFx({ error: error.message }) })
    return () => controller.abort()
  }, [symbol, data.checkedAt])
  return <section className="market-data-status" aria-label={t('marketData.title')}>
    <div role={data.phase === 'error' ? 'alert' : 'status'} aria-live="polite">
      <strong>{symbol ? `${symbol} · ${timeframe} — ` : ''}{t(`marketData.${data.phase}`)}</strong>
      {data.phase === 'empty' && <p>{t('marketData.emptyHelp')}</p>}
      {data.phase === 'error' && <p>{t('marketData.errorHelp')}</p>}
    </div>
    {isForexSymbol(symbol) && <div className="forex-data-notes">
      {fx.error && <p role="alert">{t('forex.providerStatusError')}: {fx.error}</p>}
      {!fx.error && !fx.status && <p role="status">{t('forex.providerLoading')}</p>}
      {fx.status && <>
        <p>{t('forex.provider')}: {fx.status.provider.toUpperCase()} · {t(`forex.volume_${fx.status.volume_quality}`)}</p>
        {isMetal(symbol) && fx.status.provider === 'yahoo' && <p className="forex-proxy">{t('forex.metalProxy')}</p>}
        {fx.status.state !== 'ready' && <p role="alert">{t(`forex.status_${fx.status.state}`)}{fx.status.message ? `: ${fx.status.message}` : ''}</p>}
        {fx.status.provider_changed && <p role="status">{t('forex.rebackfill')}</p>}
      </>}
    </div>}
    <details open={data.phase !== 'ready'}><summary>{ux.details}{latest !== null ? ` · ${t('marketData.lastCandle')}: ${formatTime(latest * 1000)}` : ''}</summary>
      {data.phase === 'ready' && <p>{t('marketData.count', { count: data.bars.length })}</p>}
      {data.checkedAt && <p>{t('marketData.checkedAt')}: {formatTime(data.checkedAt)}</p>}
      {(data.phase === 'ready' || data.phase === 'empty') && <p className="market-data-note">{t('marketData.timeNote')}</p>}
    </details>
    {data.error && <details><summary>{t('marketData.details')}</summary><p>{data.error}</p></details>}
    <div className="market-data-actions">
      {symbol && <button onClick={onRetry} disabled={data.phase === 'loading'}>{data.phase === 'ready' ? ux.refresh : t('workspace.retry')}</button>}
      {(data.phase === 'empty' || data.phase === 'idle' || data.phase === 'error') && <button onClick={onManage}>{t('workspace.addSymbols')}</button>}
    </div>
  </section>
}
