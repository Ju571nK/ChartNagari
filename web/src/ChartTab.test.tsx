import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { ChartTab } from './App'
import { WorkspaceProvider } from './Workspace'
import i18n from './i18n'

const chartMocks = vi.hoisted(() => ({ setData: vi.fn(), fitContent: vi.fn() }))
vi.mock('lightweight-charts', () => ({
  CandlestickSeries: {}, HistogramSeries: {}, LineSeries: {}, CrosshairMode: { Normal: 0 },
  createSeriesMarkers: () => ({ detach: vi.fn(), setMarkers: vi.fn() }),
  createChart: () => ({
    addSeries: () => ({ setData: chartMocks.setData, priceScale: () => ({ applyOptions: vi.fn() }), createPriceLine: vi.fn(), removePriceLine: vi.fn() }),
    priceScale: () => ({ applyOptions: vi.fn() }), timeScale: () => ({ fitContent: chartMocks.fitContent }),
    applyOptions: vi.fn(), remove: vi.fn(), removeSeries: vi.fn(),
  }),
}))

const bar = { time: 1700000000, open: 10, high: 12, low: 9, close: 11, volume: 100 }
const response = (data: unknown, ok = true) => ({ ok, status: ok ? 200 : 500, statusText: 'Error', json: async () => data }) as Response

beforeEach(async () => {
  await i18n.changeLanguage('en')
  localStorage.clear()
  chartMocks.setData.mockClear()
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
