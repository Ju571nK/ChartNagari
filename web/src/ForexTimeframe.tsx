import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { Timeframe } from './Workspace'
import { DEMO_STATIC } from './demoApi'

export interface TimeframeSymbol { symbol: string; type: string }

// Wait for the symbol inventory before normalizing a direct link or symbol switch.
export function useForexTimeframe(symbol: string, timeframe: Timeframe, setTimeframe: (tf: Timeframe) => void, symbols: TimeframeSymbol[]) {
  useEffect(() => {
    const selected = symbols.find(item => item.symbol === symbol)
    if (selected && selected.type !== 'forex' && timeframe === '15M') setTimeframe('1H')
  }, [symbol, timeframe, setTimeframe, symbols])
}

async function collectionTimeframes(): Promise<string[]> {
  const response = await fetch('/api/watchlist/timeframes')
  if (!response.ok) throw new Error(`HTTP ${response.status}`)
  const value = await response.json() as { timeframes?: unknown }
  if (!Array.isArray(value.timeframes) || !value.timeframes.every(tf => typeof tf === 'string')) throw new Error('Invalid collection timeframes')
  return value.timeframes
}

export function Forex15MCollection({ visible, onEnabled }: { visible: boolean; onEnabled?: () => void }) {
  const { t } = useTranslation()
  const [enabled, setEnabled] = useState<boolean | null>(null)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => {
    if (!visible || DEMO_STATIC) return
    let active = true
    setEnabled(null)
    setError('')
    collectionTimeframes().then(tfs => { if (active) setEnabled(tfs.includes('15M')) }).catch(e => { if (active) setError(String(e)) })
    return () => { active = false }
  }, [visible])
  if (!visible) return null
  if (DEMO_STATIC) return <span className="item-meta">{t('forex15m.demo')}</span>
  const enable = async () => {
    setSaving(true)
    setError('')
    try {
      // Read again before writing so other timeframe choices are preserved.
      const current = await collectionTimeframes()
      if (!current.includes('15M')) {
        const response = await fetch('/api/watchlist/timeframes', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ timeframes: [...current, '15M'] }) })
        if (!response.ok) throw new Error(`HTTP ${response.status}`)
      }
      setEnabled(true)
      onEnabled?.()
    } catch (e) { setError(String(e)) } finally { setSaving(false) }
  }
  return <div className="item-meta" role="status">
    <span>{t(enabled ? 'forex15m.enabled' : 'forex15m.optIn')}</span>{' '}
    {enabled !== true && <button className="tf-btn" disabled={saving || enabled === null && !error} onClick={enable}>{t(saving ? 'forex15m.enabling' : 'forex15m.enable')}</button>}
    {error && <span className="error-msg"> {t('forex15m.error')}: {error}</span>}
  </div>
}
