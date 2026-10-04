import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { ChartTab } from './App'
import { WorkspaceProvider } from './Workspace'
import i18n from './i18n'

const chartMocks = vi.hoisted(() => ({ setData: vi.fn(), fitContent: vi.fn(), addSeries: vi.fn(), removeSeries: vi.fn(), setMarkers: vi.fn() }))
vi.mock('lightweight-charts', () => ({
  CandlestickSeries: {}, HistogramSeries: {}, LineSeries: {}, CrosshairMode: { Normal: 0 },
  createSeriesMarkers: (_: unknown, markers: unknown) => { chartMocks.setMarkers(markers); return { detach: vi.fn(), setMarkers: chartMocks.setMarkers } },
  createChart: () => ({
    addSeries: (...args: unknown[]) => { chartMocks.addSeries(...args); return { setData: chartMocks.setData, applyOptions: vi.fn(), priceScale: () => ({ applyOptions: vi.fn() }), createPriceLine: vi.fn(), removePriceLine: vi.fn() } },
    priceScale: () => ({ applyOptions: vi.fn() }), timeScale: () => ({ fitContent: chartMocks.fitContent, setVisibleLogicalRange: vi.fn() }),
    applyOptions: vi.fn(), remove: vi.fn(), removeSeries: chartMocks.removeSeries,
  }),
}))

const bar = { time: 1700000000, open: 10, high: 12, low: 9, close: 11, volume: 100 }
const response = (data: unknown, ok = true) => ({ ok, status: ok ? 200 : 500, statusText: 'Error', json: async () => data }) as Response

it('keeps historical signals in the inspector but not on unrelated candles', async () => {
  vi.spyOn(globalThis, 'fetch').mockImplementation(async input => {
    const path = String(input)
    return path.includes('/ohlcv/') ? response([bar]) : path.includes('/signals?') ? response([{time:bar.time-86400, timeframe:'1H',rule:'hammer',direction:'LONG',score:10,message:'Historical fixture'}]) : response([])
  })
  render(<WorkspaceProvider><ChartTab uiMode="expert" /></WorkspaceProvider>)
  expect(await screen.findByText('Outside loaded chart range')).toBeInTheDocument()
  await waitFor(() => expect(chartMocks.setMarkers.mock.calls.at(-1)?.[0]).toHaveLength(0))
  expect(screen.getByText('Historical fixture')).toBeInTheDocument()
})

beforeEach(async () => {
  await i18n.changeLanguage('en')
  localStorage.clear()
  chartMocks.setData.mockClear()
  chartMocks.addSeries.mockClear()
  chartMocks.removeSeries.mockClear()
  chartMocks.setMarkers.mockClear()
  window.history.replaceState(null, '', '/?view=chart&symbol=SPCX&tf=1H')
})
afterEach(() => { vi.restoreAllMocks() })

it('keeps valid candles visible when signals fail', async () => {
  vi.spyOn(globalThis, 'fetch').mockImplementation(async input => {
    const path = String(input)
    return path.includes('/ohlcv/') ? response([bar]) : path.includes('/signals?') ? response(null, false) : response([])
  })
  const { container } = render(<WorkspaceProvider><ChartTab uiMode="beginner" /></WorkspaceProvider>)
  expect(await screen.findByText(/1 candles loaded/)).toBeInTheDocument()
  expect(await screen.findByRole('alert')).toHaveTextContent('Signals could not be loaded')
  expect(container.querySelector('.chart-area')).toHaveAttribute('aria-hidden', 'false')
})

it('hides the old chart on timeframe change and never paints a late response', async () => {
  let resolveOld!: (value: Response) => void
  let resolveNew!: (value: Response) => void
  vi.spyOn(globalThis, 'fetch').mockImplementation(async input => {
    const path = String(input)
    if (path.includes('/ohlcv/SPCX/1H')) return new Promise(resolve => { resolveOld = resolve })
    if (path.includes('/ohlcv/SPCX/1D')) return new Promise(resolve => { resolveNew = resolve })
    return response([])
  })
  const { container } = render(<WorkspaceProvider><ChartTab uiMode="beginner" /></WorkspaceProvider>)
  fireEvent.click(screen.getByRole('button', { name: '1D' }))
  expect(container.querySelector('.chart-area')).toHaveAttribute('aria-hidden', 'true')
  await act(async () => resolveNew(response([bar])))
  await waitFor(() => expect(container.querySelector('.chart-area')).toHaveAttribute('aria-hidden', 'false'))
  await act(async () => resolveOld(response([{ ...bar, close: 999 }])))
  expect(chartMocks.setData.mock.calls.some(([bars]) => bars.some((b: {close?: number}) => b.close === 999))).toBe(false)
})

it('shows empty guidance instead of an empty chart axis or no-signal advice', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(response([]))
  const { container } = render(<WorkspaceProvider><ChartTab uiMode="beginner" /></WorkspaceProvider>)
  expect(await screen.findByText(/No stored candles/)).toBeInTheDocument()
  expect(container.querySelector('.chart-area')).toHaveAttribute('aria-hidden', 'true')
  expect(screen.queryByText(/No signals on this timeframe/)).not.toBeInTheDocument()
})

it('renders independent candle patterns and removes them when disabled or timeframe changes', async () => {
  const candles = [
    { ...bar, time: 1, open: 10, high: 11, low: 9, close: 9.5 },
    { ...bar, time: 2, open: 9.5, high: 14, low: 9.5, close: 14 },
    { ...bar, time: 3, open: 14, high: 16, low: 13, close: 15 },
  ]
  vi.spyOn(globalThis, 'fetch').mockImplementation(async input => response(String(input).includes('/ohlcv/SPCX/1H') ? candles : []))
  render(<WorkspaceProvider><ChartTab uiMode="expert" /></WorkspaceProvider>)
  await screen.findByText(/3 candles loaded/)
  fireEvent.click(screen.getByRole('button', { name: 'FVG' }))
  expect(screen.getByRole('button', { name: 'FVG' })).toHaveAttribute('aria-pressed', 'true')
  expect(screen.getByText(/FVG: 1 active candidate/)).toBeInTheDocument()
  expect(chartMocks.addSeries.mock.calls.some(([, options]) => options?.title === 'FVG ↑')).toBe(true)
  fireEvent.click(screen.getByRole('button', { name: 'FVG' }))
  expect(chartMocks.removeSeries).toHaveBeenCalledTimes(2)
  fireEvent.click(screen.getByRole('button', { name: 'OB' }))
  expect(screen.getByText(/OB: 1 active candidate/)).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: '1D' }))
  await screen.findByText(/No stored candles/)
  expect(chartMocks.removeSeries).toHaveBeenCalledTimes(4)
  expect(screen.queryByText(/OB: 1 active candidate/)).not.toBeInTheDocument()
})

it('loads Zones without W.Phase and reports unavailable VIX data', async () => {
  const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async input => {
    if (String(input).includes('/wyckoff/')) return response({ phase: 'ranging', events: [], phase_zones: [] })
    return response([])
  })
  render(<WorkspaceProvider><ChartTab uiMode="expert" /></WorkspaceProvider>)
  fireEvent.click(screen.getByRole('button', { name: 'Zones' }))
  await waitFor(() => expect(fetchMock.mock.calls.some(([url]) => String(url).includes('/wyckoff/SPCX/1H'))).toBe(true))
  expect(screen.getByRole('button', { name: 'W.Phase' })).toHaveAttribute('aria-pressed', 'false')
  await screen.findByText(/Zones: No matching/)
  fireEvent.click(screen.getByRole('button', { name: 'VIX' }))
  await screen.findByText(/VIX: No stored data/)
})

it('filters both markers and the signal list to the selected timeframe', async () => {
  const signal = { symbol: 'SPCX', time: bar.time, direction: 'LONG', rule: 'smc_bos', score: 0.9, message: 'one-hour signal', timeframe: '1H' }
  vi.spyOn(globalThis, 'fetch').mockImplementation(async input => {
    const path = String(input)
    return response(path.includes('/ohlcv/') ? [bar] : path.includes('/signals?') ? [signal, { ...signal, time: bar.time + 10, timeframe: '4H', message: 'four-hour signal' }] : [])
  })
  render(<WorkspaceProvider><ChartTab uiMode="expert" /></WorkspaceProvider>)
  await waitFor(() => expect(chartMocks.setMarkers.mock.calls.at(-1)?.[0]).toHaveLength(1))
  expect(chartMocks.setMarkers.mock.calls.at(-1)?.[0][0].time).toBe(bar.time)
  fireEvent.click(screen.getByRole('button', { name: '4H' }))
  await waitFor(() => expect(chartMocks.setMarkers.mock.calls.at(-1)?.[0]?.[0]?.time).toBe(bar.time + 10))
})

it.each([
  { language: 'ko', message: 'America/New_York 10:30 기준 런던 마감 킬존 활성', context: '세션 정보', scope: '모든 시간봉' },
  { language: 'ja', message: 'America/New_York 10:30 時点でロンドン引けキルゾーンが有効', context: 'セッション情報', scope: 'すべての時間足' },
])('shows real ALL kill-zone context in $language without a candle marker', async ({ language, message, context, scope }) => {
  await i18n.changeLanguage(language)
  const session = {
    symbol: 'SPCX', time: bar.time, timeframe: 'ALL', rule: 'ict_kill_zone', direction: 'NEUTRAL', score: 1,
    message: 'London Close kill zone active (10:30 New York)', message_key: 'signal.ict.kill_zone',
    message_params: { session: 'London Close', time: '10:30', timezone: 'America/New_York' },
  }
  const candleSignal = { symbol: 'SPCX', time: bar.time, timeframe: '1H', rule: 'smc_bos', direction: 'LONG', score: 0.9, message: 'Candle signal' }
  vi.spyOn(globalThis, 'fetch').mockImplementation(async input => {
    const path = String(input)
    return response(path.includes('/ohlcv/') ? [bar] : path.includes('/signals?') ? [session, candleSignal] : [])
  })

  render(<WorkspaceProvider><ChartTab uiMode="expert" /></WorkspaceProvider>)
  expect(await screen.findByText(message)).toBeInTheDocument()
  const sessionRow = screen.getByRole('button', { name: new RegExp(context) })
  expect(sessionRow).toHaveTextContent(scope)
  expect(sessionRow).not.toHaveTextContent('NEUTRAL')
  await waitFor(() => expect(chartMocks.setMarkers.mock.calls.at(-1)?.[0]).toHaveLength(1))
  expect(chartMocks.setMarkers.mock.calls.at(-1)?.[0][0].text).toBe('BOS')
  fireEvent.click(screen.getByRole('button', { name: /SMC.*BOS/i }))
  expect(screen.getByText('Candle signal')).toBeInTheDocument()
  fireEvent.click(sessionRow)
  expect(screen.getByText(message)).toBeInTheDocument()

  fireEvent.click(screen.getByRole('button', { name: '1D' }))
  expect(await screen.findByText(message)).toBeInTheDocument()
  expect(screen.getByRole('button', { name: new RegExp(context) })).toHaveTextContent(scope)
  expect(screen.queryByText('Candle signal')).not.toBeInTheDocument()
  await waitFor(() => expect(chartMocks.setMarkers.mock.calls.at(-1)?.[0]).toHaveLength(0))
  fireEvent.click(screen.getByRole('button', { name: 'ICT' }))
  expect(screen.getByRole('button', { name: 'ICT' })).toHaveAttribute('aria-pressed', 'false')
  expect(chartMocks.setMarkers.mock.calls.at(-1)?.[0]).toHaveLength(0)
})
