# PRD.md — Chart Analyzer 제품 요구사항

> **살아있는 문서다.** Recorder가 지속적으로 갱신한다.
> 상태 태그: `[TODO]` `[IN PROGRESS]` `[DONE]` `[BLOCKED]`
> Owner 승인 없이 Phase 변경 금지.

---

## 메타

| 항목 | 내용 |
|------|------|
| 버전 | v0.1 |
| 최종 갱신 | 2026-03-07 |
| 현재 Phase | **Phase 1: Core MVP** |
| Owner | (사장) |

---

## 1. 제품 목적

미국 주식 및 암호화폐 시장에서 4~6개 종목을 지속 모니터링하고,
ICT / Wyckoff / 일반 기술적분석 방법론을 플러그인 방식으로 실행하여
Telegram으로 자동 신호를 발송하는 **로컬 실행 플랫폼**.

**핵심 가치:**
- 방법론을 코드 수정 없이 추가/교체 가능한 플러그인 구조
- MTF(1W/1D/4H/1H) 동시 분석으로 신호 품질 향상
- 사람은 사장처럼 — 방향만 제시하고 팀이 알아서 운영

---

## 2. 타겟 자산 및 데이터

### 자산 유형
- 미국 주식 (NYSE/NASDAQ)
- 암호화폐 (Binance 기준)

### 모니터링 종목 수
- 4~6개 (watchlist.yaml로 관리)

### 타임프레임 (MTF)
| TF | 역할 | 가중치 |
|----|------|--------|
| 1W | 거시 추세 | × 2.0 |
| 1D | 주요 레벨 | × 1.5 |
| 4H | 진입 구간 | × 1.2 |
| 1H | 신호 확인 | × 1.0 |

---

## 3. Phase 로드맵

### Phase 0: Setup `[DONE]`
- [x] CLAUDE.md 생성
- [x] AGENTS.md 생성
- [x] PRD.md 생성
- [x] SKILLS.md 생성
- [x] CHANGELOG.md 생성
- [x] Go 프로젝트 scaffold
- [x] Docker Compose 기본 구성

### Phase 1: Core MVP `[DONE]`

> **의존관계 (구현 순서)**
> ```
> 1-1 프로젝트 구조
>  └─→ 1-2 데이터 수집기 (코인)
>  └─→ 1-3 주식 수집기
>        └─→ 1-4 인디케이터 엔진
>              └─→ 1-5 룰 엔진 인터페이스
>                    ├─→ 1-6 일반 기술적분석
>                    ├─→ 1-8 ICT 방법론
>                    └─→ 1-9 Wyckoff 방법론
>                          └─→ 1-7 알림 시스템
>                                └─→ 1-10 프론트엔드 설정 UI
> ```
> 1-2와 1-3은 병렬 진행 가능. 1-6/1-8/1-9도 병렬 진행 가능.

#### 1-1. 프로젝트 구조 `[DONE]` ← 시작점
- Go 모듈 초기화
- 디렉토리 구조 생성
- Docker Compose (Go + SQLite)
- .env 환경변수 구성
- zerolog 구조화 로깅

#### 1-2. 데이터 수집기 `[DONE]` ← 의존: 1-1
- Binance WebSocket (코인 OHLCV 실시간)
- OHLCV → SQLite 저장
- 타임프레임 자동 재구성

#### 1-3. 주식 수집기 `[DONE]` ← 의존: 1-1 (1-2와 병렬 가능)
- Yahoo Finance REST Polling
- 장중/장외 시간 구분 처리

#### 1-4. 인디케이터 엔진 `[DONE]` ← 의존: 1-2, 1-3
- RSI, MACD, EMA, SMA
- Bollinger Bands, OBV, Volume MA
- Swing High/Low, ATR, Fibonacci

#### 1-5. 룰 엔진 인터페이스 `[DONE]` ← 의존: 1-4
- `AnalysisRule` 인터페이스 정의
- YAML 설정 파일 로더
- 신호 스코어링 (TF 가중치 × 룰 강도)

#### 1-6. 일반 기술적분석 방법론 `[DONE]` ← 의존: 1-5 (1-8, 1-9와 병렬 가능)
- RSI 과매수/과매도, 다이버전스
- 지지/저항 돌파
- EMA 크로스
- Fibonacci Confluence
- Volume Spike

#### 1-7. 알림 시스템 `[DONE]` ← 의존: 1-5 (룰 엔진에서 신호 수신)
- Telegram Bot 발송
- Discord Webhook
- 중복 방지 쿨다운 (4시간)
- 스코어 임계값 필터

#### 1-8. ICT 방법론 `[DONE]` ← 의존: 1-5 (1-6, 1-9와 병렬 가능)
- Order Block
- Fair Value Gap
- Liquidity Sweep
- Breaker Block
- Kill Zone (세션 시간 가중치)

#### 1-9. Wyckoff 방법론 `[DONE]` ← 의존: 1-5 (1-6, 1-8과 병렬 가능)
- Accumulation / Distribution Phase
- Spring / Upthrust
- Volume Anomaly

#### 1-10. 프론트엔드 설정 UI `[DONE]` ← 의존: 1-7 (최종 단계)
- 종목 추가/삭제 (React + TypeScript)
- 방법론 룰 ON/OFF
- Telegram/Discord 설정
- 수집기 상태 표시

### Phase 2: Enhancement `[IN PROGRESS]`

#### 2-1. AI 해석 레이어 `[DONE]` ← 의존: Phase 1 완료
- Claude API (`claude-opus-4-6`) 통합
- 룰 엔진 신호 → AI 컨텍스트 프롬프트 → 해석 텍스트 → Telegram/Discord 첨부
- Score ≥ `AI_MIN_SCORE`(기본 12.0) 그룹만 AI 호출 (비용 최적화)
- API 키 미설정 시 자동 비활성화 (Phase 1 동작과 동일)
- `internal/interpreter/`, `internal/pipeline/` 패키지 신설

#### 2-2. SMC 방법론 (CHoCH, BOS) `[DONE]` ← Research 백로그 높음
- Change of Character (CHoCH)
- Break of Structure (BOS)

#### 2-3. 차트 대시보드 `[DONE]`
- TradingView Lightweight Charts (v5)
- OHLCV 캔들차트 + 신호 마커 오버레이
- 종목 셀렉터 + TF 세그먼트 컨트롤 (1H/4H/1D/1W)
- 신호 영속성 (SQLite signals 테이블)

#### 2-4. 백테스팅 엔진 `[TODO]`

#### 2-5. Bloomberg/유료 데이터 피드 `[TODO]`

### Phase 3: Cloud `[TODO]`
- 클라우드 배포
- 멀티 유저 지원

---

## 4. 플러그인 구조 (불변 원칙)

```go
// 모든 방법론 룰이 구현해야 하는 인터페이스
type AnalysisRule interface {
    Name()                string
    RequiredIndicators()  []string
    Analyze(ctx AnalysisContext) (*Signal, error)
}

// 방법론 추가 = 파일 하나 + YAML 한 줄
// 기존 코드 수정 금지 (Open/Closed Principle)
```

---

## 5. 신호 스코어링

```
Score = Σ(룰 강도 × TF 가중치)
  + MTF 일치 보너스 (3개 TF 이상 동일 방향: +3점)

알림 임계값:
  Score ≥ 5  : 약한 신호 (참고용)
  Score ≥ 8  : 중간 신호
  Score ≥ 12 : 강한 신호 → 즉시 Telegram 발송
```

---

## 6. Research 백로그 (Researcher 조사 예정)

| 기법 | 우선순위 | 상태 |
|------|---------|------|
| SMC - CHoCH / BOS | 높음 | 🔬 조사 예정 |
| Elliott Wave 자동화 | 낮음 | 🔬 조사 예정 |
| VWAP 기반 신호 | 중간 | 🔬 조사 예정 |
| Order Flow (Footprint) | 낮음 | 🔬 조사 예정 |

---

## 7. 비기능 요구사항

| 항목 | 기준 |
|------|------|
| 성능 | 6종목 × 4TF = 24 시계열 동시 처리, CPU 10% 미만 |
| 안정성 | goroutine panic → 자동 재시작 |
| 보안 | API Key는 .env 또는 OS 환경변수, 코드 하드코딩 금지 |
| 확장성 | 방법론 추가 = 파일 + YAML만으로 완결 |
| 실행 | `docker compose up` 1개 커맨드로 전체 기동 |

---

## 8. 의사결정 히스토리

| 날짜 | 결정 | 이유 |
|------|------|------|
| 2026-03-07 | Go 백엔드 선택 | goroutine으로 다종목 동시 처리에 적합 |
| 2026-03-07 | SQLite 선택 | 로컬 실행, 별도 DB 서버 불필요 |
| 2026-03-07 | Telegram 우선 | Discord보다 봇 설정이 간단 |
| 2026-03-07 | Bloomberg 제외 (Phase 1) | 유료, 대체제로 Yahoo+Binance 충분 |
| 2026-03-07 | MTF 4개 TF 확정 | 1W/1D/4H/1H — 스윙 중심 분석에 최적 |
| 2026-03-07 | 플러그인 구조 채택 | 방법론 추가를 코드 수정 없이 가능하게 |