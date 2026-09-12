import { useTranslation } from 'react-i18next'

const copy = {
  en: { more: 'More', primary: 'Everyday tools', details: 'Details', refresh: 'Refresh', preferences: 'Display preferences', archive: 'Outside loaded chart range', inRange: 'Within loaded chart range', unavailable: 'Chart range unavailable', score: 'Rule score, not a probability or expected return.', signals: 'Signal history', original: 'Original signal description', advanced: 'Advanced options', auth: 'Authorize protected changes', saved: 'Saved · restart required. Application has not been verified.', pending: 'Unsaved changes', unchanged: 'No unsaved changes', general: 'Display changes apply immediately in this browser. Server and database settings are under Advanced.', alerts: 'Alerts', signalAlerts: 'Signal alerts', priceAlerts: 'Price alerts', channels: 'Delivery channels', alertHelp: 'Signal settings are global defaults. Use Symbols for per-instrument overrides; those take precedence. Price alerts are independent target-price conditions. Channel changes require a server restart.', admin: 'Server administration', rangeHelp: 'Dates refer to stored signals, not new recommendations. Range badges compare against loaded candles, not the zoomed viewport.', setup: 'Display & language', connected: 'Server connected', disconnected: 'Reconnecting', strategy: 'Strategy', risk: 'Risk distances (ATR)', readGuide: 'Chart guide', language: 'Language' },
  ko: { more: '더 보기', primary: '주요 기능', details: '상세 정보', refresh: '새로고침', preferences: '화면 설정', archive: '불러온 차트 범위 밖', inRange: '불러온 차트 범위 안', unavailable: '차트 범위 확인 불가', score: '분석 규칙 점수이며 확률이나 예상 수익률이 아닙니다.', signals: '신호 기록', original: '신호 원문', advanced: '고급 옵션', auth: '보호된 설정 변경 인증', saved: '저장됨 · 재시작 필요. 적용 여부는 아직 확인되지 않았습니다.', pending: '저장하지 않은 변경 사항', unchanged: '저장하지 않은 변경 없음', general: '화면 설정은 이 브라우저에 즉시 적용됩니다. 서버·데이터베이스 설정은 고급 설정에 있습니다.', alerts: '알림', signalAlerts: '분석 신호 알림', priceAlerts: '가격 알림', channels: '전달 채널', alertHelp: '신호 설정은 전역 기본값입니다. 종목별 예외는 종목 설정에서 관리하며 기본값보다 우선합니다. 가격 알림은 별도의 목표 가격 조건입니다. 전달 채널 변경은 서버 재시작이 필요합니다.', admin: '서버 관리', rangeHelp: '저장된 과거 신호이며 새 추천이 아닙니다. 범위 표시는 확대·축소 영역이 아닌 불러온 캔들 기준입니다.', setup: '화면·언어', connected: '서버 연결됨', disconnected: '재연결 중', strategy: '전략', risk: '손익 거리 (ATR)', readGuide: '차트 읽는 법', language: '언어' },
  ja: { more: 'その他', primary: 'よく使う機能', details: '詳細', refresh: '更新', preferences: '表示設定', archive: '読み込み済みチャートの範囲外', inRange: '読み込み済みチャートの範囲内', unavailable: 'チャート範囲を確認できません', score: '分析ルールのスコアです。確率や予想収益率ではありません。', signals: 'シグナル履歴', original: 'シグナル原文', advanced: '詳細オプション', auth: '保護された変更の認証', saved: '保存済み · 再起動が必要です。適用は未確認です。', pending: '未保存の変更', unchanged: '未保存の変更なし', general: '表示設定はこのブラウザーに即時適用されます。サーバーとデータベースは詳細設定にあります。', alerts: '通知', signalAlerts: 'シグナル通知', priceAlerts: '価格通知', channels: '配信先', alertHelp: 'シグナル設定は全体の既定値です。銘柄別の例外は銘柄設定で管理し、既定値より優先されます。価格通知は独立した目標価格条件です。配信先の変更にはサーバーの再起動が必要です。', admin: 'サーバー管理', rangeHelp: '保存された過去のシグナルであり、新しい推奨ではありません。範囲はズーム領域ではなく読み込み済みローソク足に基づきます。', setup: '表示・言語', connected: 'サーバー接続済み', disconnected: '再接続中', strategy: '戦略', risk: '損益距離 (ATR)', readGuide: 'チャートの読み方', language: '言語' },
}
export function useUXCopy() {
  const { i18n } = useTranslation()
  return copy[i18n.language.startsWith('ko') ? 'ko' : i18n.language.startsWith('ja') ? 'ja' : 'en']
}

// Keep complete rule names; abbreviations are reserved for chart markers.
const ruleLabels: Record<string, [string, string]> = {
  rsi_overbought_oversold: ['RSI 과매수·과매도', 'RSI買われすぎ・売られすぎ'], rsi_divergence: ['RSI 다이버전스', 'RSIダイバージェンス'],
  support_resistance_breakout: ['지지·저항 돌파', '支持・抵抗の突破'], ema_cross: ['EMA 교차', 'EMAクロス'], fibonacci_confluence: ['피보나치 합류', 'フィボナッチの重なり'],
  volume_spike: ['거래량 급증', '出来高急増'], vsa_effort_candle: ['VSA 거래량·가격 분석', 'VSA出来高・価格分析'],
  ict_order_block: ['ICT 오더 블록', 'ICTオーダーブロック'], ict_fair_value_gap: ['ICT 가격 불균형 (FVG)', 'ICT価格不均衡 (FVG)'], ict_liquidity_sweep: ['ICT 유동성 스윕', 'ICT流動性スイープ'],
  ict_breaker_block: ['ICT 브레이커 블록', 'ICTブレーカーブロック'], ict_kill_zone: ['ICT 주요 시간대', 'ICT主要時間帯'], ict_ote: ['ICT 되돌림 진입 구간', 'ICT押し戻りエントリー帯'], ict_amd_session: ['ICT 축적·조작·분배', 'ICT蓄積・操作・分配'],
  wyckoff_accumulation: ['와이코프 매집', 'ワイコフ蓄積'], wyckoff_distribution: ['와이코프 분산', 'ワイコフ分配'], wyckoff_spring: ['와이코프 스프링', 'ワイコフスプリング'], wyckoff_upthrust: ['와이코프 업스러스트', 'ワイコフアップスラスト'], wyckoff_volume_anomaly: ['와이코프 거래량 이상', 'ワイコフ出来高異常'],
  smc_bos: ['SMC 구조 돌파 (BOS)', 'SMC構造突破 (BOS)'], smc_choch: ['SMC 추세 성격 변화 (CHoCH)', 'SMCトレンド転換 (CHoCH)'],
  doji: ['도지', '十字線'], hammer: ['해머', 'ハンマー'], hanging_man: ['행잉맨', '首吊り線'], shooting_star: ['슈팅스타', '流れ星'], inverted_hammer: ['역해머', '逆ハンマー'], marubozu: ['장대 캔들 (마루보즈)', '丸坊主'],
  bullish_engulfing: ['상승 장악형', '強気の包み足'], bearish_engulfing: ['하락 장악형', '弱気の包み足'], bullish_harami: ['상승 잉태형', '強気のはらみ足'], bearish_harami: ['하락 잉태형', '弱気のはらみ足'],
  morning_star: ['샛별형', '明けの明星'], evening_star: ['석별형', '宵の明星'], three_white_soldiers: ['적삼병', '赤三兵'], three_black_crows: ['흑삼병', '黒三兵'],
}
export function readableRule(rule: string, language = 'en') {
  const label = ruleLabels[rule]
  if (label && language.startsWith('ko')) return label[0]
  if (label && language.startsWith('ja')) return label[1]
  return rule.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase()).replace(/\b(Rsi|Ema|Ict|Smc|Vsa|Ote|Amd|Bos)\b/g, c => c.toUpperCase()).replace(/\bChoch\b/g, 'CHoCH')
}
export function signalRange(time: number, bars: { time: number }[], timeframe: string) {
  if (!bars.length) return 'unavailable' as const
  const duration = { '1H': 3600, '4H': 14400, '1D': 86400, '1W': 604800 }[timeframe] ?? 0
  return time >= Math.min(...bars.map(b => b.time)) && time < Math.max(...bars.map(b => b.time)) + duration ? 'inRange' as const : 'archive' as const
}
