import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

const DEMO_STATIC = import.meta.env.VITE_DEMO_STATIC === 'true'

type Direction = 'LONG' | 'SHORT'
interface Templates { symbol: string; long: string; short: string }
const empty = (symbol: string): Templates => ({ symbol, long: '', short: '' })

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, init)
  if (!response.ok) {
    let message = `HTTP ${response.status}`
    try {
      const body = await response.json() as { error?: string }
      if (body.error) message = body.error
    } catch { /* retain status */ }
    throw new Error(message)
  }
  return response.json() as Promise<T>
}

export function SymbolMessageTemplateEditor({ symbol }: { symbol: string }) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState<Templates>(() => empty(symbol))
  const [saved, setSaved] = useState<Templates>(() => empty(symbol))
  const [direction, setDirection] = useState<Direction>('LONG')
  const [loading, setLoading] = useState(!DEMO_STATIC)
  const [loadFailed, setLoadFailed] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [preview, setPreview] = useState('')
  const [sample, setSample] = useState('')
  const [previewOmitted, setPreviewOmitted] = useState(false)
  const [previewing, setPreviewing] = useState(false)
  const generation = useRef(0)
  const editVersion = useRef(0)
  const previewRequest = useRef(0)
  const endpoint = `/api/symbol-message-templates/${encodeURIComponent(symbol)}`

  useEffect(() => {
    if (DEMO_STATIC) return
    const current = ++generation.current
    const controller = new AbortController()
    editVersion.current = 0
    previewRequest.current++
    setDraft(empty(symbol)); setSaved(empty(symbol)); setDirection('LONG')
    setLoading(true); setLoadFailed(false); setSaving(false); setPreviewing(false); setError(''); setNotice(''); setPreview(''); setSample(''); setPreviewOmitted(false)
    request<Templates>(endpoint, { signal: controller.signal })
      .then(value => {
        if (current !== generation.current) return
        if (!value || value.symbol !== symbol || typeof value.long !== 'string' || typeof value.short !== 'string') {
          throw new Error('Invalid template response')
        }
        setDraft(value); setSaved(value)
      })
      .catch(e => {
        if (current === generation.current && !controller.signal.aborted) {
          setLoadFailed(true)
          setError(e instanceof Error ? e.message : String(e))
        }
      })
      .finally(() => { if (current === generation.current) setLoading(false) })
    return () => { controller.abort(); generation.current++ }
  }, [symbol, endpoint])

  const key = direction === 'LONG' ? 'long' : 'short'
  const update = (value: string) => {
    editVersion.current++
    previewRequest.current++
    setPreviewing(false)
    setDraft(prev => ({ ...prev, [key]: value }))
    setError(''); setNotice(''); setPreview(''); setSample(''); setPreviewOmitted(false)
  }
  const save = async () => {
    const current = generation.current
    const version = editVersion.current
    const payload = { long: draft.long, short: draft.short }
    previewRequest.current++
    setPreviewing(false)
    setSaving(true); setError(''); setNotice('')
    try {
      const value = await request<Templates>(endpoint, {
        method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload),
      })
      if (current !== generation.current) return
      setSaved(value)
      if (version === editVersion.current) setNotice(t('messageTemplate.saved'))
    } catch (e) {
      if (current === generation.current) setError(e instanceof Error ? e.message : String(e))
    } finally {
      if (current === generation.current) setSaving(false)
    }
  }
  const showPreview = async () => {
    const current = generation.current
    const version = editVersion.current
    const previewID = ++previewRequest.current
    const selected = direction
    setPreviewing(true); setError(''); setPreview(''); setSample(''); setPreviewOmitted(false)
    try {
      const value = await request<{ html: string; custom_included: boolean; sample: string }>(`${endpoint}/preview`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ direction: selected, template: draft[key] }),
      })
      if (current === generation.current && version === editVersion.current && previewID === previewRequest.current) {
        const DOMPurify = (await import('dompurify')).default
        if (current !== generation.current || version !== editVersion.current || previewID !== previewRequest.current) return
        setPreview(DOMPurify.sanitize(value.html)); setSample(value.sample); setPreviewOmitted(draft[key] !== '' && !value.custom_included)
      }
    } catch (e) {
      if (current === generation.current && version === editVersion.current && previewID === previewRequest.current) {
        setError(e instanceof Error ? e.message : String(e))
      }
    } finally {
      if (current === generation.current && version === editVersion.current && previewID === previewRequest.current) {
        setPreviewing(false)
      }
    }
  }
  const dirty = draft.long !== saved.long || draft.short !== saved.short

  if (DEMO_STATIC) return <p className="item-meta">{t('messageTemplate.demoUnavailable')}</p>

  return <section aria-label={t('messageTemplate.heading')} style={{ marginTop: 16, borderTop: '1px solid var(--muted)', paddingTop: 12 }}>
    <h3 style={{ margin: '0 0 6px', fontSize: '0.95rem' }}>{t('messageTemplate.heading')}</h3>
    <p className="item-meta">{t('messageTemplate.description')}</p>
    <div className="tab-group" role="tablist" aria-label={t('messageTemplate.direction')}>
      {(['LONG', 'SHORT'] as Direction[]).map(item => <button key={item} type="button" role="tab"
        aria-selected={direction === item} className={`tab-btn${direction === item ? ' active' : ''}`}
        onClick={() => { previewRequest.current++; setPreviewing(false); setDirection(item); setPreview(''); setSample(''); setPreviewOmitted(false); setError('') }}>{item}</button>)}
    </div>
    <label htmlFor={`message-template-${symbol}`}>{t('messageTemplate.editor', { direction })}</label>
    <textarea id={`message-template-${symbol}`} aria-label={t('messageTemplate.editor', { direction })}
      value={draft[key]} onChange={e => update(e.target.value)} disabled={loading || loadFailed || saving}
      rows={5} style={{ display: 'block', width: '100%', resize: 'vertical', marginTop: 6 }} />
    <p className="item-meta">{t('messageTemplate.length', { count: [...draft[key]].length })}</p>
    <p className="item-meta">{t('messageTemplate.tokens')}</p>
    <p className="item-meta">{t('messageTemplate.aliases')}</p>
    <p className="item-meta">{t('messageTemplate.sample')}{sample && ` ${sample}`}</p>
    <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
      <button type="button" onClick={() => update('')} disabled={loading || loadFailed || saving || draft[key] === ''}>{t('messageTemplate.reset')}</button>
      <button type="button" onClick={showPreview} disabled={loading || loadFailed || previewing}>{previewing ? t('loading') : t('messageTemplate.preview')}</button>
      <button type="button" className="run-btn" onClick={save} disabled={loading || loadFailed || saving || !dirty}>{saving ? t('saving') : t('messageTemplate.save')}</button>
    </div>
    {notice && <p role="status">{notice}</p>}
    {error && <p role="alert" className="error-msg">{error}</p>}
    {previewOmitted && <p role="status">{t('messageTemplate.omitted')}</p>}
    {preview && <div role="region" aria-label={t('messageTemplate.preview')} style={{ marginTop: 10, padding: 10, whiteSpace: 'pre-wrap', border: '1px solid var(--muted)', borderRadius: 4 }} dangerouslySetInnerHTML={{ __html: preview }} />}
  </section>
}
