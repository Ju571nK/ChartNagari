# SKILLS.md — 현재 구현 가능 목록

> Developer와 Orchestrator가 작업 가능 여부를 판단하기 위해 읽는 파일.
> Recorder가 구현 완료 시마다 갱신한다.

---

## 범례
- ✅ 구현 완료
- 🔨 구현 중
- 📋 스펙 확정, 구현 대기
- 🔬 Researcher 조사 중
- ❌ 미지원 (이유 명시)

---

## 데이터 수집

| 기능 | 상태 | 비고 |
|------|------|------|
| Binance WebSocket (코인 실시간) | 📋 | Phase 1-2 |
| Yahoo Finance REST Polling (주식) | 📋 | Phase 1-3 |
| OHLCV → SQLite 저장 | 📋 | Phase 1-2 |
| 타임프레임 자동 재구성 (1H/4H/1D/1W) | 📋 | Phase 1-2 |
| Alpha Vantage (보조 주식) | ❌ | 무료 25req/day 제한, Phase 2 검토 |
| Bloomberg API | ❌ | 유료, Phase 2+ |

---

## 인디케이터 엔진

| 인디케이터 | 상태 | 파라미터 |
|-----------|------|---------|
| RSI | 📋 | period=14 |
| MACD | 📋 | 12/26/9 |
| EMA | 📋 | 9/21/50/200 |
| SMA | 📋 | 20/50/200 |
| Bollinger Bands | 📋 | period=20, std=2 |
| OBV | 📋 | - |
| Volume MA | 📋 | period=20 |
| Swing High/Low | 📋 | lookback=5 |
| ATR | 📋 | period=14 |
| Fibonacci Retracement | 📋 | 0.236/0.382/0.5/0.618/0.786 |

---

## 방법론 (Analysis Rules)

| 방법론 | 룰 | 상태 | Research 완료 |
|--------|-----|------|--------------|
| 일반 기술적분석 | RSI 과매수/과매도 | 📋 | ✅ |
| 일반 기술적분석 | RSI 다이버전스 | 📋 | ✅ |
| 일반 기술적분석 | 지지/저항 돌파 | 📋 | ✅ |
| 일반 기술적분석 | EMA 크로스 | 📋 | ✅ |
| 일반 기술적분석 | 거래량 급등 | 📋 | ✅ |
| 일반 기술적분석 | Fibonacci Confluence | 📋 | ✅ |
| ICT | Order Block | 📋 | ✅ |
| ICT | Fair Value Gap | 📋 | ✅ |
| ICT | Liquidity Sweep | 📋 | ✅ |
| ICT | Breaker Block | 📋 | ✅ |
| ICT | Kill Zone | 📋 | ✅ |
| Wyckoff | Accumulation Phase | 📋 | ✅ |
| Wyckoff | Distribution Phase | 📋 | ✅ |
| Wyckoff | Spring | 📋 | ✅ |
| Wyckoff | Upthrust | 📋 | ✅ |
| Wyckoff | Volume Anomaly | 📋 | ✅ |
| SMC | CHoCH | 🔬 | 미완료 |
| SMC | BOS | 🔬 | 미완료 |
| Elliott Wave | 파동 카운팅 | ❌ | 복잡도 높음, Phase 2 |

---

## 알림 시스템

| 기능 | 상태 | 비고 |
|------|------|------|
| Telegram Bot 발송 | 📋 | Phase 1-7 |
| Discord Webhook | 📋 | Phase 1-7 |
| 신호 스코어링 (가중합산) | 📋 | Phase 1-7 |
| 중복 알림 방지 (쿨다운) | 📋 | Phase 1-7 |
| Slack | ❌ | Phase 2 |
| Email (일간 리포트) | ❌ | Phase 2 |

---

## 프론트엔드 (설정 UI)

| 기능 | 상태 | 비고 |
|------|------|------|
| 종목 추가/삭제 | 📋 | Phase 1-10 |
| 방법론 룰 ON/OFF | 📋 | Phase 1-10 |
| 알림 채널 설정 | 📋 | Phase 1-10 |
| 수집기 상태 모니터링 | 📋 | Phase 1-10 |
| 차트 대시보드 | ❌ | Phase 2 |
| 백테스팅 UI | ❌ | Phase 2 |

---

## 인프라

| 기능 | 상태 | 비고 |
|------|------|------|
| Docker Compose (로컬) | 📋 | Phase 1-1 |
| SQLite 로컬 저장 | 📋 | Phase 1-2 |
| YAML 룰 설정 | 📋 | Phase 1-5 |
| .env 환경변수 관리 | 📋 | Phase 1-1 |
| 구조화 로깅 (zerolog) | 📋 | Phase 1-1 |
| 클라우드 배포 | ❌ | Phase 3 |

---

## 마지막 갱신
- Date: 2026-03-07
- Updated by: Recorder (초기 설정)
- Next review: Phase 1-1 완료 시