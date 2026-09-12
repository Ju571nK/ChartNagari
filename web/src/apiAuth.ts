// In-memory only. Never persist API tokens in local/session storage or URLs.
let sessionToken = ''
export function setSessionToken(token: string) { sessionToken = token }

export function authenticatedFetch(baseFetch: typeof fetch, origin: string): typeof fetch {
  return (input, init) => {
    const rawURL = input instanceof Request ? input.url : String(input)
    const url = new URL(rawURL, origin)
    if (!sessionToken || url.origin !== origin || !url.pathname.startsWith('/api/')) {
      return baseFetch(input, init)
    }
    const headers = new Headers(init?.headers ?? (input instanceof Request ? input.headers : undefined))
    if (!headers.has('Authorization')) headers.set('Authorization', `Bearer ${sessionToken}`)
    return baseFetch(input, { ...init, headers, redirect: 'error' })
  }
}
