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

<!-- 이후 항목은 Recorder가 자동으로 추가한다 -->