# 작업 진행 요약 — Chart Analyzer

> 작성일: 2026-03-07
> 작성: Recorder

---

## 프로젝트 개요

미국 주식 및 암호화폐(BTC, ETH 등)를 대상으로 ICT / Wyckoff / 일반 기술적분석 방법론을
플러그인 방식으로 탑재하여 MTF(1W/1D/4H/1H) 신호를 자동 감지하고 Telegram/Discord로
알림을 발송하는 **로컬 실행 플랫폼**.

- 스택: Go 1.26 백엔드 + TypeScript/React 프론트엔드(예정) + SQLite + Rancher Desktop

---

## 환경 세팅

| 항목 | 결과 |
|------|------|
| Go 설치 | 1.26.1 (brew install go) |
| Docker | Rancher Desktop (`~/.rd/bin/docker`, `docker-compose`) |
| GitHub | https://github.com/Ju571nK/Chatter.git |
| 모듈명 | `github.com/Ju571nK/Chatter` |

---

## 완료된 작업

### Phase 0: Setup ✅

| 파일 | 설명 |
|------|------|
| `CLAUDE.md` | Claude Code 진입점 |
| `AGENTS.md` | 멀티에이전트 운영 구조 (Orchestrator/Researcher/MarketAnalyst/Developer/Recorder) |
| `PRD.md` | 제품 요구사항 + Phase 로드맵 + 의존관계 다이어그램 |
| `SKILLS.md` | 구현 가능 목록 (상태 추적) |
| `CHANGELOG.md` | 전체 변경 이력 |
| `docs/STATUS.md` | 팀 현재 상태 (Orchestrator 관리) |
| `docs/research/`, `queue/`, `approved/`, `pending/` | 워크플로우 폴더 구조 |

문서 보강 작업:
- CHANGELOG 파일 경로 오기재 수정 (`docs/PRD.md` → `PRD.md` 등)
- PRD Phase 1 항목에 의존관계 다이어그램 추가 (1-1 → 1-2/1-3 → 1-4 → ...)

---

### Phase 1-1: 프로젝트 구조 ✅

| 파일 | 설명 |
|------|------|
| `go.mod` | 모듈 초기화 (`github.com/Ju571nK/Chatter`) |
| `cmd/server/main.go` | 서버 진입점 (zerolog, 설정 로드, 수집기 연결, graceful shutdown) |
| `pkg/models/signal.go` | 공유 모델 (`Signal`, `OHLCV`, `AnalysisContext`) |
| `internal/rule/interface.go` | `AnalysisRule` 플러그인 인터페이스 |
| `Dockerfile` | 멀티 스테이지 빌드 (Go 1.22 builder + alpine 런타임) |
| `docker-compose.yml` | SQLite 볼륨 마운트 + 헬스체크 |
| `.env.example` | 환경변수 템플릿 |
| `.gitignore` | `.env`, 바이너리, DB 제외 |
| `config/rules.yaml` | 전체 룰 설정 (ICT/Wyckoff/일반TA, 초기 비활성) |
| `config/watchlist.yaml` | 종목 설정 (BTCUSDT/ETHUSDT 활성, AAPL/NVDA 준비) |

---

### Phase 1-2: Binance WebSocket 수집기 ✅

**파일**: `internal/collector/binance.go`

- Binance Combined Stream 구독 (`wss://stream.binance.com:9443/stream?streams=...`)
- 심볼 × 타임프레임(1H/4H/1D/1W) 자동 스트림 URL 생성
- 확정된 캔들(`isClosed: true`)만 SQLite에 저장
- 자동 재연결 (5초 간격), ping/pong keepalive (30초)
- Context 기반 graceful shutdown

---

### Phase 1-3: Yahoo Finance 주식 수집기 ✅

**파일**: `internal/collector/yahoo.go`

- Yahoo Finance v8 Chart API 공개 엔드포인트 활용 (인증 불필요)
- 타임프레임별 파라미터 자동 매핑 (1H/4H/1D/1W → interval + range)
- 장중 시간 감지 (NYSE 기준, UTC 14:30–21:00, 월–금)
  - 장외 시간: 1D/1W만 폴링 (리소스 절약)
  - 장중 시간: 전 타임프레임 폴링
- 4H 데이터는 1H → 4H 재구성으로 처리

---

### 공통 인프라 ✅

| 파일 | 설명 |
|------|------|
| `internal/config/config.go` | `.env` + `rules.yaml` + `watchlist.yaml` 통합 로더 |
| `internal/storage/db.go` | SQLite 초기화, WAL 모드, 스키마 마이그레이션 |
| `internal/storage/ohlcv.go` | OHLCV CRUD (`SaveOHLCV`, `SaveOHLCVBatch`, `GetOHLCV`, `GetOHLCVSince`) |
| `internal/collector/timeframe.go` | 1H 캔들 → 4H/1D/1W 자동 재구성 |

---

### Phase 1-4: 인디케이터 엔진 ✅

**패키지**: `internal/indicator/`

- `Compute(bars map[string][]OHLCV) map[string]float64` — 전체 TF 일괄 계산
- 키 형식: `"{TF}:{지표명}"` (예: `"1H:RSI_14"`, `"4H:EMA_200"`)
- 구현 인디케이터:
  - RSI(14) — Wilder's smoothing
  - EMA(9/20/50/200), SMA(20/50/200), VolumeMA(20)
  - MACD(12,26,9) — line/signal/histogram
  - Bollinger Bands(20, 2σ) — upper/middle/lower/width/%B
  - OBV — 누적 거래량 방향 지표
  - ATR(14) — Wilder's smoothing
  - Swing High/Low (lookback=5)
  - Fibonacci 7레벨 (0/23.6/38.2/50/61.8/78.6/100%)
- 데이터 부족 시 해당 키 미설정 (ok=false 패턴)

---

### Phase 1-5: 룰 엔진 ✅

**패키지**: `internal/engine/`

- `RuleEngine.Register(rule)` — active 룰만 등록
- `RuleEngine.Run(ctx)` — RequiredIndicators 검증 후 Analyze 호출, Score 정렬 반환
- Score = 룰점수 × TFWeight × RuleEntry.Weight
- TF 가중치: 1W=2.0 / 1D=1.5 / 4H=1.2 / 1H=1.0
- YAML config 연동 (`RuleConfig`, `RuleEntry`)

---

## 테스트 현황

| 패키지 | 테스트 수 | 결과 |
|--------|---------|------|
| `internal/collector` | 6 | ✅ PASS |
| `internal/storage` | 5 | ✅ PASS |
| `internal/indicator` | 14 | ✅ PASS |
| `internal/engine` | 10 | ✅ PASS |
| **합계** | **35** | **전체 PASS** |

---

## 현재 프로젝트 디렉토리 구조

```
Chatter/
├── AGENTS.md
├── CLAUDE.md
├── CHANGELOG.md
├── PRD.md
├── SKILLS.md
├── Dockerfile
├── docker-compose.yml
├── go.mod / go.sum
├── .env.example
├── .gitignore
├── cmd/
│   └── server/main.go
├── config/
│   ├── rules.yaml
│   └── watchlist.yaml
├── internal/
│   ├── collector/
│   │   ├── binance.go
│   │   ├── timeframe.go
│   │   ├── timeframe_test.go
│   │   └── yahoo.go
│   ├── config/
│   │   └── config.go
│   ├── engine/                ← Phase 1-5 (NEW)
│   │   ├── config.go
│   │   ├── engine.go
│   │   └── engine_test.go
│   ├── indicator/             ← Phase 1-4 (NEW)
│   │   ├── atr.go
│   │   ├── bb.go
│   │   ├── ema_sma.go
│   │   ├── fibonacci.go
│   │   ├── indicator.go
│   │   ├── indicator_test.go
│   │   ├── macd.go
│   │   ├── obv.go
│   │   ├── rsi.go
│   │   └── swing.go
│   ├── rule/
│   │   └── interface.go
│   └── storage/
│       ├── db.go
│       ├── ohlcv.go
│       └── ohlcv_test.go
├── pkg/
│   └── models/
│       └── signal.go
├── web/               ← Phase 1-10에서 사용
└── docs/
    ├── STATUS.md
    ├── PROGRESS_SUMMARY.md  ← 이 파일
    ├── research/
    ├── queue/
    ├── approved/
    └── pending/
```

---

## 의존 패키지

| 패키지 | 용도 |
|--------|------|
| `github.com/rs/zerolog` | 구조화 로깅 |
| `github.com/gorilla/websocket` | Binance WebSocket 클라이언트 |
| `modernc.org/sqlite` | 순수 Go SQLite (CGO 불필요) |
| `gopkg.in/yaml.v3` | YAML 설정 파일 파싱 |
| `github.com/joho/godotenv` | .env 파일 로드 |

---

## 다음 단계

| 단계 | 항목 | 상태 |
|------|------|------|
| Phase 1-4 | 인디케이터 엔진 (RSI/MACD/EMA/SMA/BB/OBV/ATR/Fibonacci) | ✅ 완료 |
| Phase 1-5 | 룰 엔진 (AnalysisRule 인터페이스 구현 + YAML 로더 + 스코어링) | ✅ 완료 |
| Phase 1-6 | 일반 기술적분석 방법론 플러그인 | 대기 중 |
| Phase 1-7 | Telegram/Discord 알림 시스템 | 대기 중 |
| Phase 1-8 | ICT 방법론 플러그인 | 대기 중 |
| Phase 1-9 | Wyckoff 방법론 플러그인 | 대기 중 |
| Phase 1-10 | React + TypeScript 설정 UI | 대기 중 |
