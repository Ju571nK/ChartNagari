import { afterEach, expect, it, vi } from 'vitest'
import { authenticatedFetch, setSessionToken } from './apiAuth'

afterEach(() => setSessionToken(''))
it('attaches tokens only to same-origin API calls and preserves explicit credentials', async () => {
  const base = vi.fn().mockResolvedValue(new Response(null, { status: 204 }))
  const request = authenticatedFetch(base, 'http://localhost:8080')
  setSessionToken('test-session')
  await request('/api/settings/config', { method: 'PUT', body: '{}' })
  expect(new Headers(base.mock.calls[0][1].headers).get('Authorization')).toBe('Bearer test-session')
  expect(base.mock.calls[0][1].redirect).toBe('error')
  await request('https://external.example/api/save', { method: 'POST' })
  expect(base.mock.calls[1][1].headers).toBeUndefined()
  await request('/api/auth/check', { method: 'POST', headers: { Authorization: 'Bearer replacement' } })
  expect(new Headers(base.mock.calls[2][1].headers).get('Authorization')).toBe('Bearer replacement')
  setSessionToken('')
  await request('/api/settings/config')
  expect(new Headers(base.mock.calls[3][1].headers).has('Authorization')).toBe(false)
})
