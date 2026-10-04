import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { DxyPanel, SourceNote, StrengthMeter } from './ForexPanels'
import { SettingsTab, SymbolsTab } from './App'
import { WorkspaceProvider } from './Workspace'
import i18n from './i18n'

const json = (data: unknown, status = 200) => new Response(JSON.stringify(data), { status })
beforeEach(async () => { await i18n.changeLanguage('en'); window.history.replaceState(null, '', '/?view=symbols'); localStorage.clear() })
afterEach(() => vi.restoreAllMocks())

it('shows low coverage as unavailable even if an API value is present', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(json({ currencies: [{ currency: 'EUR', value: 97, coverage: 'low', pairs: 1 }] }))
  render(<StrengthMeter />)
  expect(await screen.findByText('EUR')).toBeInTheDocument()
  expect(screen.queryByText('97.0')).not.toBeInTheDocument()
  expect(screen.getByText('low coverage')).toBeInTheDocument()
})

it('shows an API failure instead of an empty strength meter', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(json({ error: 'failed' }, 503))
  render(<StrengthMeter />)
  expect(await screen.findByRole('alert')).toHaveTextContent('HTTP 503')
})

it('recovers on the same mounted meter after a failed request and chart revision', async () => {
  const fetcher = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(json({ error: 'backfill' }, 503))
    .mockResolvedValue(json({ currencies: [{ currency: 'USD', value: 30, coverage: 'ok', pairs: 7 }] }))
  const view = render(<StrengthMeter revision={0} />)
  expect(await screen.findByRole('alert')).toHaveTextContent('HTTP 503')
  view.rerender(<StrengthMeter revision={1} />)
  expect(await screen.findByText('30.0')).toBeInTheDocument()
  expect(fetcher).toHaveBeenCalledTimes(2)
})

it('recovers from empty backfill data on revision without remounting', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(json({ currencies: [] }))
    .mockResolvedValue(json({ currencies: [{ currency: 'USD', value: 12, coverage: 'ok', pairs: 7 }] }))
  const view = render(<StrengthMeter revision={0} />)
  expect(await screen.findByText('Enable forex pairs to see strength.')).toBeInTheDocument()
  view.rerender(<StrengthMeter revision={1} />)
  expect(await screen.findByText('12.0')).toBeInTheDocument()
})

it('ignores a stale response after the selected pair changes', async () => {
  let resolveOld!: (response: Response) => void
  const old = new Promise<Response>(resolve => { resolveOld = resolve })
  vi.spyOn(globalThis, 'fetch').mockImplementation(input => String(input).includes('EURUSD')
    ? old : Promise.resolve(json({ bars: [{ time: 1, close: 100 }, { time: 2, close: 101 }], correlation_20: 0.42, correlation_60: null })))
  const view = render(<DxyPanel symbol="EURUSD" />)
  view.rerender(<DxyPanel symbol="USDJPY" />)
  expect(await screen.findByText(/0.42/)).toBeInTheDocument()
  resolveOld(json({ bars: [{ time: 1, close: 100 }, { time: 2, close: 101 }], correlation_20: -0.99, correlation_60: null }))
  await Promise.resolve()
  expect(screen.queryByText(/-0.99/)).not.toBeInTheDocument()
})

it('rejects a malformed success response instead of treating it as no data', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(json([]))
  render(<StrengthMeter />)
  expect(await screen.findByRole('alert')).toHaveTextContent('Invalid FX data response')
})

it('shows DXY only for USD pairs and displays both correlations', async () => {
  const fetcher = vi.spyOn(globalThis, 'fetch').mockResolvedValue(json({ bars: [{ time: 1, close: 100 }, { time: 2, close: 101 }], correlation_20: -0.72, correlation_60: null }))
  const view = render(<DxyPanel symbol="EURGBP" />)
  expect(fetcher).not.toHaveBeenCalled()
  view.rerender(<DxyPanel symbol="EURUSD" />)
  expect(await screen.findByText(/-0.72/)).toBeInTheDocument()
  expect(screen.getByRole('img', { name: 'DXY daily mini chart' })).toBeInTheDocument()
})

it('respects an explicitly disabled DXY with retained history absent', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(json({ state: 'disabled', bars: [], correlation_20: null, correlation_60: null }))
  render(<DxyPanel symbol="EURUSD" />)
  expect(await screen.findByText('DXY is disabled in the watchlist.')).toBeInTheDocument()
  expect(screen.queryByText(/20-day correlation:/)).not.toBeInTheDocument()
})

it('uses saved origin and suspension metadata after a provider switch', () => {
  render(<SourceNote source={{ data_provider: 'yahoo', provider_symbol: 'GC=F', source_identity: 'yahoo|GC=F|proxy', data_proxy: true, suspended: true, suspension_reason: 'source_mismatch' }} />)
  expect(screen.getByText(/YAHOO · GC=F · futures proxy/)).toHaveTextContent('Suspended: current price source differs from this position')
})

it('adds the majors preset with canonical forex symbols', async () => {
  const fetcher = vi.spyOn(globalThis, 'fetch').mockImplementation(async input => String(input).endsWith('/symbols') ? json([]) : json([]))
  render(<WorkspaceProvider><SymbolsTab /></WorkspaceProvider>)
  fireEvent.click(await screen.findByRole('button', { name: 'Majors (7)' }))
  await waitFor(() => expect(fetcher.mock.calls.filter(([, init]) => init?.method === 'POST')).toHaveLength(7))
  const bodies = fetcher.mock.calls.filter(([, init]) => init?.method === 'POST').map(([, init]) => JSON.parse(String(init?.body)))
  expect(bodies[0]).toEqual({ symbol: 'EURUSD', type: 'forex', exchange: 'FX' })
  expect(bodies.at(-1)?.symbol).toBe('NZDUSD')
})

it('keeps an OANDA token masked and sends only edited FX settings', async () => {
  const fetcher = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => String(input) === '/api/settings/config' && init?.method === 'PUT'
    ? json({}) : json({ FOREX_PROVIDER: 'auto', OANDA_TOKEN: '__configured__', OANDA_ENVIRONMENT: 'practice' }))
  render(<SettingsTab uiMode="beginner" onSetUiMode={vi.fn()} />)
  fireEvent.click(await screen.findByRole('tab', { name: 'Data Sources' }))
  const token = screen.getByLabelText('OANDA token') as HTMLInputElement
  expect(token.value).toBe('')
  expect(token.placeholder).toMatch(/configured/i)
  fireEvent.change(screen.getByLabelText('FX provider'), { target: { value: 'oanda' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(fetcher.mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(true))
  const save = fetcher.mock.calls.find(([, init]) => init?.method === 'PUT')!
  expect(JSON.parse(String(save[1]?.body))).toEqual({ FOREX_PROVIDER: 'oanda' })
})

it('localizes a saved OANDA token placeholder and explicit clear action in Korean and Japanese', async () => {
  const fetcher = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => String(input) === '/api/settings/config' && init?.method === 'PUT'
    ? json({}) : json({ FOREX_PROVIDER: 'oanda', OANDA_TOKEN: '__configured__', OANDA_ENVIRONMENT: 'practice' }))
  render(<SettingsTab uiMode="beginner" onSetUiMode={vi.fn()} />)
  fireEvent.click(await screen.findByRole('tab', { name: 'Data Sources' }))
  const token = screen.getByLabelText('OANDA token') as HTMLInputElement

  for (const [language, placeholder, clearName] of [
    ['ko', '설정됨', 'OANDA 토큰 지우기'],
    ['ja', '設定済み', 'OANDAトークンをクリア'],
  ] as const) {
    await i18n.changeLanguage(language)
    expect(token.value).toBe('')
    expect(token.placeholder).toContain(placeholder)
    expect(screen.getByRole('button', { name: clearName })).toBeInTheDocument()
  }

  fireEvent.click(screen.getByRole('button', { name: 'OANDAトークンをクリア' }))
  expect(token.value).toBe('')
  await i18n.changeLanguage('en')
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(fetcher.mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(true))
  const save = fetcher.mock.calls.find(([, init]) => init?.method === 'PUT')!
  expect(JSON.parse(String(save[1]?.body))).toEqual({ OANDA_TOKEN: '' })
})

it('explains keyless Yahoo in Auto and lets a saved-token user choose Yahoo', async () => {
  const fetcher = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) =>
    String(input) === '/api/settings/config' && init?.method === 'PUT'
      ? json({}) : json({ FOREX_PROVIDER: 'auto', OANDA_TOKEN: '__configured__', OANDA_ENVIRONMENT: 'practice' }))
  render(<SettingsTab uiMode="beginner" onSetUiMode={vi.fn()} />)
  fireEvent.click(await screen.findByRole('tab', { name: 'Data Sources' }))
  expect(screen.getByText(/Auto uses Yahoo without an OANDA token/)).toHaveTextContent('It does not switch to Yahoo if OANDA fails.')
  fireEvent.change(screen.getByLabelText('FX provider'), { target: { value: 'yahoo' } })
  expect(screen.getByText(/Yahoo uses keyless FX prices even if an OANDA token is saved/)).toBeInTheDocument()
  expect(screen.getByLabelText('OANDA token')).toHaveAttribute('placeholder', expect.stringMatching(/configured/i))
  expect(screen.getByRole('button', { name: 'Test connection' })).toBeDisabled()
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(fetcher.mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(true))
  const saved = fetcher.mock.calls.find(([, init]) => init?.method === 'PUT')!
  expect(JSON.parse(String(saved[1]?.body))).toEqual({ FOREX_PROVIDER: 'yahoo' })
})

it('shows OANDA eligibility and the official Japan requirements in each UI language', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(json({ FOREX_PROVIDER: 'oanda', OANDA_TOKEN: '', OANDA_ENVIRONMENT: 'practice' }))
  render(<SettingsTab uiMode="beginner" onSetUiMode={vi.fn()} />)
  fireEvent.click(await screen.findByRole('tab', { name: 'Data Sources' }))
  const link = screen.getByRole('link', { name: 'OANDA Japan API eligibility (official)' })
  expect(link).toHaveAttribute('href', 'https://help.oanda.jp/oanda/faq/show/808')
  expect(screen.getByText(/Practice account does not guarantee free API access/)).toBeInTheDocument()
  for (const [language, linkName] of [['ko', 'OANDA Japan API 이용 자격 (공식)'], ['ja', 'OANDA JapanのAPI利用条件（公式）']] as const) {
    await i18n.changeLanguage(language)
    expect(screen.getByRole('link', { name: linkName })).toHaveAttribute('href', 'https://help.oanda.jp/oanda/faq/show/808')
  }
})

it('reports provider connection failures instead of silently accepting them', async () => {
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => String(input) === '/api/forex/test-connection'
    ? new Response('OANDA token rejected', { status: 502 }) : json({ FOREX_PROVIDER: 'oanda', OANDA_TOKEN: '__configured__', OANDA_ENVIRONMENT: 'practice' }))
  render(<SettingsTab uiMode="beginner" onSetUiMode={vi.fn()} />)
  fireEvent.click(await screen.findByRole('tab', { name: 'Data Sources' }))
  fireEvent.click(screen.getByRole('button', { name: 'Test connection' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('OANDA token rejected')
})

it('validates forex pairs locally and rejects malformed symbols', async () => {
  const fetcher = vi.spyOn(globalThis, 'fetch').mockResolvedValue(json([]))
  const { container } = render(<WorkspaceProvider><SymbolsTab /></WorkspaceProvider>)
  fireEvent.change(await screen.findByPlaceholderText(i18n.t('symbol_placeholder_nvda')), { target: { value: 'EURUS1' } })
  fireEvent.change(container.querySelector('.add-symbol-form select')!, { target: { value: 'forex' } })
  expect(screen.getByRole('button', { name: 'Add' })).toBeDisabled()
  fireEvent.change(screen.getByPlaceholderText(i18n.t('symbol_placeholder_nvda')), { target: { value: 'EURUSD' } })
  await waitFor(() => expect(screen.getByRole('button', { name: 'Add' })).toBeEnabled())
  expect(fetcher.mock.calls.some(([input]) => String(input).includes('/validate?'))).toBe(false)
})
