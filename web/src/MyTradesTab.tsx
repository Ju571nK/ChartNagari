import { useEffect, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { MarkActions, MarkStatus } from './MarkActions'

interface RollupRow {
  key: string
  took: number
  skipped: number
  wins: number
  losses: number
  bes: number
  hit_rate: number
  skip_rate: number
}

interface RollupResponse {
  by: string
  since: string
  rows: RollupRow[]
}

interface PendingRow {
  signal: { id: number; symbol: string; timeframe: string; rule: string; direction: string; score: number; created_at: string }
  mark: null
}

interface MarkedRow {
  signal: PendingRow['signal']
  mark: { status: MarkStatus; took_at: number | null; outcome_at: number | null; updated_at: number }
}

type Subtab = 'rollup' | 'pending' | 'history'
type GroupBy = 'rule' | 'symbol' | 'methodology' | 'timeframe'

const PERIODS: Record<string, number> = { '7d': 7, '30d': 30, '90d': 90, '365d': 365, all: 36500 }

export function MyTradesTab() {
  const { t } = useTranslation()
  const [subtab, setSubtab] = useState<Subtab>('rollup')
  const [groupBy, setGroupBy] = useState<GroupBy>('rule')
  const [period, setPeriod] = useState<keyof typeof PERIODS>('30d')
  const [rollup, setRollup] = useState<RollupResponse | null>(null)
  const [pending, setPending] = useState<PendingRow[]>([])
  const [history, setHistory] = useState<MarkedRow[]>([])
  const [version, setVersion] = useState(0)  // bump to force refetch after marking

  const sinceISO = useCallback(() => {
    const days = PERIODS[period]
    const d = new Date(Date.now() - days * 24 * 3600 * 1000)
    return d.toISOString()
  }, [period])

  // Fetch rollup
  useEffect(() => {
    if (subtab !== 'rollup') return
    let cancelled = false
    fetch(`/api/marks/rollup?by=${groupBy}&since=${encodeURIComponent(sinceISO())}`)
      .then(r => r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`)))
      .then((data: RollupResponse) => { if (!cancelled) setRollup(data) })
      .catch(() => { /* leave previous */ })
    return () => { cancelled = true }
  }, [subtab, groupBy, period, sinceISO, version])

  // Fetch pending
  useEffect(() => {
    if (subtab !== 'pending') return
    fetch(`/api/marks/pending?since=${encodeURIComponent(sinceISO())}&limit=200`)
      .then(r => r.ok ? r.json() : [])
      .then((rows: PendingRow[]) => setPending(rows ?? []))
      .catch(() => setPending([]))
  }, [subtab, sinceISO, version])

  // Fetch history
  useEffect(() => {
    if (subtab !== 'history') return
    fetch(`/api/marks/recent?since=${encodeURIComponent(sinceISO())}&limit=200`)
      .then(r => r.ok ? r.json() : [])
      .then((rows: MarkedRow[]) => setHistory(rows ?? []))
      .catch(() => setHistory([]))
  }, [subtab, sinceISO, version])

  const refresh = () => setVersion(v => v + 1)

  return (
    <div style={{ padding: 12 }}>
      <h2>{t('my_trades.title')}</h2>

      <div style={{ display: 'flex', gap: 8, marginBottom: 12 }}>
        {(['rollup', 'pending', 'history'] as Subtab[]).map(s => (
          <button
            key={s}
            onClick={() => setSubtab(s)}
            style={{
              padding: '6px 12px',
              border: '1px solid var(--mint)',
              background: subtab === s ? 'var(--mint)' : 'transparent',
              color: subtab === s ? 'var(--bg)' : 'var(--text)',
              borderRadius: 4,
              cursor: 'pointer',
            }}
          >
            {t(`my_trades.subtab.${s}`)}
          </button>
        ))}

        <select
          aria-label="period"
          value={period}
          onChange={e => setPeriod(e.target.value as keyof typeof PERIODS)}
          style={{ marginLeft: 'auto' }}
        >
          {(['7d', '30d', '90d', '365d', 'all'] as const).map(p => (
            <option key={p} value={p}>{t(`my_trades.period.${p}`)}</option>
          ))}
        </select>
      </div>

      {subtab === 'rollup' && (
        <RollupView rollup={rollup} groupBy={groupBy} setGroupBy={setGroupBy} t={t} />
      )}
      {subtab === 'pending' && (
        <PendingView rows={pending} onMarked={refresh} t={t} />
      )}
      {subtab === 'history' && (
        <HistoryView rows={history} onMarked={refresh} t={t} />
      )}
    </div>
  )
}

function RollupView({ rollup, groupBy, setGroupBy, t }: {
  rollup: RollupResponse | null
  groupBy: GroupBy
  setGroupBy: (g: GroupBy) => void
  t: (k: string) => string
}) {
  return (
    <>
      <label style={{ display: 'inline-block', marginBottom: 8 }}>
        groupBy:&nbsp;
        <select
          aria-label="groupBy"
          value={groupBy}
          onChange={e => setGroupBy(e.target.value as GroupBy)}
        >
          {(['rule', 'symbol', 'methodology', 'timeframe'] as GroupBy[]).map(g => (
            <option key={g} value={g}>{t(`my_trades.groupby.${g}`)}</option>
          ))}
        </select>
      </label>
      {rollup && rollup.rows.length === 0 && (
        <p style={{ color: 'var(--muted)' }}>{t('my_trades.empty.history')}</p>
      )}
      {rollup && rollup.rows.length > 0 && (
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <thead>
            <tr>
              <th align="left">{t(`my_trades.groupby.${groupBy}`)}</th>
              <th>{t('my_trades.summary.took')}</th>
              <th>Win</th>
              <th>Loss</th>
              <th>BE</th>
              <th>{t('my_trades.column.hit_rate')}</th>
              <th>{t('my_trades.column.skip_rate')}</th>
            </tr>
          </thead>
          <tbody>
            {rollup.rows.map(r => (
              <tr key={r.key} style={{ borderTop: '1px solid var(--border)' }}>
                <td>{r.key}</td>
                <td align="right">{r.took}</td>
                <td align="right">{r.wins}</td>
                <td align="right">{r.losses}</td>
                <td align="right">{r.bes}</td>
                <td align="right">{(r.hit_rate * 100).toFixed(1)}%</td>
                <td align="right">{(r.skip_rate * 100).toFixed(1)}%</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  )
}

function PendingView({ rows, onMarked, t }: { rows: PendingRow[]; onMarked: () => void; t: (k: string) => string }) {
  if (rows.length === 0) {
    return <p style={{ color: 'var(--muted)' }}>{t('my_trades.empty.pending')}</p>
  }
  return (
    <ul style={{ listStyle: 'none', padding: 0 }}>
      {rows.map(({ signal }) => (
        <li key={signal.id} style={{ padding: 8, borderBottom: '1px solid var(--border)' }}>
          <strong>{signal.symbol}</strong> · {signal.timeframe} · {signal.rule} · score {signal.score.toFixed(1)}
          {' '}<MarkActions signalId={signal.id} status="PENDING" onMarked={onMarked} />
        </li>
      ))}
    </ul>
  )
}

function HistoryView({ rows, onMarked, t }: { rows: MarkedRow[]; onMarked: () => void; t: (k: string) => string }) {
  if (rows.length === 0) {
    return <p style={{ color: 'var(--muted)' }}>{t('my_trades.empty.history')}</p>
  }
  return (
    <ul style={{ listStyle: 'none', padding: 0 }}>
      {rows.map(({ signal, mark }) => (
        <li key={signal.id} style={{ padding: 8, borderBottom: '1px solid var(--border)' }}>
          <strong>{signal.symbol}</strong> · {signal.timeframe} · {signal.rule} · <em>{mark.status}</em>
          {' '}<MarkActions signalId={signal.id} status={mark.status} onMarked={onMarked} />
        </li>
      ))}
    </ul>
  )
}
