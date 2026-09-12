import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

export interface Readiness { symbol: string; timeframe: string; bars: number; minimum_bars: number; from: number | null; to: number | null; ready: boolean }
export function useBacktestPreparation(symbol: string, timeframe: string, revision: number) {
  const key = JSON.stringify([symbol, timeframe, revision])
  const [state, setState] = useState<{ key: string; data?: Readiness; error?: string }>({ key: '' })
  useEffect(() => {
    const controller = new AbortController()
    if (!symbol) return () => controller.abort()
    setState({ key })
    fetch(`/api/backtest/readiness?symbol=${encodeURIComponent(symbol)}&timeframe=${encodeURIComponent(timeframe)}`, { signal: controller.signal })
      .then(async response => {
        if (!response.ok) throw new Error(`HTTP ${response.status}`)
        const data: Readiness = await response.json()
        if (!data || data.symbol !== symbol || data.timeframe !== timeframe || !Number.isInteger(data.bars) || !Number.isInteger(data.minimum_bars) || typeof data.ready !== 'boolean') throw new Error('Invalid backtest preparation response')
        if (!controller.signal.aborted) setState({ key, data })
      })
      .catch((error: Error) => { if (!controller.signal.aborted) setState({ key, error: error.message }) })
    return () => controller.abort()
  }, [key, symbol, timeframe])
  return state.key === key ? state : { key }
}

export function BacktestPreparation({ data, error, symbol, onRetry }: { data?: Readiness; error?: string; symbol: string; onRetry: () => void }) {
  const { t, i18n } = useTranslation()
  const format = (time: number) => new Date(time * 1000).toLocaleString(i18n.language, { timeZoneName: 'short' })
  const state = !symbol ? 'select' : error ? 'error' : !data ? 'loading' : data.bars === 0 ? 'empty' : !data.ready ? 'insufficient' : 'ready'
  return <section className="market-data-status" aria-label={t('backtestPrep.title')}>
    <p role={error ? 'alert' : 'status'}>{t(`backtestPrep.${state}`)}</p>
    {data && <>
      <p>{t('backtestPrep.count', { count: data.bars, minimum: data.minimum_bars })}</p>
      {data.from !== null && data.to !== null && <p>{t('backtestPrep.range')}: {format(data.from)} — {format(data.to)}</p>}
    </>}
    <p>{t('backtestPrep.multipliers')}</p>
    <p>{t('backtestPrep.caution')}</p>
    {symbol && (data || error) && <button onClick={onRetry}>{t('workspace.retry')}</button>}
  </section>
}
