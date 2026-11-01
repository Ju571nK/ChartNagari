import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import i18n from './i18n'
import { PaperTab } from './App'

const summary = {
  open_positions: 1,
  closed_trades: 2,
  wins: 1,
  losses: 1,
  win_rate: 0.5,
  total_pnl_pct: 4,
  avg_win_pct: 5,
  avg_loss_pct: -1,
}

const openRow = {
  id: 7,
  symbol: 'BTCUSDT',
  timeframe: '1H',
  rule: 'rsi',
  direction: 'LONG',
  entry_price: 100,
  tp: 110,
  sl: 90,
  entry_time: '2026-11-01T00:00:00Z',
  exit_price: 0,
  exit_time: null,
  status: 'OPEN',
  pnl_pct: 0,
}

const history = [
  { ...openRow, id: 3, status: 'CLOSED_MANUAL', exit_price: 105, pnl_pct: 5 },
  { ...openRow, id: 4, status: 'CLOSED_SL', exit_price: 90, pnl_pct: -1 },
  { ...openRow, id: 5, status: 'CLOSED_TP', exit_price: 110, pnl_pct: 10 },
]

function jsonResponse(body: unknown): Response {
  return { ok: true, status: 200, json: async () => body } as Response
}

beforeEach(() => {
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (url, init) => {
    const u = String(url)
    if (init?.method === 'POST' && u.includes('/api/paper/positions/7/close')) {
      return jsonResponse({ id: 7, status: 'CLOSED_MANUAL' })
    }
    if (u.includes('/api/paper/summary')) return jsonResponse(summary)
    if (u.includes('/api/paper/positions')) return jsonResponse([openRow])
    if (u.includes('/api/paper/history')) return jsonResponse(history)
    return { ok: false, status: 404, json: async () => ({}) } as Response
  })
})

afterEach(() => { vi.restoreAllMocks() })

describe('PaperTab', () => {
  it('closes an open position and labels a manual close apart from a stop', async () => {
    render(<I18nextProvider i18n={i18n}><PaperTab /></I18nextProvider>)
    expect(await screen.findByText(/Manual|수동|手動/)).toBeInTheDocument()
    expect(screen.getByText('SL ✗')).toBeInTheDocument()
    expect(screen.getByText('TP ✓')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /^Close$|^청산$|^決済$/ }))
    await waitFor(() => {
      const post = vi.mocked(globalThis.fetch).mock.calls.find(c => (c[1] as RequestInit | undefined)?.method === 'POST')
      expect(String(post?.[0])).toContain('/api/paper/positions/7/close')
      expect((post?.[1] as RequestInit).body).toBe('{}')
    })
  })
})
