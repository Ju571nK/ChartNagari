import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { HTFReadinessPanel } from './BacktestPreparation'
import './i18n'

afterEach(() => { cleanup(); vi.unstubAllGlobals() })
it('loads only on request and displays an explicitly non-calibrated inventory', async () => {
  const fetcher = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ symbol: 'TEST', timeframe: '1H', ready: false, buckets: Array.from({ length: 10 }, (_, decile) => ({ decile, signals: 0, distinct_days: 0, from: null, to: null, history_sufficient: false })) }) })
  vi.stubGlobal('fetch', fetcher)
  render(<HTFReadinessPanel symbol="TEST" timeframe="1H" />)
  expect(fetcher).not.toHaveBeenCalled()
  fireEvent.click(screen.getByText('HTF calibration · history inventory'))
  fireEvent.click(screen.getByRole('button', { name: 'Check history' }))
  expect(await screen.findByRole('table')).toBeTruthy()
  expect(screen.getAllByText('Insufficient')).toHaveLength(10)
  expect(screen.getByText(/Automatic calibration is not enabled/)).toBeTruthy()
})
it('reports errors and permits retry', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false }))
  render(<HTFReadinessPanel symbol="TEST" timeframe="4H" />)
  fireEvent.click(screen.getByText('HTF calibration · history inventory'))
  fireEvent.click(screen.getByRole('button', { name: 'Check history' }))
  expect(await screen.findByRole('alert')).toBeTruthy()
  await waitFor(() => expect((screen.getByRole('button') as HTMLButtonElement).disabled).toBe(false))
})
it('does not offer calibration for daily signals', () => {
  render(<HTFReadinessPanel symbol="TEST" timeframe="1D" />)
  expect(screen.queryByRole('button', { hidden: true })).toBeNull()
  expect(screen.getByText(/Select either timeframe/)).toBeTruthy()
})

it('aborts an in-flight inventory when the selection is removed', async () => {
  let signal: AbortSignal | undefined
  vi.stubGlobal('fetch', vi.fn((_url, options) => {
    signal = options.signal
    return new Promise(() => {})
  }))
  const view = render(<HTFReadinessPanel symbol="TEST" timeframe="1H" />)
  fireEvent.click(screen.getByText('HTF calibration · history inventory'))
  fireEvent.click(screen.getByRole('button', { name: 'Check history' }))
  await waitFor(() => expect(signal).toBeDefined())
  view.unmount()
  expect(signal?.aborted).toBe(true)
})

it('rejects a demo or mismatched response instead of displaying false readiness', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => ({ ready: true }) }))
  render(<HTFReadinessPanel symbol="TEST" timeframe="1H" />)
  fireEvent.click(screen.getByText('HTF calibration · history inventory'))
  fireEvent.click(screen.getByRole('button', { name: 'Check history' }))
  expect(await screen.findByRole('alert')).toBeTruthy()
  expect(screen.queryByRole('table')).toBeNull()
})
