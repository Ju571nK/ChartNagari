import { act, fireEvent, render, renderHook, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MarketDataStatus, useMarketData } from './MarketDataStatus'
import i18n from './i18n'

const bars = [{ time: 1700000000, open: 10, high: 12, low: 9, close: 11, volume: 100 }]
const response = (data: unknown, ok = true) => ({ ok, status: ok ? 200 : 503, statusText: 'Unavailable', json: async () => data }) as Response
beforeEach(async () => { await i18n.changeLanguage('en') })
afterEach(() => { vi.restoreAllMocks() })

function Panel() {
  const data = useMarketData('SPCX', '1H')
  return <MarketDataStatus data={data} symbol="SPCX" timeframe="1H" onRetry={() => {}} onManage={() => {}} />
}

describe('Instrument data status', () => {
  it('distinguishes loading and empty data with a recovery action', async () => {
    let resolve!: (response: Response) => void
    vi.spyOn(globalThis, 'fetch').mockImplementation(() => new Promise(r => { resolve = r }))
    render(<Panel />)
    expect(screen.getByRole('status')).toHaveTextContent('Loading price data')
    await act(async () => resolve(response([])))
    expect(screen.getByRole('status')).toHaveTextContent('No stored candles')
    expect(screen.getByRole('button', { name: /Manage instruments/ })).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('shows request failure separately from empty data', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(response(null, false))
    render(<Panel />)
    expect(await screen.findByRole('alert')).toHaveTextContent('Price data request failed')
    expect(screen.queryByText('No stored candles')).not.toBeInTheDocument()
  })

  it('reports loaded count and candle time without declaring old data unhealthy', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(response(bars))
    render(<Panel />)
    expect(await screen.findByText(/1 candles loaded/)).toBeInTheDocument()
    expect(screen.getByText(/Last candle start/).tagName).toBe('SUMMARY')
    expect(screen.getByText(/1 candles loaded/).closest('details')).not.toHaveAttribute('open')
    expect(screen.getByRole('button', { name: 'Refresh' })).toBeInTheDocument()
    expect(screen.getByText(/Collection time is not recorded/)).toBeInTheDocument()
    expect(screen.getByText(/Checked in this browser/)).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('aborts and ignores late responses after a symbol or timeframe change', async () => {
    const pending: Array<(response: Response) => void> = []
    const requests: AbortSignal[] = []
    vi.spyOn(globalThis, 'fetch').mockImplementation((_input, options) => {
      requests.push(options!.signal as AbortSignal)
      return new Promise(resolve => pending.push(resolve))
    })
    const { result, rerender } = renderHook(({ symbol, tf }) => useMarketData(symbol, tf), { initialProps: { symbol: 'BTCUSDT', tf: '1H' } })
    rerender({ symbol: 'SPCX', tf: '1D' })
    expect(requests[0].aborted).toBe(true)
    expect(result.current.bars).toEqual([])
    await act(async () => pending[1](response(bars)))
    await act(async () => pending[0](response([{ ...bars[0], close: 999 }])))
    expect(result.current.bars[0].close).toBe(11)
  })

  it('clears already loaded metadata immediately on selection change and retries errors', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(response(bars)).mockResolvedValueOnce(response(null, false)).mockResolvedValueOnce(response([]))
    const { result, rerender } = renderHook(({ symbol, revision }) => useMarketData(symbol, '1H', revision), { initialProps: { symbol: 'SPCX', revision: 0 } })
    await waitFor(() => expect(result.current.phase).toBe('ready'))
    rerender({ symbol: 'TSLA', revision: 0 })
    expect(result.current.bars).toEqual([])
    expect(result.current.checkedAt).toBeUndefined()
    await waitFor(() => expect(result.current.phase).toBe('error'))
    rerender({ symbol: 'TSLA', revision: 1 })
    await waitFor(() => expect(result.current.phase).toBe('empty'))
  })

  it.each(['en', 'ko', 'ja'])('provides translated actions for %s', async language => {
    await i18n.changeLanguage(language)
    const onManage = vi.fn()
    render(<MarketDataStatus data={{ key: '', phase: 'idle', bars: [] }} symbol="" timeframe="1H" onRetry={() => {}} onManage={onManage} />)
    expect(screen.getByRole('status')).not.toHaveTextContent('marketData.')
    fireEvent.click(screen.getByRole('button'))
    expect(onManage).toHaveBeenCalledOnce()
  })
})
