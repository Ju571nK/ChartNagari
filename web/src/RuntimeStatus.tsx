import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

const words = {
  en: {
    saved: 'Saved. See the startup comparison below for restart requirements.', details: 'Collection details', fields: 'Per-field startup comparison',
    settings: 'Saved settings / startup state', refresh: 'Check status', loading: 'Checking status…', unavailable: 'Status unavailable. Check that the updated server is running and try again.',
    note: 'Compared with the YAML read at this server’s startup, not proof of an enabled or connected integration. External adapters and MCP must be checked separately.',
    startup_match: 'Matches startup settings', pending_restart: 'Saved · restart required', external: 'Saved · external process not verified', unknown: 'Startup state unknown', unsaved: 'Unsaved edit',
    calendar: 'Calendar collection status', provider: 'Active provider', attempt: 'Last attempt', successTime: 'Last success', count: 'US events in last successful batch', none: 'None',
    calendarNote: 'Status history is kept for this server run only. Collection runs at startup and every 6 hours (failures retry after 1 and 5 minutes). Checking status does not trigger collection. Existing cached events may remain visible after a failure.',
    disabled: 'Disabled · no provider key loaded at startup', waiting: 'Waiting for first collection', fetching: 'Collecting…', success: 'Last collection succeeded', error: 'Last collection failed',
    authentication: 'Authentication rejected. Check the provider key.', permission: 'Access denied. Check account permissions or subscription.', rate_limit: 'Provider rate limit reached. Wait before retrying.', network: 'Network or timeout error. Check server connectivity.', providerError: 'Provider request failed. Check provider availability and endpoint access.', invalid_response: 'Unexpected provider response. Check provider compatibility.', storage: 'Could not save events. Check database health.', empty: 'Successful collection returned no usable US events; this does not mean the key is missing.',
  },
  ko: {
    saved: '저장했습니다. 아래 시작 시점 비교에서 재시작 필요 항목을 확인하세요.', details: '수집 상세 정보', fields: '항목별 시작 시점 비교',
    settings: '저장 설정 / 시작 시점 상태', refresh: '상태 확인', loading: '상태 확인 중…', unavailable: '상태를 확인할 수 없습니다. 업데이트된 서버가 실행 중인지 확인한 후 다시 시도하세요.',
    note: '서버 시작 시 읽은 YAML과 비교합니다. 기능 활성화나 연결 성공을 보장하지 않습니다. 외부 어댑터와 MCP는 별도로 확인해야 합니다.',
    startup_match: '시작 시 읽은 설정과 일치', pending_restart: '저장됨 · 재시작 필요', external: '저장됨 · 외부 프로세스 확인 필요', unknown: '시작 시점 상태 확인 불가', unsaved: '저장하지 않은 변경',
    calendar: '캘린더 수집 상태', provider: '실행 중인 제공자', attempt: '마지막 시도', successTime: '마지막 성공', count: '마지막 성공 수집의 미국 일정 수', none: '없음',
    calendarNote: '상태 기록은 이번 서버 실행 동안만 유지됩니다. 시작 시와 6시간마다 수집하며 실패 시 1분·5분 후 재시도합니다. 상태 확인은 수집을 실행하지 않습니다. 실패 후에도 기존 저장 일정은 표시될 수 있습니다.',
    disabled: '비활성 · 시작 시 불러온 제공자 키 없음', waiting: '첫 수집 대기 중', fetching: '수집 중…', success: '마지막 수집 성공', error: '마지막 수집 실패',
    authentication: '인증이 거부되었습니다. 제공자 키를 확인하세요.', permission: '접근이 거부되었습니다. 계정 권한이나 구독을 확인하세요.', rate_limit: '제공자 요청 한도를 초과했습니다. 재시도를 기다리세요.', network: '네트워크 또는 시간 초과 오류입니다. 서버 연결 상태를 확인하세요.', providerError: '제공자 요청이 실패했습니다. 제공자 서비스와 API 접근을 확인하세요.', invalid_response: '예상하지 못한 응답입니다. 제공자 API 호환성을 확인하세요.', storage: '일정 저장에 실패했습니다. 데이터베이스 상태를 확인하세요.', empty: '수집은 성공했지만 사용 가능한 미국 일정이 없습니다. 키가 없다는 의미는 아닙니다.',
  },
  ja: {
    saved: '保存しました。以下の起動時比較で再起動が必要な項目を確認してください。', details: '収集の詳細', fields: '項目別の起動時比較',
    settings: '保存設定 / 起動時の状態', refresh: '状態を確認', loading: '状態を確認中…', unavailable: '状態を確認できません。更新済みサーバーの起動を確認して再試行してください。',
    note: 'サーバー起動時に読み込んだYAMLとの比較です。機能の有効化や接続成功を保証しません。外部アダプターとMCPは別途確認が必要です。',
    startup_match: '起動時の設定と一致', pending_restart: '保存済み · 再起動が必要', external: '保存済み · 外部プロセス未確認', unknown: '起動時の状態は不明', unsaved: '未保存の変更',
    calendar: 'カレンダー収集状態', provider: '稼働中のプロバイダー', attempt: '最終試行', successTime: '最終成功', count: '最後に収集した米国イベント数', none: 'なし',
    calendarNote: '履歴は今回のサーバー起動中のみ保持されます。起動時と6時間ごとに収集し、失敗時は1分・5分後に再試行します。状態確認は収集を実行しません。失敗後も保存済みイベントは表示されます。',
    disabled: '無効 · 起動時にプロバイダーキーなし', waiting: '初回収集待ち', fetching: '収集中…', success: '最終収集成功', error: '最終収集失敗',
    authentication: '認証が拒否されました。キーを確認してください。', permission: 'アクセス拒否。アカウント権限や契約を確認してください。', rate_limit: 'リクエスト上限です。再試行をお待ちください。', network: 'ネットワークまたはタイムアウトエラーです。', providerError: 'プロバイダーへのリクエストが失敗しました。', invalid_response: '予期しない応答です。API互換性を確認してください。', storage: 'イベントの保存に失敗しました。データベースを確認してください。', empty: '収集は成功しましたが利用可能な米国イベントはありません。キーがないという意味ではありません。',
  },
}

function useWords() {
  const { i18n } = useTranslation()
  return words[i18n.language.startsWith('ko') ? 'ko' : i18n.language.startsWith('ja') ? 'ja' : 'en']
}

export function SettingsSavedNotice() { return <>{useWords().saved}</> }

function useStatus<T>(url: string, revision: number, valid: (value: unknown) => value is T) {
  const [data, setData] = useState<T | null>(null)
  const [loading, setLoading] = useState(true)
  const [failed, setFailed] = useState(false)
  const request = useRef<AbortController | null>(null)
  const refresh = useCallback(() => {
    request.current?.abort()
    const controller = new AbortController()
    request.current = controller
    setLoading(true)
    fetch(url, { signal: controller.signal, cache: 'no-store' })
      .then(async r => { if (!r.ok) throw new Error('status'); const value: unknown = await r.json(); if (!valid(value)) throw new Error('status'); return value })
      .then(value => { if (!controller.signal.aborted) { setData(value); setFailed(false) } })
      .catch(() => { if (!controller.signal.aborted) { setFailed(true); setData(null) } })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
  }, [url, valid])
  useEffect(() => {
    refresh()
    const timer = window.setInterval(refresh, 30000)
    return () => { window.clearInterval(timer); request.current?.abort() }
  }, [refresh, revision])
  return { data, loading, failed, refresh }
}

type SettingsState = { fields: Record<string, string> }
const validSettings = (v: unknown): v is SettingsState => !!v && typeof v === 'object' && 'fields' in v && !!v.fields && typeof v.fields === 'object' && !Array.isArray(v.fields) && Object.values(v.fields).every(x => typeof x === 'string')

export function SettingsRuntimeStatus({ fields, edits, revision }: { fields: { key: string; label: string }[]; edits: Record<string, string>; revision: number }) {
  const copy = useWords()
  const { data, loading, failed, refresh } = useStatus('/api/settings/status', revision, validSettings)
  const label = (key: string) => {
    if (key in edits) return copy.unsaved
    const state = data?.fields[key]
    return state === 'startup_match' || state === 'pending_restart' || state === 'external' ? copy[state] : copy.unknown
  }
  const pending = fields.filter(f => data?.fields[f.key] === 'pending_restart').length
  return <section className="runtime-status" aria-label={copy.settings}>
    <strong>{copy.settings}</strong>
    {pending > 0 && <p role="status">{copy.pending_restart} ({pending})</p>}
    {failed && <p role="status">{copy.unavailable}</p>}
    <details><summary>{copy.fields}</summary><dl>{fields.map(f => <div key={f.key}><dt>{f.label}</dt><dd>{label(f.key)}</dd></div>)}</dl><p>{copy.note}</p></details>
    <button type="button" disabled={loading} onClick={refresh}>{loading ? copy.loading : copy.refresh}</button>
  </section>
}

type CalendarState = { state: string; provider?: string; failure?: string; last_attempt?: string; last_success?: string; event_count?: number }
const validCalendar = (v: unknown): v is CalendarState => !!v && typeof v === 'object' && 'state' in v && typeof v.state === 'string' && ['disabled', 'waiting', 'fetching', 'success', 'error', 'unknown'].includes(v.state)
export function CalendarCollectionStatus() {
  const copy = useWords()
  const { data, loading, failed, refresh } = useStatus('/api/calendar/status', 0, validCalendar)
  const state = data?.state
  const stateLabel = state === 'disabled' || state === 'waiting' || state === 'fetching' || state === 'success' || state === 'error' ? copy[state] : copy.unknown
  const failure = data?.failure
  const failureLabel = failure === 'authentication' || failure === 'permission' || failure === 'rate_limit' || failure === 'network' || failure === 'invalid_response' || failure === 'storage' ? copy[failure] : copy.providerError
  const date = (value?: string) => !value || value.startsWith('0001-') || Number.isNaN(Date.parse(value)) ? copy.none : new Date(value).toLocaleString()
  return <section className="runtime-status" aria-label={copy.calendar}>
    <strong>{copy.calendar}</strong>
    <p role="status">{failed ? copy.unavailable : loading && !data ? copy.loading : stateLabel}</p>
    {data && <>
      <p>{copy.provider}: {data.provider === 'FMP' || data.provider === 'Finnhub' ? data.provider : copy.none}</p>
      {state === 'error' && <p>{failureLabel}</p>}
      {state === 'success' && data.event_count === 0 && <p>{copy.empty}</p>}
      <details><summary>{copy.details}</summary><dl>
        <div><dt>{copy.attempt}</dt><dd>{date(data.last_attempt)}</dd></div>
        <div><dt>{copy.successTime}</dt><dd>{date(data.last_success)}</dd></div>
        <div><dt>{copy.count}</dt><dd>{data.last_success && !data.last_success.startsWith('0001-') ? data.event_count : '—'}</dd></div>
      </dl><p>{copy.calendarNote}</p></details>
    </>}
    <button type="button" disabled={loading} onClick={refresh}>{loading ? copy.loading : copy.refresh}</button>
  </section>
}
