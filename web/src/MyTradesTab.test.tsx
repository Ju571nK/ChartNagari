import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import i18n from './i18n'
import { MyTradesTab } from './MyTradesTab'

const ROLLUP_RESPONSE = {
  by: 'rule',
  since: '2026-04-09T00:00:00Z',
  rows: [
    { key: 'ict_liquidity_sweep', took: 12, skipped: 6, wins: 8, losses: 3, bes: 1, hit_rate: 0.667, skip_rate: 0.333 },
    { key: 'wyckoff_spring',      took: 8,  skipped: 5, wins: 4, losses: 3, bes: 1, hit_rate: 0.5,   skip_rate: 0.385 },
  ],
}

const wrap = (ui: React.ReactElement) => (
  <I18nextProvider i18n={i18n}>{ui}</I18nextProvider>
)

beforeEach(() => {
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (url) => {
    const u = String(url)
    if (u.includes('/api/marks/rollup')) {
      return { ok: true, status: 200, json: async () => ROLLUP_RESPONSE } as Response
    }
    if (u.includes('/api/marks/pending')) {
      return { ok: true, status: 200, json: async () => [] } as Response
    }
    if (u.includes('/api/marks/recent')) {
      return { ok: true, status: 200, json: async () => [] } as Response
    }
    return { ok: false, status: 404, json: async () => ({}) } as Response
  })
})
afterEach(() => { vi.restoreAllMocks() })

describe('MyTradesTab', () => {
  it('renders Rollup table from fetch', async () => {
    render(wrap(<MyTradesTab />))
    await waitFor(() => expect(screen.queryByText('ict_liquidity_sweep')).toBeDefined())
    expect(screen.getByText(/66.7%/)).toBeDefined()
  })

  it('changing GroupBy refetches', async () => {
    render(wrap(<MyTradesTab />))
    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalled())
    const initial = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls.length
    const select = screen.getByLabelText(/groupBy/i) as HTMLSelectElement
    fireEvent.change(select, { target: { value: 'symbol' } })
    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls
      const last = calls[calls.length - 1][0] as string
      expect(last).toContain('by=symbol')
    })
    expect((globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls.length).toBeGreaterThan(initial)
  })

  it('switches to Pending subtab', async () => {
    render(wrap(<MyTradesTab />))
    const pendingTab = await screen.findByRole('button', { name: /Pending|대기 중|保留中/ })
    fireEvent.click(pendingTab)
    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls
      const urls = calls.map(c => String(c[0]))
      expect(urls.some(u => u.includes('/api/marks/pending'))).toBe(true)
    })
  })

  it('shows empty pending message', async () => {
    render(wrap(<MyTradesTab />))
    fireEvent.click(await screen.findByRole('button', { name: /Pending|대기 중|保留中/ }))
    await waitFor(() => expect(screen.queryByText(/all caught up|대기 중인 시그널 없음|保留中のシグナルなし/)).toBeDefined())
  })
})
