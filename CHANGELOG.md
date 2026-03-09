# CHANGELOG.md

> Recorder 에이전트가 테스트 통과 후에만 기록한다.
> 미완성/진행중 항목은 기록하지 않는다.

---

## 형식

```
## [버전 or 날짜] - YYYY-MM-DD
### Added    → 새 기능
### Changed  → 변경된 기능
### Fixed    → 버그 수정
### Removed  → 제거된 기능
### Research → Researcher가 채택한 새 기법
### Docs     → 문서 변경
```

---

## [0.1.0] - 2026-03-07

### Docs
- 프로젝트 초기 문서 세트 생성
  - `CLAUDE.md` : 프로젝트 진입점
  - `AGENTS.md` : 멀티에이전트 운영 지시서
  - `PRD.md` : 제품 요구사항 문서 v0.1
  - `SKILLS.md` : 구현 가능 목록 초안
  - `CHANGELOG.md` : 이 파일

### Research
- ICT (Order Block, FVG, Liquidity Sweep, Breaker Block, Kill Zone) — 사전 검증 완료
- Wyckoff (Accumulation, Distribution, Spring, Upthrust, Volume Anomaly) — 사전 검증 완료
- 일반 기술적분석 (RSI, S/R, EMA Cross, Fibonacci, Volume Spike) — 사전 검증 완료

---

## [1.5.0] - 2026-03-08

### Added
- `internal/paper/trader.go`: 실시간 페이퍼 트레이딩 엔진 (PaperTrader, PaperPosition, PaperSummary)
- `internal/paper/trader_test.go`: 10개 테스트 PASS (오픈/중복방지/제로진입/TP/SL/구바 무시/룰필터/요약/Long-Short레벨/멀티심볼)
- `internal/storage/paper.go`: DB CRUD (SavePaperPosition, GetOpenPositions, GetAllOpenPositions, ClosePaperPosition, GetClosedPositions)
- `internal/storage/db.go`: `paper_positions` 테이블 + 인덱스 스키마 추가

### Changed
- `internal/pipeline/pipeline.go`: PaperTrader 인터페이스 + SetPaperTrader + analyzeSymbol에 OnSignals/CheckPositions 연결
- `internal/api/server.go`: PaperStore 인터페이스 + GET /api/paper/positions, /history, /summary 엔드포인트
- `cmd/server/main.go`: paper.New() 초기화 + pipe.SetPaperTrader + apiSrv.WithPaperStore
- `web/src/App.tsx`: PaperTab 컴포넌트 (요약 카드 6개 + 오픈 포지션 테이블 + 청산 히스토리 테이블)

---

## [1.4.0] - 2026-03-08

### Added
- `internal/collector/tiingo.go`: Tiingo REST 수집기 (1D/1W = daily endpoint, 1H/4H = IEX intraday endpoint)
- `internal/config/config.go`: TiingoConfig 추가 (TIINGO_API_KEY, TIINGO_POLL_INTERVAL)
- `.env.example`: TIINGO_API_KEY, TIINGO_POLL_INTERVAL 항목 추가

### Changed
- `cmd/server/main.go`: TIINGO_API_KEY 설정 시 Tiingo 수집기 우선 사용, 미설정 시 Yahoo fallback
- PRD.md: Phase 2-5 방향 → "Yahoo → Tiingo 대체"로 업데이트

---

## [1.3.0] - 2026-03-08

### Added
- `pkg/models/signal.go`: EntryPrice, TP, SL 필드 추가 (ATR 기반 거래 레벨)
- `internal/pipeline/pipeline.go`: enrichSignalLevels() — 신호 발생 시 진입가/TP/SL 자동 계산
- `internal/notifier/format.go`: fmtPrice() 헬퍼 + formatTelegram에 💰 진입/TP/SL 라인 추가
- `internal/notifier/discord.go`: Discord embed fields에 진입가/TP/SL 항목 추가
- `internal/notifier/notifier_test.go`: 포맷 테스트 3개 추가 (WithLevels, NoLevelsWhenZero, ContainsFields)
- `docs/research/20260308_free_data_sources.md`: 무료 데이터 소스 VERIFIED 리서치 (Tiingo 1순위 권고)

### Research
- 무료 데이터 소스 조사 완료: Tiingo(1순위) > Polygon.io(주식 전용) > Alpha Vantage(낮은 한도) → VERIFIED

---

## [1.2.0] - 2026-03-08

### Changed
- AGENTS.md v0.3: TraderAdvisor 에이전트 추가 (실전 트레이더 자문 역할)
- PRD.md: Phase 2-5 Bloomberg → `[BLOCKED]` (유료 API 계약 불가)
- Orchestrator 트리거: VERIFIED 기법 → TraderAdvisor 실전 코멘트 연결
- Orchestrator 트리거: 새 UI 기능 → TraderAdvisor 유용성 검토 추가

---

## [1.1.0] - 2026-03-08

### Added
- `internal/backtest/engine.go`: 슬라이딩 윈도우 룰 재실행 엔진 (ATR 기반 TP/SL 시뮬레이션)
- `internal/backtest/stats.go`: ComputeStats — 승률, 평균손익비, 수익팩터, MDD, 샤프비율, 누적수익률, 최대연속손실
- `internal/backtest/runner.go`: OHLCVLoader 인터페이스 + Runner (스토리지 + 엔진 통합)
- `internal/backtest/engine_test.go`: 10개 테스트 PASS (Empty, InsufficientBars, LongTP, ShortTP, LongSL, Timeout, Filter, Stats, StatsEmpty, MaxDrawdown)
- `internal/storage/ohlcv.go`: GetOHLCVAll — 전체 바 오름차순 조회 (백테스트 전용)
- `internal/api/server.go`: BacktestRunner 인터페이스 + WithBacktestRunner + `POST /api/backtest` 핸들러
- `web/src/App.tsx`: BacktestTab 컴포넌트 (설정 폼, 통계 카드 6개, 거래 목록 테이블)
- `web/src/App.css`: 백테스트 탭 스타일 (.backtest-controls, .run-btn, .backtest-stats, .backtest-table)

### Changed
- `cmd/server/main.go`: allRules 슬라이스 도입 (룰 엔진 + 백테스트 엔진 공유), BacktestEngine/Runner 연결
- PRD.md: Phase 2-4 → `[DONE]`
- docs/STATUS.md: 팀 상태 갱신

---

## [1.0.0] - 2026-03-08

### Added
- `internal/storage/signals.go`: SaveSignal, GetSignals — 신호 영속성
- `internal/pipeline/pipeline.go`: SignalSaver 인터페이스 + SetSignalSaver + 신호 자동 저장
- `internal/api/server.go`: ChartStore 인터페이스 + `GET /api/ohlcv/{symbol}/{tf}` + `GET /api/signals` 엔드포인트
- `web/src/App.tsx`: ChartTab 컴포넌트 (TradingView Lightweight Charts v5, 캔들차트 + 신호 마커)
- `web/src/App.css`: 차트 컨트롤 스타일 (chart-controls, tf-group, chart-area) + .badge-smc
- `lightweight-charts` npm 패키지 (v5.1.0)

### Changed
- `internal/storage/db.go`: signals 테이블 스키마 추가
- `cmd/server/main.go`: DB → API WithChartStore, Pipeline SetSignalSaver 연결
- PRD.md: Phase 2-3 → `[DONE]`

---

## [0.9.0] - 2026-03-08

### Added
- `internal/methodology/smc/helpers.go`: trendDir, structuralHigh, structuralLow 공통 헬퍼
- `internal/methodology/smc/bos.go`: SMCBOSRule — Break of Structure (추세 지속 신호)
- `internal/methodology/smc/choch.go`: SMCChoCHRule — Change of Character (추세 전환 신호)
- `internal/methodology/smc/smc_test.go`: SMC 패키지 테스트 14개 전체 PASS
- `config/rules.yaml`: smc_bos (strength: 5.0), smc_choch (strength: 6.0) 항목 추가
- `cmd/server/main.go`: SMCBOSRule, SMCChoCHRule 룰 엔진 등록

### Changed
- PRD.md: Phase 2-2 → `[DONE]`
- docs/STATUS.md: 팀 상태 갱신

---

## [0.3.0] - 2026-03-07

### Added
- `internal/config/config.go`: .env + YAML 통합 설정 로더
- `internal/storage/db.go`: SQLite 초기화, WAL 모드, 스키마 마이그레이션
- `internal/storage/ohlcv.go`: OHLCV CRUD (SaveOHLCV, SaveOHLCVBatch, GetOHLCV, GetOHLCVSince)
- `internal/collector/binance.go`: Binance WebSocket 수집기 (자동 재연결, 확정 캔들만 저장)
- `internal/collector/yahoo.go`: Yahoo Finance REST 수집기 (장중/장외 시간 구분)
- `internal/collector/timeframe.go`: 1H → 4H/1D/1W 자동 재구성 유틸리티
- `cmd/server/main.go`: 수집기 goroutine 연결, SIGTERM graceful shutdown
- 테스트 11개 (collector 6, storage 5) — 전체 PASS

### Changed
- PRD.md: 1-1, 1-2, 1-3 → `[DONE]`
- 의존성 추가: `modernc.org/sqlite`, `gorilla/websocket`, `yaml.v3`, `godotenv`

---

## [0.2.0] - 2026-03-07

### Added
- Go 프로젝트 디렉토리 구조 생성 (`cmd/`, `internal/`, `pkg/`, `config/`, `web/`)
- `go.mod` 초기화 (`github.com/Ju571nK/Chatter`, Go 1.26)
- `cmd/server/main.go` — zerolog 구조화 로깅 포함 서버 진입점
- `pkg/models/signal.go` — 공유 데이터 모델 (`Signal`, `OHLCV`, `AnalysisContext`)
- `internal/rule/interface.go` — `AnalysisRule` 플러그인 인터페이스 정의
- `config/rules.yaml` — 전체 룰 설정 파일 (모든 룰 비활성 상태로 초기 세팅)
- `config/watchlist.yaml` — 모니터링 종목 설정 (BTC, ETH 활성화)
- `Dockerfile` — 멀티 스테이지 빌드 (builder + alpine 런타임)
- `docker-compose.yml` — SQLite 볼륨 마운트 + 헬스체크 포함
- `.env.example` — 환경변수 템플릿
- `.gitignore` — `.env`, 바이너리, SQLite 데이터 제외

### Changed
- PRD.md: Phase 0 → `[DONE]`, Phase 1 → `[IN PROGRESS]`

## [0.4.0] - 2026-03-07

### Added
- `internal/indicator/` 패키지 — 인디케이터 엔진 (Phase 1-4)
  - `indicator.go`: `Compute(bars map[string][]OHLCV) map[string]float64` — 전체 TF 인디케이터 일괄 계산, 키 형식 `"{TF}:{지표명}"` (예: `"1H:RSI_14"`)
  - `rsi.go`: RSI(14) — Wilder's smoothing
  - `ema_sma.go`: EMA(9/20/50/200), SMA(20/50/200), VolumeMA(20)
  - `macd.go`: MACD(12,26,9) — line/signal/histogram
  - `bb.go`: Bollinger Bands(20, 2σ) — upper/middle/lower/width/%B
  - `obv.go`: OBV (누적 거래량 방향 지표)
  - `atr.go`: ATR(14) — Wilder's smoothing
  - `swing.go`: Swing High/Low (lookback=5)
  - `fibonacci.go`: Fibonacci 7레벨 (0/23.6/38.2/50/61.8/78.6/100%)
  - `indicator_test.go`: 14개 테스트 — 전체 PASS
- `internal/engine/` 패키지 — 룰 엔진 (Phase 1-5)
  - `config.go`: `RuleConfig`, `RuleEntry`, `TFWeight()` (1W=2.0/1D=1.5/4H=1.2/1H=1.0)
  - `engine.go`: `RuleEngine` — Register/Run, RequiredIndicators 검증, Score=룰점수×TF가중치×룰가중치, 내림차순 정렬
  - `engine_test.go`: 10개 테스트 — 전체 PASS

### Changed
- PRD.md: Phase 1-4, 1-5 → `[DONE]`
- 전체 테스트: 25개 PASS (기존 11개 유지 + 신규 14개)

---

## [0.5.0] - 2026-03-07

### Added
- `internal/methodology/general_ta/` 패키지 — 일반 기술적분석 플러그인 (Phase 1-6)
  - `helpers.go`: 패키지 내부 유틸리티 (`rollingRSI`, `rollingEMA`, `swingLowPair`, `swingHighPair`)
  - `rsi_overbought_oversold.go`: RSI(14)≥70 → SHORT, ≤30 → LONG, 전 TF 스캔
  - `rsi_divergence.go`: 가격/RSI 다이버전스 감지 (강세/약세), rollingRSI 내부 계산
  - `ema_cross.go`: EMA(9)/EMA(20) 골든크로스·데드크로스 감지
  - `support_resistance_breakout.go`: SWING_HIGH/LOW 돌파 감지
  - `fibonacci_confluence.go`: 가격이 주요 피보나치 레벨(0.5% 허용오차) 근처일 때 신호
  - `volume_spike.go`: 거래량 2×MA20 초과 시 방향 신호
  - `general_ta_test.go`: 18개 테스트 — 전체 PASS
- `internal/methodology/ict/` 패키지 — ICT 방법론 플러그인 (Phase 1-8)
  - `order_block.go`: 마지막 약세/강세 캔들 → 충격파 → 가격 복귀 시 신호
  - `fair_value_gap.go`: 3캔들 불균형 갭(FVG) 감지, 가격이 갭 진입 시 신호
  - `liquidity_sweep.go`: 스윙 레벨 위/아래 위크 돌파 후 복귀 — 유동성 스윕 신호
  - `breaker_block.go`: 실패한 오더블록(브레이커) 감지 — 반대 방향 신호
  - `kill_zone.go`: 런던(08:00-11:00 UTC) / 뉴욕(13:00-16:00 UTC) 킬존 시간 감지
  - `ict_test.go`: 15개 테스트 — 전체 PASS
- `internal/methodology/wyckoff/` 패키지 — Wyckoff 방법론 플러그인 (Phase 1-9)
  - `accumulation.go`: 좁은 레인지(<8%) + EMA50 하단 + 낮은 거래량 → LONG
  - `distribution.go`: 좁은 레인지 + EMA50 상단 + 낮은 거래량 → SHORT
  - `spring.go`: 스윙저점 아래 위크 후 복귀 + 고거래량 → LONG
  - `upthrust.go`: 스윙고점 위 위크 후 반전 + 고거래량 → SHORT
  - `volume_anomaly.go`: 거래량 2.5×MA20 초과 → 방향 신호
  - `wyckoff_test.go`: 14개 테스트 — 전체 PASS

### Changed
- `config/rules.yaml`: 16개 룰 모두 `enabled: true` 활성화 (구현 완료)
- `PRD.md`: Phase 1-6, 1-8, 1-9 → `[DONE]`
- 전체 테스트: 82개 PASS (기존 35개 유지 + 신규 47개)

---

## [0.6.0] - 2026-03-07

### Added
- `internal/notifier/` 패키지 — Telegram/Discord 알림 시스템 (Phase 1-7)
  - `notifier.go`: `Notifier` — 스코어 임계값 필터, 쿨다운 검사, 멀티 Sender 디스패치
  - `cooldown.go`: `Cooldown` — `{symbol}|{rule}` 키 기반 in-memory 쿨다운 (기본 4시간)
  - `format.go`: `formatTelegram`, `discordColor`, `directionIcon` 메시지 포매터
  - `telegram.go`: `TelegramSender` — Bot API `/sendMessage`, HTML parse_mode
  - `discord.go`: `DiscordSender` — Webhook embed 메시지 (컬러 코딩: 녹색/빨간/황색)
  - `notifier_test.go`: 18개 테스트 — 전체 PASS (httptest.Server로 실제 HTTP 검증 포함)

### Design
- `Sender` 인터페이스로 백엔드 교체/확장 가능 (Slack 등 추후 추가 용이)
- HTTP 클라이언트 주입 가능 → 테스트에서 실제 API 호출 없음
- 쿨다운 시계(`now` func) 주입 가능 → 만료 테스트 가능

### Changed
- 전체 테스트: 100개 PASS (기존 82개 유지 + 신규 18개)

---

## [0.7.0] - 2026-03-07

### Added
- `internal/api/` 패키지 — Go REST API 서버 (Phase 1-10)
  - `server.go`: `Server` — 5개 엔드포인트, YAML 파일 읽기/쓰기, CORS 미들웨어
    - `GET /api/status` — 시스템 요약 (phase, symbols, rules, tests)
    - `GET /api/symbols` — 전체 종목 목록 (crypto + stock)
    - `PUT /api/symbols/{symbol}` — 종목 enabled 토글
    - `GET /api/rules` — 전체 룰 목록
    - `PUT /api/rules/{name}` — 룰 enabled 토글
    - `GET /` — React 정적 파일 서빙 (web/dist/)
  - `api_test.go`: 16개 테스트 — 전체 PASS
- `web/` — React + TypeScript 설정 UI
  - `vite.config.ts`: 개발 모드에서 `/api/*` → Go 서버 프록시
  - `src/App.tsx`: 3탭 UI (종목 / 룰 / 상태), 실시간 토글 반영
  - `src/App.css`: 다크 테마, 방법론별 컬러 배지
  - `npm run build` → `web/dist/` 빌드 성공 (27 모듈, 148KB JS)

### Changed
- `cmd/server/main.go`: HTTP API 서버 goroutine 추가 (port `:8080`)
- `PRD.md`: Phase 1-10 → `[DONE]`, Phase 1 전체 → `[DONE]`
- 전체 테스트: 116개 PASS (기존 100개 유지 + 신규 16개)

### 🎉 Phase 1: Core MVP 완료

## [0.8.0] - 2026-03-08

### Added
- `internal/interpreter/` 패키지 — Claude AI 해석 레이어 (Phase 2-1)
  - `interpreter.go`: `Interpreter` — `New(apiKey, minScore, clientOpts...)`, `Enrich(ctx, []SignalGroup) []Signal`
  - SignalGroup 총 스코어 ≥ minScore 일 때만 Claude API 호출 (비용 절감)
  - API 키 미설정 시 자동 비활성화 (파이프라인 전체 정상 동작 유지)
  - 모델: `claude-opus-4-6` / max_tokens: 600 / 언어: 한국어 200자
  - 오류 시 원본 신호 그대로 반환 (Graceful degradation)
  - `interpreter_test.go`: 7개 테스트 — 전체 PASS (httptest.Server 기반)
- `internal/pipeline/` 패키지 — 분석 파이프라인 (Phase 2-1)
  - `pipeline.go`: SQLite → 인디케이터 → 룰 엔진 → AI 해석 → 알림 전체 연결
  - `OHLCVReader` 인터페이스로 DB 의존성 분리 (테스트 용이)
  - 1분 간격 ticker, 심볼별 독립 처리 (한 심볼 실패가 다른 심볼에 영향 없음)
  - `pipeline_test.go`: 6개 테스트 — 전체 PASS

### Changed
- `pkg/models/signal.go`: `Signal`에 `AIInterpretation string` 필드 추가
- `internal/notifier/format.go`: AI 해석이 있으면 Telegram 메시지 끝에 `💡 <i>해석</i>` 추가
- `internal/notifier/discord.go`: AI 해석이 있으면 embed description에 `💡 해석` 추가
- `internal/config/config.go`: `AnthropicConfig{APIKey, MinScore}` 추가 + `parseFloat` 헬퍼
- `cmd/server/main.go`: 룰 엔진 플러그인 전체 등록, Notifier/Interpreter 초기화, 파이프라인 goroutine 시작, `toEngineConfig()` 변환 함수
- `.env.example`: `ANTHROPIC_API_KEY`, `AI_MIN_SCORE` 추가
- 의존성 추가: `github.com/anthropics/anthropic-sdk-go v1.26.0`
- 전체 테스트: 129개 PASS (기존 116개 유지 + 신규 13개)

### Design
- 룰 엔진 (마이크로초, 무료, 24/7) → AI (레이턴시, 유료, 선택) 1차/2차 필터 구조
- `ANTHROPIC_API_KEY` 미설정 = Phase 1 동작과 100% 동일 (하위 호환)

---

<!-- 이후 항목은 Recorder가 자동으로 추가한다 -->