import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

export interface AlertDropRow {
  symbol: string
  rule: string
  timeframe: string
  direction: string
  reason: string
  stage: string
  knob: string
  score: number
  threshold: number
  signal_id: number
  sender: string
  detail: string
  count: number
  first_seen: string
  last_seen: string
}

interface AlertDropsResponse {
  enabled: boolean
  drops: AlertDropRow[]
}

function shortTime(iso: string): string {
  const parsed = new Date(iso)
  if (Number.isNaN(parsed.getTime())) return iso
  return parsed.toISOString().slice(0, 16).replace('T', ' ')
}

export function AlertDropsPanel({ symbol, refreshToken }: { symbol: string; refreshToken: number }) {
  const { t } = useTranslation()
  const [phase, setPhase] = useState<'loading' | 'error' | 'ready'>('loading')
  const [enabled, setEnabled] = useState(false)
  const [drops, setDrops] = useState<AlertDropRow[]>([])

  useEffect(() => {
    const ctrl = new AbortController()
    const params = new URLSearchParams({ limit: '50' })
    if (symbol) params.set('symbol', symbol)
    setPhase('loading')
    fetch(`/api/alert-drops?${params.toString()}`, { signal: ctrl.signal })
      .then(async (res) => {
        if (!res.ok) throw new Error(String(res.status))
        return res.json() as Promise<AlertDropsResponse>
      })
      .then((body) => {
        if (ctrl.signal.aborted) return
        setEnabled(Boolean(body.enabled))
        setDrops(Array.isArray(body.drops) ? body.drops : [])
        setPhase('ready')
      })
      .catch(() => {
        if (ctrl.signal.aborted) return
        setPhase('error')
      })
    return () => ctrl.abort()
  }, [symbol, refreshToken])

  return (
    <section aria-labelledby="alert-drops-title">
      <p id="alert-drops-title" className="section-title">{t('alertDrops.title')}</p>
      {phase === 'loading' && <p className="alert-drop-note" role="status">{t('alertDrops.loading')}</p>}
      {phase === 'error' && <p className="error-msg" role="alert">{t('alertDrops.error')}</p>}
      {phase === 'ready' && !enabled && <p className="alert-drop-note" role="status">{t('alertDrops.disabled')}</p>}
      {phase === 'ready' && enabled && drops.length === 0 && (
        <>
          <p className="alert-drop-note" role="status">{t('alertDrops.empty')}</p>
          <p className="alert-drop-note" role="status">{t('alertDrops.emptyHelp')}</p>
        </>
      )}
      {phase === 'ready' && enabled && drops.length > 0 && (
        <div className="alert-drop-scroll">
          <table className="alert-drops">
            <thead>
              <tr>
                <th>{t('alertDrops.col.reason')}</th>
                <th>{t('alertDrops.col.rule')}</th>
                <th>{t('alertDrops.col.timeframe')}</th>
                <th>{t('alertDrops.col.direction')}</th>
                <th>{t('alertDrops.col.score')}</th>
                <th>{t('alertDrops.col.count')}</th>
                <th>{t('alertDrops.col.lastSeen')}</th>
                <th>{t('alertDrops.col.detail')}</th>
              </tr>
            </thead>
            <tbody>
              {drops.map((row, index) => (
                <tr key={`${row.rule}|${row.timeframe}|${row.direction}|${row.reason}|${row.sender}|${index}`}>
                  <td title={row.knob}>{t(`alertDrops.reason.${row.reason}`)}</td>
                  <td>{row.rule}</td>
                  <td>{row.timeframe}</td>
                  <td className={row.direction === 'LONG' ? 'dir-long' : row.direction === 'SHORT' ? 'dir-short' : undefined}>
                    {row.direction}
                  </td>
                  <td>{row.score.toFixed(1)}</td>
                  <td>{row.count}</td>
                  <td>{shortTime(row.last_seen)}</td>
                  <td>{row.sender ? `${row.sender}: ${row.detail}` : row.detail}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}
