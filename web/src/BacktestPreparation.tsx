import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useUXCopy } from './uxCopy'

export interface Readiness { symbol: string; timeframe: string; bars: number; minimum_bars: number; from: number | null; to: number | null; ready: boolean }
export function useBacktestPreparation(symbol: string, timeframe: string, revision: number) {
  const key = JSON.stringify([symbol, timeframe, revision])
  const [state, setState] = useState<{ key: string; data?: Readiness; error?: string }>({ key: '' })
  useEffect(() => {
    const controller = new AbortController()
    if (!symbol) return () => controller.abort()
    setState({ key })
    fetch(`/api/backtest/readiness?symbol=${encodeURIComponent(symbol)}&timeframe=${encodeURIComponent(timeframe)}`, { signal: controller.signal })
      .then(async response => {
        if (!response.ok) throw new Error(`HTTP ${response.status}`)
        const data: Readiness = await response.json()
        if (!data || data.symbol !== symbol || data.timeframe !== timeframe || !Number.isInteger(data.bars) || !Number.isInteger(data.minimum_bars) || typeof data.ready !== 'boolean') throw new Error('Invalid backtest preparation response')
        if (!controller.signal.aborted) setState({ key, data })
      })
      .catch((error: Error) => { if (!controller.signal.aborted) setState({ key, error: error.message }) })
    return () => controller.abort()
  }, [key, symbol, timeframe])
  return state.key === key ? state : { key }
}

export function BacktestPreparation({ data, error, symbol, onRetry, expert = false }: { data?: Readiness; error?: string; symbol: string; onRetry: () => void; expert?: boolean }) {
  const { t, i18n } = useTranslation()
  const ux = useUXCopy()
  const format = (time: number) => new Date(time * 1000).toLocaleString(i18n.language, { timeZoneName: 'short' })
  const state = !symbol ? 'select' : error ? 'error' : !data ? 'loading' : data.bars === 0 ? 'empty' : !data.ready ? 'insufficient' : 'ready'
  return <section className="market-data-status" aria-label={t('backtestPrep.title')}>
    <p role={error ? 'alert' : 'status'}>{t(`backtestPrep.${state}`)}</p>
    <details open={!!error || (!!data && !data.ready)}><summary>{ux.details}</summary>
    {data && <>
      <p>{t('backtestPrep.count', { count: data.bars, minimum: data.minimum_bars })}</p>
      {data.from !== null && data.to !== null && <p>{t('backtestPrep.range')}: {format(data.from)} — {format(data.to)}</p>}
    </>}
    <p>{t('backtestPrep.multipliers')}</p>
    <p>{t('backtestPrep.caution')}</p>
    </details>
    {expert && data && <HTFReadinessPanel key={`${data.symbol}:${data.timeframe}`} symbol={data.symbol} timeframe={data.timeframe} />}
    {symbol && (data || error) && <button onClick={onRetry}>{data?.ready ? ux.refresh : t('workspace.retry')}</button>}
  </section>
}

interface HTFInventory {
  symbol: string; timeframe: string; ready: boolean
  buckets: { decile: number; signals: number; distinct_days: number; from: number | null; to: number | null; history_sufficient: boolean }[]
}

const htfText = {
  en: { title: 'HTF calibration · history inventory', load: 'Check history', loading: 'Checking…', error: 'History check failed. Try again.', lower: 'HTF penalties apply to 1H and 4H signals. Select either timeframe.', note: 'Automatic calibration is not enabled. Existing penalty settings remain unchanged. These are post-filter signal counts, not trades or unbiased backtest results. Pre-filter history, validated outcomes and out-of-sample tests are still required.', gate: 'Initial history gate: at least 2 years and 30 distinct signal dates per bucket. Passing this gate does not establish statistical reliability.', bucket: 'ATR percentile', count: 'Signals / dates', range: 'UTC date range', history: 'History gate', pass: 'Met', fail: 'Insufficient' },
  ko: { title: 'HTF 자동 보정 · 이력 점검', load: '이력 확인', loading: '확인 중…', error: '이력 조회에 실패했습니다. 다시 시도하세요.', lower: 'HTF 감점은 1H·4H 신호에 적용됩니다. 해당 시간봉을 선택하세요.', note: '자동 보정은 비활성 상태이며 기존 감점 설정을 유지합니다. 아래 수치는 필터 통과 후 저장된 신호 건수로, 거래 수나 편향 없는 백테스트 성과가 아닙니다. 필터 이전 이력, 검증된 결과, 별도 기간 검증이 추가로 필요합니다.', gate: '기초 이력 기준: 구간마다 2년 이상, 서로 다른 신호 발생일 30일 이상. 기준 충족이 통계적 신뢰성을 보장하지는 않습니다.', bucket: 'ATR 백분위', count: '신호 / 발생일', range: 'UTC 날짜 범위', history: '이력 기준', pass: '충족', fail: '부족' },
  ja: { title: 'HTF自動補正 · 履歴確認', load: '履歴を確認', loading: '確認中…', error: '履歴の取得に失敗しました。再試行してください。', lower: 'HTF減点は1H・4Hシグナルに適用されます。該当する時間足を選択してください。', note: '自動補正は無効で、既存の減点設定を維持します。以下はフィルター通過後の保存シグナル数であり、取引数や偏りのないバックテスト結果ではありません。フィルター前の履歴、検証済みの結果、別期間での検証が必要です。', gate: '初期基準：各区間で2年以上、異なるシグナル発生日が30日以上。基準の充足は統計的信頼性を保証しません。', bucket: 'ATR百分位', count: 'シグナル / 日数', range: 'UTC日付範囲', history: '履歴基準', pass: '充足', fail: '不足' },
}

export function HTFReadinessPanel({ symbol, timeframe }: { symbol: string; timeframe: string }) {
  const { i18n } = useTranslation()
  const text = htfText[i18n.language.startsWith('ko') ? 'ko' : i18n.language.startsWith('ja') ? 'ja' : 'en']
  const [revision, setRevision] = useState(0)
  const [data, setData] = useState<HTFInventory>()
  const [error, setError] = useState(false)
  const [loading, setLoading] = useState(false)
  const supported = timeframe === '1H' || timeframe === '4H'
  useEffect(() => {
    if (!revision || !supported) return
    const controller = new AbortController()
    setLoading(true); setError(false); setData(undefined)
    fetch(`/api/backtest/htf-readiness?symbol=${encodeURIComponent(symbol)}&timeframe=${encodeURIComponent(timeframe)}`, { signal: controller.signal })
      .then(async response => {
        if (!response.ok) throw new Error('Request failed')
        const result: HTFInventory = await response.json()
        if (!result || result.symbol !== symbol || result.timeframe !== timeframe || result.ready !== false || !Array.isArray(result.buckets) || result.buckets.length !== 10 || result.buckets.some((b, index) => !b || b.decile !== index || !Number.isInteger(b.signals) || b.signals < 0 || !Number.isInteger(b.distinct_days) || b.distinct_days < 0 || typeof b.history_sufficient !== 'boolean' || [b.from, b.to].some(value => value !== null && (!Number.isFinite(value) || Math.abs(value) > 8640000000000)))) throw new Error('Invalid inventory')
        if (!controller.signal.aborted) setData(result)
      })
      .catch(() => { if (!controller.signal.aborted) setError(true) })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [revision, supported, symbol, timeframe])
  const date = (value: number | null) => value === null ? '—' : new Date(value * 1000).toISOString().slice(0, 10)
  return <details>
    <summary>{text.title}</summary>
    <p>{text.note}</p><p>{text.gate}</p>
    {!supported ? <p>{text.lower}</p> : <button disabled={loading} onClick={() => setRevision(n => n + 1)}>{loading ? text.loading : text.load}</button>}
    {error && <p role="alert">{text.error}</p>}
    {data && <div style={{ overflowX: 'auto' }}><table>
      <caption>{symbol} · {timeframe}</caption>
      <thead><tr><th scope="col">{text.bucket}</th><th scope="col">{text.count}</th><th scope="col">{text.range}</th><th scope="col">{text.history}</th></tr></thead>
      <tbody>{data.buckets.map(b => <tr key={b.decile}><th scope="row">[{b.decile * 10}%, {(b.decile + 1) * 10}%{b.decile === 9 ? ']' : ')'}</th><td>{b.signals} / {b.distinct_days}</td><td>{date(b.from)} — {date(b.to)}</td><td>{b.history_sufficient ? text.pass : text.fail}</td></tr>)}</tbody>
    </table></div>}
  </details>
}
