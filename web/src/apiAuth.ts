// In-memory only. Never persist API tokens in local/session storage or URLs.
let sessionToken = ''
export function setSessionToken(token: string) { sessionToken = token }

let serverOrigin = ''
let transport: typeof fetch | undefined
export function connectionFetch(input: RequestInfo | URL, init?: RequestInit) { return (transport ?? fetch)(input, init) }
let generation = 0
let connectionAbort = new AbortController()
export function getServerOrigin() { return serverOrigin || window.location.origin }
export function selectServer(origin: string, token: string) {
  sessionStorage.setItem('chartnagari.selected-server', origin)
  connectionAbort.abort()
  connectionAbort = new AbortController()
  generation++
  serverOrigin = origin
  sessionToken = token
}
export function serverWebSocketURL() {
  const url = new URL('/ws', getServerOrigin())
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  return url.href
}

export function authenticatedFetch(baseFetch: typeof fetch, origin: string): typeof fetch {
  transport = baseFetch
  return (input, init) => {
    const rawURL = input instanceof Request ? input.url : String(input)
    const url = new URL(rawURL, origin)
    if (url.origin !== origin || !url.pathname.startsWith('/api/')) {
      return baseFetch(input, init)
    }
    const target = serverOrigin || origin
    const headers = new Headers(init?.headers ?? (input instanceof Request ? input.headers : undefined))
    if (sessionToken && !headers.has('Authorization')) headers.set('Authorization', `Bearer ${sessionToken}`)
    const sourceSignal = init?.signal ?? (input instanceof Request ? input.signal : undefined)
    const signal = sourceSignal ? AbortSignal.any([connectionAbort.signal, sourceSignal]) : connectionAbort.signal
    const epoch = generation
    const destination = new URL(url.pathname + url.search, target).href
    const request = input instanceof Request ? new Request(destination, input) : destination
    return baseFetch(request, { ...init, headers, signal, credentials: target === origin ? init?.credentials : 'omit', redirect: 'error' })
      .then(response => { if (epoch !== generation) throw new DOMException('Server changed', 'AbortError'); return response })
  }
}

export async function createServerSocket(): Promise<WebSocket> {
  const epoch = generation
  const address = serverWebSocketURL()
  const response = await fetch('/api/connection/ws-ticket', { method: 'POST' })
  if (!response.ok) {
    if ((response.status === 404 || response.status === 401) && getServerOrigin() === window.location.origin && epoch === generation) return new WebSocket(address)
    throw new Error('WebSocket authentication unavailable')
  }
  const { ticket } = await response.json()
  if (epoch !== generation) throw new DOMException('Server changed', 'AbortError')
  if (typeof ticket !== 'string' || !/^[a-f0-9]{64}$/.test(ticket)) throw new Error('Invalid WebSocket ticket')
  return new WebSocket(address, ['chartnagari.browser.v1', `ticket.${ticket}`])
}

export async function downloadAPI(path: string) {
  const epoch = generation
  const response = await fetch(path)
  if (!response.ok) throw new Error(`Download failed (${response.status})`)
  const blob = await response.blob()
  if (epoch !== generation) throw new DOMException('Server changed', 'AbortError')
  const href = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = href
  a.download = response.headers.get('Content-Disposition')?.match(/filename="?([^";]+)/)?.[1] || path.split('/').pop()?.split('?')[0] || 'download'
  a.click()
  setTimeout(() => URL.revokeObjectURL(href), 1000)
}
