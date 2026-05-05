import { useEffect, useRef, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'

const TIMEFRAMES = ['1H', '4H', '1D', '1W'] as const
type Timeframe = typeof TIMEFRAMES[number]

interface ProfileInfo {
  name: string
  allowed_methodologies: string[]
  blocked_methodologies: string[]
  allowed_rules: string[]
  alert_limit_per_day: number
  cooldown_hours: number
  score_threshold: number
}

interface FieldSource<T> {
  value: T
  source: 'override' | 'profile'
}

interface EffectiveResponse {
  symbol: string
  score_threshold: FieldSource<number>
  cooldown_hours: FieldSource<number>
  alert_limit_per_day: FieldSource<number>
  timeframes: FieldSource<string[] | null>
  allowed_rules: FieldSource<string[] | null>
}

interface OverrideState {
  score_threshold: number | null
  cooldown_hours: number | null
  alert_limit_per_day: number | null
  timeframes: Timeframe[] | null
  allowed_rules: string[] | null
}

interface Props {
  symbol: string
  profile: ProfileInfo
  apiToken?: string
}

const DEBOUNCE_MS = 500

async function apiFetch<T>(path: string, init?: RequestInit, token?: string): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(init?.headers as Record<string, string> | undefined),
  }
  if (token) headers['Authorization'] = `Bearer ${token}`
  const res = await fetch(path, { ...init, headers })
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  return res.json() as Promise<T>
}

export function SymbolOverrideEditor({ symbol, profile, apiToken }: Props) {
  const { t } = useTranslation()
  const [state, setState] = useState<OverrideState>({
    score_threshold: null,
    cooldown_hours: null,
    alert_limit_per_day: null,
    timeframes: null,
    allowed_rules: null,
  })
  const [savedAt, setSavedAt] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)
  const debounceTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const pendingFlush = useRef<OverrideState | null>(null)

  // Initial GET — populate state from current override row.
  useEffect(() => {
    let cancelled = false
    apiFetch<EffectiveResponse>(`/api/symbol-overrides/${encodeURIComponent(symbol)}`, undefined, apiToken)
      .then(eff => {
        if (cancelled) return
        setState({
          score_threshold: eff.score_threshold.source === 'override' ? eff.score_threshold.value : null,
          cooldown_hours: eff.cooldown_hours.source === 'override' ? eff.cooldown_hours.value : null,
          alert_limit_per_day: eff.alert_limit_per_day.source === 'override' ? eff.alert_limit_per_day.value : null,
          timeframes: eff.timeframes.source === 'override' ? (eff.timeframes.value as Timeframe[]) : null,
          allowed_rules: eff.allowed_rules.source === 'override' ? eff.allowed_rules.value : null,
        })
      })
      .catch(() => { /* leave defaults */ })
    return () => { cancelled = true }
  }, [symbol, apiToken])

  // Schedule a debounced PUT whenever state changes.
  const scheduleSave = useCallback((next: OverrideState) => {
    pendingFlush.current = next
    if (debounceTimer.current) clearTimeout(debounceTimer.current)
    debounceTimer.current = setTimeout(async () => {
      const payload = pendingFlush.current
      pendingFlush.current = null
      debounceTimer.current = null
      if (!payload) return
      try {
        await apiFetch<EffectiveResponse>(
          `/api/symbol-overrides/${encodeURIComponent(symbol)}`,
          { method: 'PUT', body: JSON.stringify(payload) },
          apiToken,
        )
        setSavedAt(Date.now())
        setError(null)
      } catch (e) {
        setError(e instanceof Error ? e.message : 'unknown')
      }
    }, DEBOUNCE_MS)
  }, [symbol, apiToken])

  // Flush on unmount with pending changes.
  // Uses fetch + keepalive (NOT sendBeacon — which forces POST and would 405 against our PUT route).
  useEffect(() => () => {
    // Tighten guard: require BOTH a live timer AND a pending payload.
    if (!debounceTimer.current || !pendingFlush.current) return
    clearTimeout(debounceTimer.current)
    const url = `/api/symbol-overrides/${encodeURIComponent(symbol)}`
    const body = JSON.stringify(pendingFlush.current)
    const headers: Record<string, string> = { 'Content-Type': 'application/json' }
    if (apiToken) headers['Authorization'] = `Bearer ${apiToken}`
    fetch(url, { method: 'PUT', body, headers, keepalive: true }).catch(() => { /* best-effort */ })
  }, [symbol, apiToken])

  const updateField = <K extends keyof OverrideState>(key: K, value: OverrideState[K]) => {
    setState(prev => {
      const next = { ...prev, [key]: value }
      scheduleSave(next)
      return next
    })
  }

  const resetField = (key: keyof OverrideState) => {
    updateField(key, null as never)
  }

  const resetAll = async () => {
    if (debounceTimer.current) clearTimeout(debounceTimer.current)
    pendingFlush.current = null
    try {
      await apiFetch<EffectiveResponse>(
        `/api/symbol-overrides/${encodeURIComponent(symbol)}`,
        { method: 'DELETE' },
        apiToken,
      )
      setState({
        score_threshold: null, cooldown_hours: null,
        alert_limit_per_day: null, timeframes: null, allowed_rules: null,
      })
      setSavedAt(Date.now())
    } catch (e) {
      setError(e instanceof Error ? e.message : 'unknown')
    }
  }

  const sliderField = (
    label: string,
    field: 'score_threshold' | 'cooldown_hours' | 'alert_limit_per_day',
    min: number,
    max: number,
    step: number,
    profileDefault: number,
  ) => {
    const overrideVal = state[field]
    const effective = overrideVal ?? profileDefault
    return (
      <div style={{ marginBottom: 12 }}>
        <label htmlFor={`f-${field}`} style={{ display: 'block', fontSize: '0.78rem', color: 'var(--muted)' }}>
          {label}
        </label>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <input
            id={`f-${field}`}
            type="range"
            min={min} max={max} step={step}
            value={effective}
            onChange={e => updateField(field, Number(e.target.value))}
            style={{ flex: 1 }}
          />
          <span style={{ minWidth: 48, textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>
            {effective}
          </span>
          {overrideVal !== null && (
            <button
              data-testid={`reset-${field}`}
              onClick={() => resetField(field)}
              style={{ background: 'transparent', border: 'none', cursor: 'pointer', color: 'var(--muted)' }}
              title={t('override.reset')}
            >↺</button>
          )}
        </div>
        <span style={{ fontSize: '0.7rem', color: 'var(--muted)' }}>
          {overrideVal !== null
            ? t('override.profile_default_value', { value: profileDefault })
            : t('override.profile_default')}
        </span>
      </div>
    )
  }

  const timeframeField = () => {
    const active = new Set(state.timeframes ?? [])
    return (
      <div style={{ marginBottom: 12 }}>
        <div style={{ fontSize: '0.78rem', color: 'var(--muted)', marginBottom: 4 }}>
          {t('override.timeframes')}
        </div>
        <div style={{ display: 'flex', gap: 6 }}>
          {TIMEFRAMES.map(tf => {
            const on = active.has(tf)
            return (
              <button
                key={tf}
                onClick={() => {
                  const next = new Set(active)
                  if (on) next.delete(tf)
                  else next.add(tf)
                  const arr = Array.from(next) as Timeframe[]
                  updateField('timeframes', arr.length > 0 ? arr : null)
                }}
                style={{
                  padding: '4px 10px',
                  border: `1px solid ${on ? 'var(--mint)' : 'var(--muted)'}`,
                  background: on ? 'var(--mint)' : 'transparent',
                  color: on ? 'var(--bg)' : 'var(--muted)',
                  borderRadius: 4, cursor: 'pointer', fontSize: '0.78rem',
                }}
              >{tf}</button>
            )
          })}
          {state.timeframes !== null && (
            <button
              data-testid="reset-timeframes"
              onClick={() => resetField('timeframes')}
              style={{ background: 'transparent', border: 'none', cursor: 'pointer', color: 'var(--muted)' }}
              title={t('override.reset')}
            >↺</button>
          )}
        </div>
      </div>
    )
  }

  return (
    <div style={{ padding: 12, background: 'rgba(91,146,121,0.04)', border: '1px solid rgba(91,146,121,0.15)', borderRadius: 6 }}>
      {sliderField(t('override.score_threshold'), 'score_threshold', 0, 50, 0.5, profile.score_threshold)}
      {sliderField(t('override.cooldown_hours'), 'cooldown_hours', 0, 168, 1, profile.cooldown_hours)}
      {sliderField(t('override.alert_limit'), 'alert_limit_per_day', 0, 20, 1, profile.alert_limit_per_day)}
      {timeframeField()}

      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: 12, fontSize: '0.72rem' }}>
        <span style={{ color: error ? 'var(--danger)' : 'var(--muted)' }}>
          {error ? `${t('override.save_failed')}: ${error}`
            : savedAt ? t('override.saved_ago', { n: Math.max(0, Math.round((Date.now() - savedAt) / 1000)) })
            : ''}
        </span>
        <button
          data-testid="reset-all"
          onClick={resetAll}
          style={{ background: 'transparent', border: '1px solid var(--muted)', borderRadius: 4, padding: '4px 8px', color: 'var(--muted)', cursor: 'pointer', fontSize: '0.7rem' }}
        >
          {t('override.reset_all')}
        </button>
      </div>
    </div>
  )
}
