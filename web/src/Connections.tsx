import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { connectionFetch, getServerOrigin, selectServer } from './apiAuth'

export function normalizeServer(value: string): string {
  const url = new URL(value.trim())
  const loopback = ['localhost', '127.0.0.1', '[::1]'].includes(url.hostname)
  if ((url.protocol !== 'https:' && !(url.protocol === 'http:' && loopback)) || url.username || url.password || url.search || url.hash || (url.pathname !== '/' && url.pathname !== '')) throw new Error('Use an HTTPS server origin without a path, query or credentials. HTTP is allowed only for localhost.')
  return url.origin
}

type Profile = { name: string; origin: string }
const profileKey = 'chartnagari.connections.v1'
function readProfiles(): Profile[] {
  try {
    const value: unknown = JSON.parse(localStorage.getItem(profileKey) || '[]')
    if (!Array.isArray(value)) return []
    return value.slice(0, 20).flatMap(p => {
      try { return typeof p.name === 'string' && typeof p.origin === 'string' ? [{ name: p.name.slice(0, 80), origin: normalizeServer(p.origin) }] : [] } catch { return [] }
    })
  } catch { return [] }
}
const copy = {
  ko: { title: '서버 연결', current: '현재 서버', local: '현재 웹 서버 사용', name: '연결 이름', url: '서버 주소', token: '서버 관리자 API 토큰', connect: '확인 후 연결', remove: '삭제', confirm: '서버를 전환하면 미저장 입력과 현재 화면이 초기화됩니다. 이미 서버가 받은 작업은 취소되지 않습니다. 계속할까요?', help: '토큰은 메모리에만 유지되며 새로고침하면 잊습니다. 연결 이름과 주소만 이 브라우저에 저장합니다. 원격 서버에서 원격 접근을 활성화하고 이 앱의 주소를 허용해야 합니다. 로컬 LLM은 연결된 서버 기준입니다.', checking: '인증·지원 기능 확인 중…', failed: '연결 실패: 주소, HTTPS 인증서, 토큰, 원격 접근 설정 및 허용된 앱 주소를 확인하세요.', active: '연결 확인됨', noRemote: '원격 연결을 지원하도록 설정된 서버가 아닙니다.', forget: '연결 해제', again: '새로고침 후에는 연결을 다시 선택·인증하세요.' },
  en: { title: 'Server connections', current: 'Current server', local: 'Use this web server', name: 'Connection name', url: 'Server origin', token: 'Server administrator API token', connect: 'Verify and connect', remove: 'Remove', confirm: 'Switching servers discards unsaved inputs and resets the workspace. Operations already received by a server are not cancelled. Continue?', help: 'Tokens stay in memory and are forgotten on reload. Only names and addresses are saved in this browser. Enable remote access on the target server and allow this app’s origin. Local LLM means local to the connected server.', checking: 'Checking authentication and capabilities…', failed: 'Connection failed: check address, HTTPS certificate, token, remote access and allowed app origin.', active: 'Connection verified', noRemote: 'This server is not configured for remote connections.', forget: 'Disconnect', again: 'After reloading, select and authenticate the connection again.' },
  ja: { title: 'サーバー接続', current: '現在のサーバー', local: 'このWebサーバーを使用', name: '接続名', url: 'サーバーアドレス', token: 'サーバー管理者APIトークン', connect: '確認して接続', remove: '削除', confirm: '切り替えると未保存の入力と画面がリセットされます。受信済みの処理は取り消されません。続行しますか？', help: 'トークンはメモリ内のみで保持され、再読み込みで消去されます。名前とアドレスのみ保存します。接続先でリモートアクセスを有効にし、このアプリのオリジンを許可してください。ローカルLLMは接続先サーバー基準です。', checking: '認証と機能を確認中…', failed: '接続失敗。アドレス、HTTPS証明書、トークン、リモートアクセス、許可オリジンを確認してください。', active: '接続確認済み', noRemote: 'リモート接続が設定されていません。', forget: '切断', again: '再読み込み後は接続先を選択し、再認証してください。' },
}

export function ConnectionPanel({ onConnect, connected = true, onDisconnect }: { onConnect: () => void; connected?: boolean; onDisconnect?: () => void }) {
  const { i18n } = useTranslation()
  const t = copy[i18n.language.startsWith('ko') ? 'ko' : i18n.language.startsWith('ja') ? 'ja' : 'en']
  const [profiles, setProfiles] = useState(readProfiles)
  const [name, setName] = useState('')
  const [origin, setOrigin] = useState(() => sessionStorage.getItem('chartnagari.selected-server') || window.location.origin)
  const [token, setToken] = useState('')
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState<'' | 'checking' | 'active' | 'failed' | 'noRemote'>('')
  const [capabilities, setCapabilities] = useState<string[]>([])
  const persist = (next: Profile[]) => { localStorage.setItem(profileKey, JSON.stringify(next)); setProfiles(next) }
  const connect = async () => {
    setBusy(true); setMessage('checking')
    try {
      const target = normalizeServer(origin)
      // The endpoint being probed is explicit: never send the active server's token.
      const response = await connectionFetch(new URL('/api/connection', target), { headers: { Authorization: `Bearer ${token}` }, redirect: 'error', credentials: 'omit', signal: AbortSignal.timeout(10000) })
      if (!response.ok) throw new Error(t.failed)
      const info = await response.json()
      if (info.application !== 'ChartNagari' || info.protocol !== 1 || (target !== window.location.origin && info.remote_access !== true)) throw new Error('noRemote')
      if (!window.confirm(t.confirm)) { setMessage(''); return }
      if (target !== window.location.origin) persist([...profiles.filter(p => p.origin !== target), { name: name.trim().slice(0, 80) || profiles.find(p => p.origin === target)?.name || target, origin: target }].slice(-20))
      selectServer(target, token)
      setCapabilities(Object.entries(info.capabilities || {}).filter(([, enabled]) => enabled === true).map(([key]) => key))
      window.history.replaceState(null, '', window.location.pathname)
      setToken(''); setMessage('active')
      onConnect()
    } catch (error) { setMessage(error instanceof Error && error.message === 'noRemote' ? 'noRemote' : 'failed') }
    finally { setBusy(false) }
  }
  return <details className="connection-panel" open={connected ? undefined : true}><summary>{t.title} · {connected ? getServerOrigin() : t.again}</summary>
    {connected && <p>{t.current}: {getServerOrigin()}</p>}<p>{t.help}</p>
    {connected && capabilities.length > 0 && <p>API v1 · {capabilities.join(' · ')}</p>}
    {connected && onDisconnect && <button disabled={busy} onClick={() => { if (window.confirm(t.confirm)) { selectServer(getServerOrigin(), ''); setToken(''); setMessage(''); onDisconnect() } }}>{t.forget}</button>}
    <button disabled={busy} onClick={() => { setOrigin(window.location.origin); setToken(''); setName('') }}>{t.local}</button>
    {profiles.map(p => <div key={p.origin}><button disabled={busy} onClick={() => { setName(p.name); setOrigin(p.origin); setToken(''); setMessage('') }}>{p.name} · {p.origin}</button><button disabled={busy} aria-label={`${t.remove} ${p.name}`} onClick={() => persist(profiles.filter(x => x.origin !== p.origin))}>{t.remove}</button></div>)}
    <label>{t.name}<input disabled={busy} value={name} onChange={e => setName(e.target.value)} maxLength={80} /></label>
    <label>{t.url}<input disabled={busy} value={origin} onChange={e => { setOrigin(e.target.value); setToken('') }} type="url" autoCapitalize="none" spellCheck={false} /></label>
    <label>{t.token}<input disabled={busy} value={token} onChange={e => setToken(e.target.value)} type="password" autoComplete="off" /></label>
    <button disabled={busy} onClick={() => void connect()}>{busy ? t.checking : t.connect}</button>
    <p role="status">{message ? t[message] : ''}</p><small>{t.again}</small>
  </details>
}
