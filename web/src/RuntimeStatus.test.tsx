import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { CalendarCollectionStatus, SettingsRuntimeStatus } from './RuntimeStatus'
import i18n from './i18n'

beforeEach(async () => { await i18n.changeLanguage('en') })
afterEach(() => { cleanup(); vi.restoreAllMocks() })
const response = (data: unknown) => new Response(JSON.stringify(data))

it('distinguishes pending, startup-matching, external and unsaved fields without rendering values', async () => {
  vi.spyOn(globalThis, 'fetch').mockImplementation(async () => response({ fields: { FMP_API_KEY: 'pending_restart', FINNHUB_API_KEY: 'startup_match', ALPACA_API_KEY: 'external' } }))
  render(<SettingsRuntimeStatus fields={[{ key: 'FMP_API_KEY', label: 'FMP' }, { key: 'FINNHUB_API_KEY', label: 'Finnhub' }, { key: 'ALPACA_API_KEY', label: 'Alpaca' }, { key: 'OPENAI_API_KEY', label: 'OpenAI' }]} edits={{ OPENAI_API_KEY: 'never-show-secret' }} revision={0} />)
  await screen.findByText('Saved · restart required (1)')
  fireEvent.click(screen.getByText('Per-field startup comparison', { selector: 'summary' }))
  expect(screen.getByText('Matches startup settings')).toBeInTheDocument()
  expect(screen.getByText('Saved · external process not verified')).toBeInTheDocument()
  expect(screen.getByText('Unsaved edit')).toBeInTheDocument()
  expect(screen.queryByText('never-show-secret')).toBeNull()
})

it('refreshes settings comparison after save and does not infer activation from save alone', async () => {
  const fetcher = vi.spyOn(globalThis, 'fetch').mockImplementation(async () => response({ fields: { FMP_API_KEY: 'startup_match' } }))
  const view = render(<SettingsRuntimeStatus fields={[{ key: 'FMP_API_KEY', label: 'FMP' }]} edits={{}} revision={0} />)
  await screen.findByText('Matches startup settings')
  fetcher.mockImplementation(async () => response({ fields: { FMP_API_KEY: 'pending_restart' } }))
  view.rerender(<SettingsRuntimeStatus fields={[{ key: 'FMP_API_KEY', label: 'FMP' }]} edits={{}} revision={1} />)
  await screen.findByText('Saved · restart required (1)')
})

it('separates empty successful collection from missing provider and failed collection', async () => {
  const fetcher = vi.spyOn(globalThis, 'fetch').mockImplementation(async () => response({ state: 'success', provider: 'FMP', event_count: 0, last_success: '2026-09-12T10:00:00Z' }))
  render(<CalendarCollectionStatus />)
  await screen.findByText('Last collection succeeded')
  expect(screen.getByText(/Successful collection returned no usable US events/)).toBeInTheDocument()
  fetcher.mockImplementation(async () => response({ state: 'error', provider: 'FMP', failure: 'permission', last_success: '2026-09-12T10:00:00Z', event_count: 0 }))
  fireEvent.click(screen.getByRole('button', { name: 'Check status' }))
  await screen.findByText(/Access denied. Check account permissions/)
  expect(screen.queryByText(/Successful collection returned/)).toBeNull()
  expect(fetcher.mock.calls.every(([, options]) => !options?.method || options.method === 'GET')).toBe(true)
})

it('handles old-server or invalid status responses and allows retry', async () => {
  const fetcher = vi.spyOn(globalThis, 'fetch').mockImplementation(async () => response({}))
  render(<CalendarCollectionStatus />)
  await screen.findByText(/Status unavailable/)
  fetcher.mockImplementation(async () => response({ state: 'disabled', provider: 'none' }))
  fireEvent.click(screen.getByRole('button', { name: 'Check status' }))
  await screen.findByText('Disabled · no provider key loaded at startup')
  expect(screen.queryByText(/Status unavailable/)).toBeNull()
})

it('aborts pending diagnostics requests on unmount', () => {
  let signal: AbortSignal | undefined
  vi.spyOn(globalThis, 'fetch').mockImplementation((_url, options) => { signal = options?.signal as AbortSignal; return new Promise(() => {}) })
  const view = render(<CalendarCollectionStatus />)
  view.unmount()
  expect(signal?.aborted).toBe(true)
})

it('localizes Korean diagnostics and leaves absent success time unconfirmed', async () => {
  await i18n.changeLanguage('ko')
  vi.spyOn(globalThis, 'fetch').mockImplementation(async () => response({ state: 'waiting', provider: 'Finnhub', last_success: '0001-01-01T00:00:00Z', event_count: 0 }))
  render(<CalendarCollectionStatus />)
  await waitFor(() => expect(screen.getByText('첫 수집 대기 중')).toBeInTheDocument())
  expect(screen.getByText('—')).toBeInTheDocument()
})
