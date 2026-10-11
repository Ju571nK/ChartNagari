import { fireEvent, render, renderHook, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import i18n from './i18n'
import { Forex15MCollection, useForexTimeframe } from './ForexTimeframe'
import { signalRange } from './uxCopy'

beforeEach(async () => { await i18n.changeLanguage('en') })
afterEach(() => vi.restoreAllMocks())
const reply = (timeframes: string[]) => ({ ok: true, json: async () => ({ timeframes }) }) as Response

it('requires an explicit opt-in and preserves the latest collection timeframes', async () => {
  const refresh = vi.fn()
  const fetcher = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(reply(['1H', '4H'])).mockResolvedValueOnce(reply(['1H', '4H', '1W'])).mockResolvedValue(reply(['1H', '4H', '1W', '15M']))
  render(<Forex15MCollection visible onEnabled={refresh} />)
  const button = await screen.findByRole('button', { name: 'Enable Forex 15M collection' })
  await waitFor(() => expect(button).toBeEnabled())
  expect(fetcher).toHaveBeenCalledTimes(1)
  fireEvent.click(button)
  await waitFor(() => expect(refresh).toHaveBeenCalledTimes(1))
  expect(fetcher.mock.calls[2][0]).toBe('/api/watchlist/timeframes')
  expect(JSON.parse(String(fetcher.mock.calls[2][1]?.body))).toEqual({ timeframes: ['1H', '4H', '1W', '15M'] })
})

it('does not access collection settings for legacy timeframes or non-Forex', () => {
  const fetcher = vi.spyOn(globalThis, 'fetch')
  render(<Forex15MCollection visible={false} />)
  expect(fetcher).not.toHaveBeenCalled()
})

it('normalizes a known non-Forex selection to 1H while preserving Forex15M', () => {
  const setTimeframe = vi.fn()
  const { rerender } = renderHook(({ symbol }) => useForexTimeframe(symbol, '15M', setTimeframe, [{ symbol: 'EURUSD', type: 'forex' }, { symbol: 'SPY', type: 'stock' }]), { initialProps: { symbol: 'EURUSD' } })
  expect(setTimeframe).not.toHaveBeenCalled()
  rerender({ symbol: 'SPY' })
  expect(setTimeframe).toHaveBeenCalledWith('1H')
})

it('uses 900-second signal range boundaries for15M', () => {
  expect(signalRange(999, [{ time: 100 }], '15M')).toBe('inRange')
  expect(signalRange(1000, [{ time: 100 }], '15M')).toBe('archive')
})
