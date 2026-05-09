import { useState } from 'react'
import { useTranslation } from 'react-i18next'

export type MarkStatus = 'PENDING' | 'TOOK' | 'SKIPPED' | 'WIN' | 'LOSS' | 'BE'

interface Props {
  signalId: number
  status: MarkStatus
  onMarked?: (newStatus: string) => void
  apiToken?: string
}

const ACTIONS_BY_STATUS: Record<MarkStatus, { action: string; labelKey: string }[]> = {
  PENDING:  [{ action: 'took', labelKey: 'my_trades.action.took' }, { action: 'skip', labelKey: 'my_trades.action.skipped' }],
  TOOK:     [
    { action: 'win',  labelKey: 'my_trades.action.win' },
    { action: 'loss', labelKey: 'my_trades.action.loss' },
    { action: 'be',   labelKey: 'my_trades.action.be' },
    { action: 'undo', labelKey: 'my_trades.action.undo' },
  ],
  SKIPPED:  [{ action: 'undo', labelKey: 'my_trades.action.undo' }],
  WIN:      [{ action: 'undo', labelKey: 'my_trades.action.edit' }],
  LOSS:     [{ action: 'undo', labelKey: 'my_trades.action.edit' }],
  BE:       [{ action: 'undo', labelKey: 'my_trades.action.edit' }],
}

export function MarkActions({ signalId, status, onMarked, apiToken }: Props) {
  const { t } = useTranslation()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const submit = async (action: string) => {
    setBusy(true); setError(null)
    try {
      const headers: Record<string, string> = { 'Content-Type': 'application/json' }
      if (apiToken) headers['Authorization'] = `Bearer ${apiToken}`
      const res = await fetch(`/api/marks/${signalId}`, {
        method: 'POST',
        headers,
        body: JSON.stringify({ action }),
      })
      if (!res.ok) {
        const data = await res.json().catch(() => ({} as { error?: string }))
        throw new Error(data.error ?? `HTTP ${res.status}`)
      }
      const data = await res.json() as { status: string }
      onMarked?.(data.status)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'unknown')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div style={{ display: 'inline-flex', gap: 4 }}>
      {ACTIONS_BY_STATUS[status].map(({ action, labelKey }) => (
        <button
          key={action}
          disabled={busy}
          onClick={() => submit(action)}
          style={{
            padding: '4px 10px',
            border: '1px solid var(--mint)',
            background: 'transparent',
            color: 'var(--text)',
            borderRadius: 4,
            cursor: busy ? 'wait' : 'pointer',
            fontSize: '0.78rem',
          }}
        >{t(labelKey)}</button>
      ))}
      {error && (
        <span style={{ color: 'var(--danger)', fontSize: '0.72rem', marginLeft: 6 }}>
          {t('my_trades.save_failed')}: {error}
        </span>
      )}
    </div>
  )
}
