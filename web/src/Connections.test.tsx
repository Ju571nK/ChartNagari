import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { ConnectionPanel, normalizeServer } from './Connections'
import { authenticatedFetch, connectionFetch, getServerOrigin, selectServer } from './apiAuth'
import i18n from './i18n'

beforeEach(async () => { selectServer('', ''); localStorage.clear(); sessionStorage.clear(); await i18n.changeLanguage('en') })
afterEach(() => { cleanup(); selectServer('', ''); sessionStorage.clear(); vi.restoreAllMocks() })

it('validates secure origins and rejects URL credentials, paths, queries and public HTTP', () => {
  expect(normalizeServer('https://server.example/')).toBe('https://server.example')
  expect(normalizeServer('http://localhost:8080')).toBe('http://localhost:8080')
  for (const value of ['http://192.168.1.2:8080', 'https://user:token@server.example', 'https://server.example/api', 'https://server.example/?token=secret', 'file:///tmp', 'https://server.example/#x']) expect(() => normalizeServer(value)).toThrow()
})

it('routes APIs and streams only to the chosen server, isolates credentials and aborts old requests', async () => {
  const base = vi.fn().mockImplementation(async () => new Response('{}'))
  const request = authenticatedFetch(base, window.location.origin)
  selectServer('https://one.example', 'one-token')
  await request('/api/calendar')
  const first = base.mock.calls[0]
  expect(first[0]).toBe('https://one.example/api/calendar')
  expect(first[1].credentials).toBe('omit')
  expect(new Headers(first[1].headers).get('Authorization')).toBe('Bearer one-token')
  selectServer('https://two.example', 'two-token')
  expect(first[1].signal.aborted).toBe(true)
  await request('/api/settings/config', { method: 'PUT', body: '{}' })
  expect(base.mock.calls[1][0]).toBe('https://two.example/api/settings/config')
  expect(new Headers(base.mock.calls[1][1].headers).get('Authorization')).toBe('Bearer two-token')
  await request('https://outside.example/api/test')
  expect(base.mock.calls[2][1]).toBeUndefined()
  await connectionFetch(new URL('/api/connection', window.location.origin), { headers: { Authorization: 'Bearer local-token' } })
  expect(String(base.mock.calls[3][0])).toBe(window.location.origin + '/api/connection')
})

it('persists only profile names and origins after verification and confirmation', async () => {
  const base = vi.fn().mockImplementation(async () => new Response(JSON.stringify({ application: 'ChartNagari', protocol: 1, remote_access: true, capabilities: { calendar: true } })))
  authenticatedFetch(base, window.location.origin)
  vi.spyOn(window, 'confirm').mockReturnValue(true)
  const connected = vi.fn()
  render(<ConnectionPanel onConnect={connected} />)
  fireEvent.click(screen.getByText(/Server connections/, { selector: 'summary' }))
  fireEvent.change(screen.getByLabelText('Connection name'), { target: { value: 'Home' } })
  fireEvent.change(screen.getByLabelText('Server origin'), { target: { value: 'https://home.example' } })
  fireEvent.change(screen.getByLabelText('Server administrator API token'), { target: { value: 'private-token' } })
  fireEvent.click(screen.getByRole('button', { name: 'Verify and connect' }))
  await waitFor(() => expect(connected).toHaveBeenCalledOnce())
  expect(getServerOrigin()).toBe('https://home.example')
  expect(localStorage.getItem('chartnagari.connections.v1')).not.toContain('private-token')
  expect(sessionStorage.getItem('chartnagari.selected-server')).toBe('https://home.example')
  expect(screen.getByLabelText('Server administrator API token')).toHaveValue('')
})

it('leaves the active connection unchanged on verification failure', async () => {
  authenticatedFetch(vi.fn().mockResolvedValue(new Response('', { status: 401 })), window.location.origin)
  const connected = vi.fn()
  render(<ConnectionPanel onConnect={connected} connected={false} />)
  fireEvent.click(screen.getByRole('button', { name: 'Verify and connect' }))
  await screen.findByText(/Connection failed:/)
  expect(connected).not.toHaveBeenCalled()
  expect(getServerOrigin()).toBe(window.location.origin)
})
