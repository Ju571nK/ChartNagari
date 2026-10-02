import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import AISetupWizard from './AISetupWizard'
import i18n from './i18n'
import { selectServer } from './apiAuth'

vi.mock('./OllamaSettings', () => ({ default: () => <div>Ollama install controls</div> }))

const catalog = [{ id: 'small', name: 'Sample model', model: 'sample:latest', kind: 'ollama', capability: 'general', description: 'A model option', source_url: 'https://example.test/model', supported: true }, { id: 'laya', name: 'Laya', model: 'laya', kind: 'laya', capability: 'decisions', description: 'Typed decisions', source_url: 'https://example.test/laya', supported: false }, { id: 'unsafe', name: 'Unsafe source', model: 'x', kind: 'ollama', capability: 'general', description: 'Unsafe URL test', source_url: 'javascript:alert(1)', supported: false }]
const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status, headers: { 'Content-Type': 'application/json' } })
const setup = { profiles: [], catalog, active_profile_id: null }
let calls: Array<{ url: string; init?: RequestInit }> = []

beforeEach(async () => {
  await i18n.changeLanguage('en')
  calls = []
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    calls.push({ url, init })
    if (url === '/api/ai/setup') return json(setup)
    if (url === '/api/ai/profiles') return json({ id: 'p1', name: 'Sample model', kind: 'ollama', base_url: 'http://127.0.0.1:11434', model: 'sample:latest', has_api_key: false, active: false })
    if (url.endsWith('/test')) return json({ ok: true, message: 'Sample generated', output: 'A genuine sample', duration_ms: 42 })
    if (url.endsWith('/activate')) return json({ ok: true })
    if (url.endsWith('/models')) return json({ models: ['sample:latest'] })
    if (url.endsWith('/pull')) return json({ ok: true })
    throw new Error(`Unexpected request: ${url}`)
  }))
})
afterEach(() => { cleanup(); vi.unstubAllGlobals(); selectServer('', ''); vi.restoreAllMocks() })

async function chooseAndSave(user: ReturnType<typeof userEvent.setup>) {
  await screen.findByText('Sample model')
  await user.click(screen.getByRole('button', { name: /Sample model.*general/s }))
  await user.click(screen.getByRole('button', { name: 'Save profile' }))
  await screen.findByText('Saved. Test this configuration before activation.')
}

describe('AI setup wizard', () => {
  it('saves an inactive profile and activates only after a sample output test', async () => {
    const user = userEvent.setup()
    render(<AISetupWizard />)
    await chooseAndSave(user)
    expect(screen.queryByRole('button', { name: 'Activate profile' })).not.toBeInTheDocument()
    expect(calls.some(call => call.url.endsWith('/activate'))).toBe(false)
    await user.click(screen.getByRole('button', { name: 'List models' }))
    expect(await screen.findByText(/Available models: sample:latest/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Test sample response' }))
    expect(await screen.findByText('A genuine sample')).toBeInTheDocument()
    expect(screen.getByText('Sample response passed')).toBeInTheDocument()
    expect(screen.queryByText('Sample generated')).not.toBeInTheDocument()
    expect(screen.getByText('Latency: 42 ms')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Activate profile' }))
    expect(await screen.findByText('Active profile: Sample model')).toBeInTheDocument()
    expect(calls.filter(call => call.url.endsWith('/activate'))).toHaveLength(1)
  })

  it('keeps activation unavailable after a failed test and allows retry', async () => {
    const base = globalThis.fetch
    vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
      if (url.endsWith('/test')) return json({ ok: false, message: 'Model refused request', output: '', duration_ms: 24 })
      return base(url, init)
    }))
    const user = userEvent.setup()
    render(<AISetupWizard />)
    await chooseAndSave(user)
    await user.click(screen.getByRole('button', { name: 'Test sample response' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Model refused request')
    expect(screen.queryByRole('button', { name: 'Activate profile' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Test sample response' })).toBeEnabled()
  })

  it('does not count a reachable endpoint without sample output as tested', async () => {
    const base = globalThis.fetch
    vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) =>
      url.endsWith('/test') ? json({ ok: true, message: 'Connected', output: '', duration_ms: 12 }) : base(url, init)))
    const user = userEvent.setup()
    render(<AISetupWizard />)
    await chooseAndSave(user)
    await user.click(screen.getByRole('button', { name: 'Test sample response' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('The test returned no sample output.')
    expect(screen.queryByRole('button', { name: 'Activate profile' })).not.toBeInTheDocument()
  })

  it('sends API keys only in the save request and masks the field afterward', async () => {
    const user = userEvent.setup()
    render(<AISetupWizard />)
    await screen.findByText('Sample model')
    await user.click(screen.getByRole('button', { name: /Sample model.*general/s }))
    await user.type(screen.getByLabelText('API key'), 'fixture-secret')
    expect(screen.getByLabelText('API key')).toHaveAttribute('type', 'password')
    await user.click(screen.getByRole('button', { name: 'Save profile' }))
    await screen.findByText('Saved. Test this configuration before activation.')
    const saveCall = calls.find(call => call.url === '/api/ai/profiles')
    expect(JSON.parse(String(saveCall?.init?.body)).api_key).toBe('fixture-secret')
    expect(screen.getByLabelText('API key')).toHaveValue('')
    expect(Object.keys(localStorage).map(key => localStorage.getItem(key))).not.toContain('fixture-secret')
  })

  it('clears an existing API key when saving a changed endpoint', async () => {
    const keyedSetup = {
      ...setup,
      active_profile_id: 'cloud',
      profiles: [{ id: 'cloud', name: 'Cloud', kind: 'openai-compatible', base_url: 'https://old.example/v1', model: 'model-x', has_api_key: true, active: true }],
    }
    const base = globalThis.fetch
    vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
      if (url === '/api/ai/setup') return json(keyedSetup)
      if (url === '/api/ai/profiles') { calls.push({ url, init }); return json({ ...keyedSetup.profiles[0], base_url: 'https://new.example/v1', active: false }) }
      return base(url, init)
    }))
    const user = userEvent.setup()
    render(<AISetupWizard />)
    await screen.findByText('Sample model')
    await user.selectOptions(screen.getByLabelText('Saved profiles'), 'cloud')
    await user.clear(screen.getByLabelText('Endpoint URL'))
    await user.type(screen.getByLabelText('Endpoint URL'), 'https://new.example/v1')
    const clearKey = screen.getByRole('checkbox', { name: 'Clear saved API key on save' })
    await user.click(clearKey)
    await user.click(screen.getByRole('button', { name: 'Save profile' }))
    await screen.findByText('Saved. Test this configuration before activation.')
    expect(screen.getByText(/Active profile: Cloud · the previously activated settings remain in use/)).toBeInTheDocument()
    const saveCall = calls.find(call => call.url === '/api/ai/profiles')
    const payload = JSON.parse(String(saveCall?.init?.body))
    expect(payload).toMatchObject({ id: 'cloud', base_url: 'https://new.example/v1', clear_api_key: true })
    expect(payload).not.toHaveProperty('api_key')
  })

  it('shows that previous activated settings remain in use after loading an inactive active profile', async () => {
    const loadedSetup = {
      ...setup,
      active_profile_id: 'previous',
      profiles: [{ id: 'previous', name: 'Previous config', kind: 'ollama' as const, base_url: 'http://127.0.0.1:11434', model: 'old:latest', has_api_key: false, active: false }],
    }
    vi.stubGlobal('fetch', vi.fn(async (url: string) => url === '/api/ai/setup' ? json(loadedSetup) : json({})))
    render(<AISetupWizard />)
    expect(await screen.findByText(/Active profile: Previous config · the previously activated settings remain in use/)).toBeInTheDocument()
  })

  it('invalidates an in-flight test when the configuration changes', async () => {
    let resolveTest!: (response: Response) => void
    const base = globalThis.fetch
    vi.stubGlobal('fetch', vi.fn((url: string, init?: RequestInit) => {
      if (url.endsWith('/test')) return new Promise<Response>(resolve => { resolveTest = resolve })
      return base(url, init)
    }))
    const user = userEvent.setup()
    render(<AISetupWizard />)
    await chooseAndSave(user)
    await user.click(screen.getByRole('button', { name: 'Test sample response' }))
    await user.clear(screen.getByLabelText('Model reference'))
    await user.type(screen.getByLabelText('Model reference'), 'different:latest')
    resolveTest(json({ ok: true, message: 'Old test', output: 'Old output', duration_ms: 3 }))
    await waitFor(() => expect(screen.queryByText('Old output')).not.toBeInTheDocument())
    expect(screen.queryByRole('button', { name: 'Activate profile' })).not.toBeInTheDocument()
  })

  it('aborts an in-progress model download without reporting completion', async () => {
    let pullSignal: AbortSignal | undefined
    const base = globalThis.fetch
    vi.stubGlobal('fetch', vi.fn((url: string, init?: RequestInit) => {
      if (url.endsWith('/pull')) {
        pullSignal = init?.signal ?? undefined
        return new Promise<Response>((_, reject) => pullSignal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError'))))
      }
      return base(url, init)
    }))
    const user = userEvent.setup()
    render(<AISetupWizard />)
    await chooseAndSave(user)
    await user.click(screen.getByRole('button', { name: 'Download model' }))
    expect(screen.getByRole('button', { name: 'Downloading on server…' })).toBeDisabled()
    await user.click(screen.getByRole('button', { name: 'Cancel request' }))
    expect(pullSignal?.aborted).toBe(true)
    expect(screen.queryByText('Download completed. Refresh the model list and run a test.')).not.toBeInTheDocument()
  })

  it('shows server errors and lets setup reload', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(json({ error: 'Setup unavailable' }, 503)).mockResolvedValueOnce(json(setup)))
    const user = userEvent.setup()
    render(<AISetupWizard />)
    expect(await screen.findByRole('alert')).toHaveTextContent('Setup unavailable')
    await user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('Sample model')).toBeInTheDocument()
  })

  it('discards an old server test response when the selected workspace changes', async () => {
    let resolveTest!: (response: Response) => void
    let setupCount = 0
    const base = globalThis.fetch
    vi.stubGlobal('fetch', vi.fn((url: string, init?: RequestInit) => {
      if (url === '/api/ai/setup') return Promise.resolve(json(setupCount++ === 0 ? setup : { profiles: [], catalog: [], active_profile_id: null }))
      if (url.endsWith('/test')) return new Promise<Response>(resolve => { resolveTest = resolve })
      return base(url, init)
    }))
    const user = userEvent.setup()
    render(<AISetupWizard />)
    await chooseAndSave(user)
    await user.click(screen.getByRole('button', { name: 'Test sample response' }))
    await act(async () => { selectServer('https://other.example', 'other-token') })
    await waitFor(() => expect(screen.queryByText('Saved. Test this configuration before activation.')).not.toBeInTheDocument())
    await act(async () => { resolveTest(json({ ok: true, message: 'Old', output: 'Stale sample', duration_ms: 1 })) })
    expect(screen.queryByText('Stale sample')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Activate profile' })).not.toBeInTheDocument()
  })

  it('disables unsupported Laya and accepts a custom Hugging Face reference without claiming activation', async () => {
    const user = userEvent.setup()
    render(<AISetupWizard />)
    expect(await screen.findByRole('button', { name: /Laya.*decisions/s })).toBeDisabled()
    const layaSource = screen.getByRole('link', { name: 'Source: Laya' })
    expect(layaSource).toHaveAttribute('href', 'https://example.test/laya')
    expect(layaSource).toHaveAttribute('target', '_blank')
    expect(layaSource).toHaveAttribute('rel', 'noopener noreferrer')
    expect(screen.queryByRole('link', { name: 'Source: Unsafe source' })).not.toBeInTheDocument()
    await user.type(screen.getByLabelText('Custom Hugging Face GGUF reference'), 'hf.co/owner/repo')
    expect(screen.getByLabelText('Model reference')).toHaveValue('hf.co/owner/repo')
    expect(screen.queryByRole('button', { name: 'Activate profile' })).not.toBeInTheDocument()
  })
})
