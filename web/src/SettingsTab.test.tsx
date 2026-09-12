import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { SettingsTab } from './App'
import i18n from './i18n'
import { setSessionToken } from './apiAuth'

beforeEach(async () => { await i18n.changeLanguage('en') })
afterEach(() => { vi.restoreAllMocks(); setSessionToken('') })

it('offers in-memory authorization and clears the input after checking the current token', async () => {
  const request = vi.spyOn(globalThis, 'fetch').mockImplementation(async input =>
    String(input) === '/api/auth/check' ? new Response(null, { status: 204 }) : new Response('{}', { status: 200 }))
  render(<SettingsTab uiMode="beginner" onSetUiMode={vi.fn()} />)
  const input = await screen.findByLabelText(/Current API token/)
  fireEvent.change(input, { target: { value: 'fixture-token' } })
  fireEvent.click(screen.getByRole('button', { name: 'Authorize changes' }))
  expect(await screen.findByRole('status')).toHaveTextContent('Authorized for this tab')
  expect(input).toHaveValue('')
  expect(request).toHaveBeenCalledWith('/api/auth/check', expect.objectContaining({ headers: { Authorization: 'Bearer fixture-token' } }))
  fireEvent.click(screen.getByRole('button', { name: 'Forget token' }))
  expect(screen.getByRole('status')).toHaveTextContent('Token forgotten')
})

it('saves only edited YAML fields and explicitly clears a masked secret', async () => {
  const fetcher = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({
    DB_PATH: 'data.db', API_TOKEN: '__configured__', SERVER_HOST: '127.0.0.1',
  }), { status: 200 }))
  // Return a fresh body for each read.
  fetcher.mockImplementation(async () => new Response(JSON.stringify({ DB_PATH: 'data.db', API_TOKEN: '__configured__', SERVER_HOST: '127.0.0.1' }), { status: 200 }))
  render(<SettingsTab uiMode="beginner" onSetUiMode={vi.fn()} />)
  fireEvent.click(await screen.findByRole('tab', { name: 'Advanced' }))
  const db = await screen.findByLabelText('Database path (restart required)')
  fireEvent.change(db, { target: { value: 'new.db' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(fetcher.mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(true))
  const saved = fetcher.mock.calls.find(([, init]) => init?.method === 'PUT')!
  expect(saved[0]).toBe('/api/settings/config')
  expect(JSON.parse(String(saved[1]?.body))).toEqual({ DB_PATH: 'new.db' })
  await screen.findByLabelText('Database path (restart required)')
  fireEvent.click(screen.getByRole('button', { name: /Clear API Token/ }))
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(fetcher.mock.calls.filter(([, init]) => init?.method === 'PUT')).toHaveLength(2))
  const last = fetcher.mock.calls.filter(([, init]) => init?.method === 'PUT')[1]
  expect(JSON.parse(String(last[1]?.body))).toEqual({ API_TOKEN: '' })
})
