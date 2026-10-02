import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { getServerOrigin, subscribeServerChange } from './apiAuth'
import OllamaSettings from './OllamaSettings'

type Kind = 'ollama' | 'openai-compatible'
type Profile = { id: string; name: string; kind: Kind; base_url: string; model: string; has_api_key: boolean; active: boolean }
type CatalogItem = { id: string; name: string; model: string; kind: Kind | string; capability: string; description: string; source_url: string; supported?: boolean }
type Setup = { profiles: Profile[]; catalog: CatalogItem[]; active_profile_id: string | null }
type TestResult = { ok: boolean; message: string; output: string; duration_ms: number }
type Draft = { id?: string; name: string; kind: Kind; base_url: string; model: string; api_key: string }
type Busy = 'save' | 'models' | 'pull' | 'test' | 'activate' | null

const localURL = 'http://127.0.0.1:11434'
const emptyDraft: Draft = { name: '', kind: 'ollama', base_url: localURL, model: '', api_key: '' }
const hfReference = /^hf\.co\/[A-Za-z0-9][\w.-]*\/[A-Za-z0-9][\w.-]*(?::[\w.-]+)?$/
const catalogCopy: Record<string, { capability: string; description: string }> = {
  'qwen3-4b': { capability: 'ai_setup.capability_text_generation', description: 'ai_setup.catalog_qwen3_4b' },
  'qwen3-8b': { capability: 'ai_setup.capability_text_generation', description: 'ai_setup.catalog_qwen3_8b' },
  'qwen3-14b': { capability: 'ai_setup.capability_text_generation', description: 'ai_setup.catalog_qwen3_14b' },
  'laya-typed-decisions': { capability: 'ai_setup.capability_typed_decision', description: 'ai_setup.catalog_laya' },
}

function safeSourceURL(value: string): string | null {
  try {
    const url = new URL(value)
    return url.protocol === 'http:' || url.protocol === 'https:' ? url.href : null
  } catch {
    return null
  }
}

function errorText(body: unknown, status: number): string {
  if (body && typeof body === 'object') {
    const message = (body as { error?: unknown; message?: unknown }).error ?? (body as { message?: unknown }).message
    if (typeof message === 'string' && message.trim()) return message
  }
  return `HTTP ${status}`
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(path, { credentials: 'include', ...init })
  const body: unknown = await response.json().catch(() => null)
  if (!response.ok) throw new Error(errorText(body, response.status))
  return body as T
}

export default function AISetupWizard() {
  const { t } = useTranslation()
  const [setup, setSetup] = useState<Setup | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [draft, setDraft] = useState<Draft>(emptyDraft)
  const [device, setDevice] = useState<'server' | 'remote'>('server')
  const [selection, setSelection] = useState('')
  const [models, setModels] = useState<string[] | null>(null)
  const [busy, setBusy] = useState<Busy>(null)
  const [testResult, setTestResult] = useState<TestResult | null>(null)
  const [testedSignature, setTestedSignature] = useState('')
  const [savedSignature, setSavedSignature] = useState('')
  const [clearApiKey, setClearApiKey] = useState(false)
  const [pullMessage, setPullMessage] = useState('')
  const [showOllamaHelp, setShowOllamaHelp] = useState(true)
  const operation = useRef<AbortController | null>(null)
  const sequence = useRef(0)
  const mounted = useRef(true)

  const signature = (value: Draft) => JSON.stringify([value.id, value.name.trim(), value.kind, value.base_url.trim(), value.model.trim()])
  const currentSignature = signature(draft)
  const saved = Boolean(draft.id && currentSignature === savedSignature && !draft.api_key && !clearApiKey)
  const tested = saved && testedSignature === currentSignature && testResult?.ok === true
  const activeProfile = setup?.profiles.find(profile => profile.id === setup.active_profile_id)
  const active = activeProfile?.id === draft.id && activeProfile?.active === true && saved

  const cancelOperation = useCallback(() => {
    sequence.current++
    operation.current?.abort()
    operation.current = null
    setBusy(null)
  }, [])

  const load = useCallback(async () => {
    cancelOperation()
    setLoading(true)
    setError('')
    const controller = new AbortController()
    operation.current = controller
    const ticket = ++sequence.current
    const server = getServerOrigin()
    try {
      const data = await request<Setup>('/api/ai/setup', { signal: controller.signal })
      if (ticket !== sequence.current || server !== getServerOrigin() || !mounted.current) return
      setSetup(data)
    } catch (e) {
      if (ticket === sequence.current && !controller.signal.aborted && mounted.current) setError((e as Error).message)
    } finally {
      if (ticket === sequence.current && mounted.current) { operation.current = null; setLoading(false) }
    }
  }, [cancelOperation])

  useEffect(() => {
    mounted.current = true
    void load()
    const unsubscribe = subscribeServerChange(() => {
      setSetup(null)
      setDraft(emptyDraft)
      setSelection('')
      setModels(null)
      setTestResult(null)
      setTestedSignature('')
      setSavedSignature('')
      setClearApiKey(false)
      void load()
    })
    return () => { mounted.current = false; cancelOperation(); unsubscribe() }
  }, [load, cancelOperation])

  const updateDraft = (patch: Partial<Draft>) => {
    if (Object.hasOwn(patch, 'id') && patch.id === undefined) setClearApiKey(false)
    cancelOperation()
    setDraft(previous => ({ ...previous, ...patch }))
    setTestResult(null)
    setTestedSignature('')
    setModels(null)
    setPullMessage('')
    setError('')
  }

  const chooseProfile = (profile: Profile) => {
    setSelection(`profile:${profile.id}`)
    setDevice(profile.kind === 'ollama' && profile.base_url === localURL ? 'server' : 'remote')
    updateDraft({ id: profile.id, name: profile.name, kind: profile.kind, base_url: profile.base_url, model: profile.model, api_key: '' })
    setSavedSignature(signature({ id: profile.id, name: profile.name, kind: profile.kind, base_url: profile.base_url, model: profile.model, api_key: '' }))
    setClearApiKey(false)
  }

  const chooseCatalog = (item: CatalogItem) => {
    if (item.supported === false || (item.kind !== 'ollama' && item.kind !== 'openai-compatible')) return
    setSelection(`catalog:${item.id}`)
    setClearApiKey(false)
    if (item.kind === 'openai-compatible') setDevice('remote')
    updateDraft({ id: undefined, name: item.name, kind: item.kind, model: item.model, base_url: item.kind === 'openai-compatible' && device === 'server' ? '' : draft.base_url, api_key: '' })
    setSavedSignature('')
  }

  const run = async <T,>(action: Exclude<Busy, null>, path: string, init: RequestInit, done: (body: T) => void) => {
    cancelOperation()
    setBusy(action)
    setError('')
    const controller = new AbortController()
    operation.current = controller
    const ticket = ++sequence.current
    const server = getServerOrigin()
    try {
      const body = await request<T>(path, { ...init, signal: controller.signal })
      if (ticket !== sequence.current || server !== getServerOrigin() || !mounted.current) return
      done(body)
    } catch (e) {
      if (ticket === sequence.current && !controller.signal.aborted && mounted.current) setError((e as Error).message)
    } finally {
      if (ticket === sequence.current && mounted.current) { setBusy(null); operation.current = null }
    }
  }

  const save = () => {
    if (!draft.name.trim() || !draft.base_url.trim() || !draft.model.trim()) { setError(t('ai_setup.required')); return }
    if ((selection === 'custom' || draft.model.startsWith('hf.co/')) && !hfReference.test(draft.model)) { setError(t('ai_setup.invalid_hf')); return }
    const snapshot = { ...draft }
    setTestResult(null)
    setTestedSignature('')
    void run<Profile>('save', '/api/ai/profiles', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id: snapshot.id, name: snapshot.name.trim(), kind: snapshot.kind, base_url: snapshot.base_url.trim(), model: snapshot.model.trim(), ...(snapshot.api_key ? { api_key: snapshot.api_key } : {}), ...(clearApiKey ? { clear_api_key: true } : {}) }),
    }, profile => {
      const clean = { id: profile.id, name: profile.name, kind: profile.kind, base_url: profile.base_url, model: profile.model, api_key: '' }
      setDraft(clean)
      setClearApiKey(false)
      setSavedSignature(signature(clean))
      setSetup(previous => previous && ({ ...previous, profiles: [...previous.profiles.filter(p => p.id !== profile.id), profile] }))
      setSelection(`profile:${profile.id}`)
      setModels(null)
    })
  }

  const listModels = () => {
    if (!saved || !draft.id) return
    void run<{ models: string[] }>('models', `/api/ai/profiles/${encodeURIComponent(draft.id)}/models`, {}, body => setModels(body.models))
  }
  const pull = () => {
    if (!saved || !draft.id) return
    setPullMessage('')
    void run<{ ok?: boolean; message?: string }>('pull', `/api/ai/profiles/${encodeURIComponent(draft.id)}/pull`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ model: draft.model }),
    }, body => { if (body?.ok === false) { setError(body.message || t('ai_setup.download_failed')); return } setPullMessage(t('ai_setup.download_complete')); setModels(null) })
  }
  const test = () => {
    if (!saved || !draft.id) return
    const fingerprint = currentSignature
    setTestResult(null)
    setTestedSignature('')
    void run<TestResult>('test', `/api/ai/profiles/${encodeURIComponent(draft.id)}/test`, { method: 'POST' }, body => {
      if (body.ok && body.output?.trim()) {
        setTestResult(body)
        setTestedSignature(fingerprint)
      } else {
        const message = body.ok ? t('ai_setup.no_output') : body.message || t('ai_setup.test_failed')
        setTestResult({ ...body, ok: false, message })
        setError(message)
      }
    })
  }
  const activate = () => {
    if (!tested || !draft.id) return
    void run<{ ok: boolean }>('activate', `/api/ai/profiles/${encodeURIComponent(draft.id)}/activate`, { method: 'POST' }, body => {
      if (!body.ok) { setError(t('ai_setup.activate_failed')); return }
      setSetup(previous => previous && ({ ...previous, active_profile_id: draft.id!, profiles: previous.profiles.map(p => ({ ...p, active: p.id === draft.id })) }))
    })
  }

  return <section className="ai-setup" aria-label={t('ai_setup.title')}>
    <h3>{t('ai_setup.title')}</h3>
    <p className="ai-setup-muted">{t('ai_setup.intro')}</p>
    {loading && <p role="status">{t('ai_setup.loading')}</p>}
    {error && <div className="ai-setup-error" role="alert">{error} <button type="button" onClick={() => { if (!setup) void load(); else setError('') }}>{t(setup ? 'ai_setup.dismiss' : 'ai_setup.retry')}</button></div>}
    {setup && <>
      {activeProfile && <p className="ai-setup-active">{t('ai_setup.active')}: {activeProfile.name}{(!activeProfile.active || (activeProfile.id === draft.id && !saved)) && <> · {t('ai_setup.active_snapshot_edited')}</>}</p>}
      {setup.profiles.length > 0 && <div className="ai-setup-field"><label htmlFor="ai-existing">{t('ai_setup.saved_profiles')}</label><select id="ai-existing" value={selection.startsWith('profile:') ? selection.slice('profile:'.length) : ''} onChange={e => { const profile = setup.profiles.find(p => p.id === e.target.value); if (profile) chooseProfile(profile) }}><option value="">{t('ai_setup.choose_profile')}</option>{setup.profiles.map(p => <option key={p.id} value={p.id}>{p.name}{p.active ? ` (${t('ai_setup.active')})` : ''}</option>)}</select></div>}
      <div className="ai-setup-field"><span>{t('ai_setup.install_device')}</span><p className="ai-setup-muted">{t('ai_setup.device_explain')}</p><div className="ai-setup-options"><button type="button" className={device === 'server' ? 'selected' : ''} onClick={() => { setDevice('server'); setSelection(''); updateDraft({ id: undefined, kind: 'ollama', base_url: localURL, api_key: '' }); setSavedSignature('') }}>{t('ai_setup.this_server')}</button><button type="button" className={device === 'remote' ? 'selected' : ''} onClick={() => { setDevice('remote'); setSelection(''); updateDraft({ id: undefined, base_url: '', api_key: '' }); setSavedSignature('') }}>{t('ai_setup.remote_server')}</button></div></div>
      <div className="ai-setup-field"><span>{t('ai_setup.model_choice')}</span><div className="ai-setup-catalog">{setup.catalog.map(item => { const sourceURL = safeSourceURL(item.source_url); const copy = catalogCopy[item.id]; return <div className="ai-setup-catalog-item" key={item.id}><button type="button" className={selection === `catalog:${item.id}` ? 'selected' : ''} disabled={item.supported === false || (item.kind !== 'ollama' && item.kind !== 'openai-compatible')} onClick={() => chooseCatalog(item)}><strong>{item.name}</strong><small>{t(copy?.capability ?? item.capability)} · {item.kind}</small><span>{t(copy?.description ?? item.description)}</span>{(item.id.toLowerCase().includes('laya') || item.name.toLowerCase().includes('laya')) && <em>{t('ai_setup.laya_note')}</em>}{item.supported === false && <em>{t('ai_setup.experimental')}</em>}</button>{sourceURL && <a className="ai-setup-source" href={sourceURL} target="_blank" rel="noopener noreferrer">{t('ai_setup.source', { name: item.name })}</a>}</div> })}</div><p className="ai-setup-muted">{t('ai_setup.catalog_note')}</p></div>
      <div className="ai-setup-field"><label htmlFor="ai-hf">{t('ai_setup.custom_hf')}</label><input id="ai-hf" value={selection === 'custom' || draft.model.startsWith('hf.co/') ? draft.model : ''} placeholder="hf.co/owner/repo" onChange={e => { setSelection('custom'); updateDraft({ id: undefined, kind: 'ollama', model: e.target.value }); setSavedSignature('') }} /><small>{t('ai_setup.hf_explain')}</small></div>
      <div className="ai-setup-grid"><div className="ai-setup-field"><label htmlFor="ai-name">{t('ai_setup.profile_name')}</label><input id="ai-name" value={draft.name} onChange={e => updateDraft({ name: e.target.value })} /></div><div className="ai-setup-field"><label htmlFor="ai-kind">{t('ai_setup.provider')}</label><select id="ai-kind" value={draft.kind} onChange={e => updateDraft({ kind: e.target.value as Kind })}><option value="ollama">Ollama</option><option value="openai-compatible">OpenAI compatible</option></select></div><div className="ai-setup-field"><label htmlFor="ai-url">{t('ai_setup.endpoint')}</label><input id="ai-url" type="url" value={draft.base_url} placeholder="https://server.example/v1" onChange={e => updateDraft({ base_url: e.target.value })} /></div><div className="ai-setup-field"><label htmlFor="ai-model">{t('ai_setup.model_reference')}</label><input id="ai-model" value={draft.model} onChange={e => { setSelection(''); updateDraft({ model: e.target.value }) }} /></div><div className="ai-setup-field"><label htmlFor="ai-key">{t('ai_setup.api_key')}</label><input id="ai-key" type="password" autoComplete="new-password" value={draft.api_key} placeholder={setup.profiles.find(p => p.id === draft.id)?.has_api_key ? t('ai_setup.key_saved') : ''} onChange={e => updateDraft({ api_key: e.target.value })} /><small>{t('ai_setup.key_help')}</small>{setup.profiles.find(p => p.id === draft.id)?.has_api_key && <label className="ai-setup-checkbox"><input type="checkbox" checked={clearApiKey} onChange={e => setClearApiKey(e.target.checked)} />{t('ai_setup.clear_api_key')}</label>}</div></div>
      <div className="ai-setup-actions"><button type="button" onClick={save} disabled={!!busy || saved}>{busy === 'save' ? t('ai_setup.saving') : t('ai_setup.save')}</button>{saved && !active && <span className="ai-setup-muted">{t('ai_setup.saved_inactive')}</span>}</div>
      {device === 'server' && <details className="ai-setup-help" open={showOllamaHelp} onToggle={event => setShowOllamaHelp(event.currentTarget.open)}><summary>{t('ai_setup.ollama_help')}</summary>{showOllamaHelp && <OllamaSettings installationOnly />}</details>}
      {saved && <div className="ai-setup-verify"><h4>{t('ai_setup.verify')}</h4><div className="ai-setup-actions"><button type="button" onClick={listModels} disabled={!!busy}>{t('ai_setup.list_models')}</button>{draft.kind === 'ollama' && <button type="button" onClick={pull} disabled={!!busy}>{busy === 'pull' ? t('ai_setup.downloading') : t('ai_setup.download_model')}</button>}{busy === 'pull' && <button type="button" onClick={cancelOperation}>{t('ai_setup.cancel')}</button>}</div>{models && <p className="ai-setup-muted">{t('ai_setup.available_models')}: {models.length ? models.join(', ') : t('ai_setup.none')}</p>}{pullMessage && <p role="status">{pullMessage}</p>}<div className="ai-setup-actions"><button type="button" onClick={test} disabled={!!busy}>{busy === 'test' ? t('ai_setup.testing') : t('ai_setup.test')}</button>{tested && <button type="button" onClick={activate} disabled={!!busy || active}>{active ? t('ai_setup.active') : t('ai_setup.activate')}</button>}</div>{testResult && <div className={tested ? 'ai-setup-success' : 'ai-setup-error'} role="status"><strong>{testResult.ok ? t('ai_setup.test_ok') : testResult.message || t('ai_setup.test_failed')}</strong>{Number.isFinite(testResult.duration_ms) && <span>{t('ai_setup.latency')}: {testResult.duration_ms} ms</span>}{testResult.output && <div><span>{t('ai_setup.sample_output')}</span><pre>{testResult.output}</pre></div>}</div>}</div>}
    </>}
  </section>
}
