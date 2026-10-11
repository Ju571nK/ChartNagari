import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { AnalysisTab } from './AnalysisTab'
import { WorkspaceProvider } from './Workspace'
import i18n from './i18n'

const legacy = {
  id: 1, symbol: 'SPCX', final: 'SIDEWAYS', confidence: 'LOW',
  bull_pct: 0, bear_pct: 0, sideways_pct: 0, created_at: '2026-09-11T08:00:00Z',
  aggregator_reason: 'Model unavailable', macro_text: '', fundamental_text: '', sentiment_text: '',
}

beforeEach(async () => {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  await i18n.changeLanguage('en')
  window.history.replaceState(null, '', '/?view=analysis&symbol=SPCX')
})
afterEach(() => { vi.restoreAllMocks() })

function mockResult(record: typeof legacy & { status?: string }) {
  vi.spyOn(globalThis, 'fetch').mockImplementation(async input => ({
    ok: true,
    json: async () => String(input).endsWith('/1') ? { result: record } : String(input).endsWith('/full') ? record : [record],
  }) as Response)
}

describe('Analysis failures', () => {
  it.each([undefined, 'failed'])('does not present %s failure history as a neutral verdict', async status => {
    mockResult({ ...legacy, status })
    render(<WorkspaceProvider><AnalysisTab /></WorkspaceProvider>)
    const row = await screen.findByRole('button', { name: /SPCX Analysis failed/ })
    fireEvent.keyDown(row, { key: 'Enter' })
    expect(await screen.findByRole('alert')).toHaveTextContent('This is not a neutral market signal')
    expect(screen.queryByText(/→ SIDEWAYS/)).not.toBeInTheDocument()
    expect(screen.queryByText('LOW')).not.toBeInTheDocument()
    expect(screen.queryByText('Scenario Probability')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Telegram|PDF/i })).not.toBeInTheDocument()
  })

  it('shows a newly failed run with recovery guidance', async () => {
    mockResult({ ...legacy, status: 'failed', final: 'ERROR' })
    render(<WorkspaceProvider><AnalysisTab /></WorkspaceProvider>)
    fireEvent.click(screen.getByRole('button', { name: /Run Analysis/i }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Check the AI provider')
    expect(screen.queryByRole('button', { name: /Telegram|PDF/i })).not.toBeInTheDocument()
  })

  it('preserves valid neutral results and export actions', async () => {
    mockResult({ ...legacy, sideways_pct: 100, confidence: 'HIGH' })
    render(<WorkspaceProvider><AnalysisTab /></WorkspaceProvider>)
    fireEvent.click(await screen.findByRole('button', { name: /SPCX.*SIDEWAYS/ }))
    expect(await screen.findByRole('button', { name: /PDF/i })).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.getByText('100.0%')).toBeInTheDocument()
  })
})


it('offersForex15M analysis using the selected symbol inventory', async () => {
  window.history.replaceState(null, '', '/?view=analysis&symbol=EURUSD&tf=1H')
  const fetcher = vi.spyOn(globalThis, 'fetch').mockImplementation(async input => ({
    ok: true,
    json: async () => String(input) === '/api/symbols' ? [{ symbol: 'EURUSD', type: 'forex' }] : String(input) === '/api/watchlist/timeframes' ? { timeframes: ['1H'] } : String(input) === '/api/analysis/full' ? { ...legacy, status: 'failed' } : [],
  }) as Response)
  render(<WorkspaceProvider><AnalysisTab /></WorkspaceProvider>)
  expect(await screen.findByRole('option', { name: '15M' })).toBeInTheDocument()
  fireEvent.change(screen.getByRole('combobox', { name: 'Timeframe' }), { target: { value: '15M' } })
  fireEvent.click(screen.getByRole('button', { name: /Run Analysis/i }))
  await waitFor(() => expect(fetcher.mock.calls.some(([url, options]) => String(url) === '/api/analysis/full' && JSON.parse(String(options?.body)).timeframe === '15M')).toBe(true))
})
