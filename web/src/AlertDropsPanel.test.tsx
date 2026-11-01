import { render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AlertDropsPanel } from './AlertDropsPanel'
import i18n from './i18n'

const ok = (data: unknown) => ({ ok: true, status: 200, json: async () => data }) as Response

beforeEach(async () => { await i18n.changeLanguage('en') })
afterEach(() => { vi.restoreAllMocks() })

describe('Alert drops panel', () => {
  it('shows the disabled state when recording is off', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(ok({ enabled: false, drops: [] }))
    render(<AlertDropsPanel symbol="BTCUSDT" refreshToken={0} />)
    expect(await screen.findByText('Alert drop recording is off.')).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })

  it('shows an empty list for a symbol with no drops', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(ok({ enabled: true, drops: [] }))
    render(<AlertDropsPanel symbol="BTCUSDT" refreshToken={0} />)
    expect(await screen.findByText('No dropped alerts for this symbol.')).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })

  it('lists a drop with the translated reason', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(ok({
      enabled: true,
      drops: [{
        symbol: 'BTCUSDT', rule: 'ict_fvg', timeframe: '1H', direction: 'LONG',
        reason: 'cooldown', stage: 'notifier', knob: 'override.cooldown_hours',
        score: 14.2, threshold: 0, signal_id: 0, sender: '', detail: '4h0m0s',
        count: 37, first_seen: '2026-11-01T09:00:00Z', last_seen: '2026-11-01T12:15:00Z',
      }],
    }))
    const { rerender } = render(<AlertDropsPanel symbol="BTCUSDT" refreshToken={0} />)
    expect(await screen.findByRole('table')).toBeInTheDocument()
    expect(screen.getByText('Cooldown')).toBeInTheDocument()
    expect(screen.getByText('ict_fvg')).toBeInTheDocument()
    expect(screen.getByText('37')).toBeInTheDocument()
    expect(screen.getByText('4h0m0s')).toBeInTheDocument()
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1))
    expect(String(fetchMock.mock.calls[0][0])).toContain('symbol=BTCUSDT')
    rerender(<AlertDropsPanel symbol="BTCUSDT" refreshToken={1} />)
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2))
  })
})
