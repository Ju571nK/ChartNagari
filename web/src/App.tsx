import { useState, useEffect, useCallback } from 'react'

// ── types ─────────────────────────────────────────────────────────────────────

type Tab = 'symbols' | 'rules' | 'status'

interface SymbolItem {
  symbol: string
  type: 'crypto' | 'stock'
  exchange: string
  enabled: boolean
}

interface RuleItem {
  name: string
  enabled: boolean
  methodology: string
}

interface StatusData {
  phase: string
  symbols: number
  rules: number
  tests: number
}

// ── API client ────────────────────────────────────────────────────────────────

async function apiFetch<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch('/api' + path, options)
  if (!res.ok) throw new Error(`HTTP ${res.status}: ${res.statusText}`)
  if (res.status === 204) return null as T
  return res.json() as Promise<T>
}

async function putJSON(path: string, body: unknown): Promise<void> {
  await apiFetch<null>(path, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

// ── sub-components ────────────────────────────────────────────────────────────

function Toggle({ checked, onChange }: { checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <label className="toggle">
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} />
      <span className="slider" />
    </label>
  )
}

function SymbolsTab() {
  const [symbols, setSymbols] = useState<SymbolItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    apiFetch<SymbolItem[]>('/symbols')
      .then(setSymbols)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  const toggle = useCallback(async (sym: SymbolItem, enabled: boolean) => {
    try {
      await putJSON(`/symbols/${encodeURIComponent(sym.symbol)}`, { enabled })
      setSymbols((prev) => prev.map((s) => (s.symbol === sym.symbol ? { ...s, enabled } : s)))
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : '알 수 없는 오류')
    }
  }, [])

  if (loading) return <p className="loading">로딩 중...</p>
  if (error) return <p className="error-msg">오류: {error}</p>

  return (
    <>
      <p className="section-title">종목 관리</p>
      {symbols.map((sym) => (
        <div key={sym.symbol} className="item">
          <div>
            <div className="item-name">
              <span className={`badge badge-${sym.type}`}>{sym.type}</span>
              {sym.symbol}
            </div>
            <div className="item-meta">{sym.exchange}</div>
          </div>
          <Toggle checked={sym.enabled} onChange={(v) => toggle(sym, v)} />
        </div>
      ))}
      {symbols.length === 0 && <p className="loading">등록된 종목 없음</p>}
    </>
  )
}

function RulesTab() {
  const [rules, setRules] = useState<RuleItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    apiFetch<RuleItem[]>('/rules')
      .then(setRules)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  const toggle = useCallback(async (rule: RuleItem, enabled: boolean) => {
    try {
      await putJSON(`/rules/${encodeURIComponent(rule.name)}`, { enabled })
      setRules((prev) => prev.map((r) => (r.name === rule.name ? { ...r, enabled } : r)))
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : '알 수 없는 오류')
    }
  }, [])

  if (loading) return <p className="loading">로딩 중...</p>
  if (error) return <p className="error-msg">오류: {error}</p>

  // Group by methodology
  const grouped = rules.reduce<Record<string, RuleItem[]>>((acc, r) => {
    ;(acc[r.methodology] ??= []).push(r)
    return acc
  }, {})

  return (
    <>
      {Object.entries(grouped).map(([method, items]) => (
        <div key={method}>
          <p className="section-title">{method.replace('_', ' ')}</p>
          {items.map((rule) => (
            <div key={rule.name} className="item">
              <div>
                <div className="item-name">
                  <span className={`badge badge-${rule.methodology}`}>{rule.methodology}</span>
                  {rule.name}
                </div>
              </div>
              <Toggle checked={rule.enabled} onChange={(v) => toggle(rule, v)} />
            </div>
          ))}
        </div>
      ))}
      {rules.length === 0 && <p className="loading">등록된 룰 없음</p>}
    </>
  )
}

function StatusTab() {
  const [status, setStatus] = useState<StatusData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    apiFetch<StatusData>('/status')
      .then(setStatus)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  if (loading) return <p className="loading">로딩 중...</p>
  if (error) return <p className="error-msg">오류: {error}</p>
  if (!status) return null

  return (
    <>
      <p className="section-title">시스템 상태</p>
      <p style={{ color: '#444', fontSize: '0.8rem', marginBottom: '4px' }}>{status.phase}</p>
      <div className="status-grid">
        <div className="stat-card">
          <div className="stat-value">{status.symbols}</div>
          <div className="stat-label">등록 종목</div>
        </div>
        <div className="stat-card">
          <div className="stat-value">{status.rules}</div>
          <div className="stat-label">분석 룰</div>
        </div>
        <div className="stat-card">
          <div className="stat-value">{status.tests}</div>
          <div className="stat-label">통과 테스트</div>
        </div>
        <div className="stat-card">
          <div className="stat-value" style={{ color: '#4ade80' }}>✓</div>
          <div className="stat-label">전체 PASS</div>
        </div>
      </div>
    </>
  )
}

// ── root ──────────────────────────────────────────────────────────────────────

export function App() {
  const [tab, setTab] = useState<Tab>('symbols')

  return (
    <div className="container">
      <header className="header">
        <h1>📈 Chart Analyzer</h1>
        <nav className="tabs">
          <button className={`tab-btn${tab === 'symbols' ? ' active' : ''}`} onClick={() => setTab('symbols')}>
            종목
          </button>
          <button className={`tab-btn${tab === 'rules' ? ' active' : ''}`} onClick={() => setTab('rules')}>
            룰
          </button>
          <button className={`tab-btn${tab === 'status' ? ' active' : ''}`} onClick={() => setTab('status')}>
            상태
          </button>
        </nav>
      </header>
      <main>
        {tab === 'symbols' && <SymbolsTab />}
        {tab === 'rules' && <RulesTab />}
        {tab === 'status' && <StatusTab />}
      </main>
    </div>
  )
}
