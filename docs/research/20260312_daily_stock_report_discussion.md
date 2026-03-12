# 팀 내 논의 보고서: 주식 전용 일일 리포트

- 논의일: 2026-03-12
- 주관: Orchestrator
- 참여 에이전트: Orchestrator, Researcher, MarketAnalyst, TraderAdvisor, Developer, Designer, Recorder
- 결과: **구현 권고 (RECOMMENDED)** — Owner 승인 대기

---

## 1. 논의 배경 및 안건

### Orchestrator 개시 발언

현재 플랫폼은 신호(Signal)가 발생할 때만 Telegram 알림을 발송하는
**이벤트 드리븐** 구조다. Owner로부터 "주식을 대상으로만 일일 리포트"를 추가하는
방안에 대한 팀 검토 요청이 접수됐다.

논의 안건:
1. 일일 리포트의 목적과 포함 내용은 무엇인가?
2. 기존 시스템과 어떻게 통합할 것인가?
3. 주식만 대상으로 하는 이유와 암호화폐 제외 논거는 타당한가?
4. 구현 난이도 및 우선순위는?
5. 보고서 포맷(Telegram 메시지 구조)은 어떻게 설계할 것인가?

---

## 2. 각 에이전트 의견

---

### 2-1. Researcher 의견

**안건 1 & 3 — 일일 리포트 목적 및 주식 집중의 근거**

현재 플랫폼의 주식 수집기는 **야간에도 데이터를 폴링**하지만,
미국 주식 시장(NYSE/NASDAQ)은 ET 기준 09:30~16:00에만 유효 데이터가 생성된다.
결과적으로 다음 문제가 발생한다:

- **신호 단편화**: 강한 신호가 장중에 여러 번 발생해도 각각 독립 알림으로 전송돼
  "오늘 하루 전체"를 조망하는 뷰가 없다.
- **맥락 부재**: 개별 신호에는 AI 해석이 붙지만, 하루를 마감할 때 "이 종목의 오늘
  총 신호 방향은 무엇인가?"를 한 눈에 볼 수 없다.
- **암호화폐는 24/7**: BTC/ETH는 종가·장 마감 개념이 없어 "일일"이라는 주기가
  자의적이다. 반면 주식은 미국 주식 종가(16:00 ET) 기준으로 자연스러운 마감이 존재.

**결론:**
- 일일 리포트의 주식 집중은 **시장 구조상 근거 있음** (VERIFIED).
- 발송 시점: **미국 장 마감 직후 16:15 ET (한국 시간 다음 날 05:15 KST)** 또는
  **한국 오전 9:00 KST** (다음 날 pre-market 확인용) 두 옵션이 유효.

**교차 확인 소스:**
- TradingView, StockCharts, Finviz 등 주요 주식 도구들이 모두 종가 기준 일일 리포트를 채택
- ICT 기법도 "Daily Candle Close" 기준 분석을 명시적으로 강조

---

### 2-2. MarketAnalyst 의견

**안건 1 & 2 — 포함 내용 및 기존 시스템 통합 방안**

현재 플랫폼의 신호 데이터(`signals` 테이블)와 OHLCV 데이터(`ohlcv` 테이블)를
조합하면 일일 리포트에 필요한 대부분의 정보를 즉시 조회할 수 있다.

#### 권고 포함 항목 (우선순위 순)

**[섹션 A] 종목별 일일 요약** (watchlist 전 종목)

| 항목 | 데이터 출처 | 설명 |
|------|------------|------|
| 종가 + 일간 등락률 | OHLCV (1D) | 가장 기본 정보 |
| 당일 발생 신호 수 / 방향 | signals 테이블 | BULLISH n건, BEARISH m건 |
| 최고 스코어 신호 | signals 테이블 | 오늘 가장 강한 신호 1건 |
| MTF 방향 합의 | 룰 엔진 재실행 | 1H/4H/1D/1W 각 방향 |
| 거래량 비교 | OHLCV | 20일 평균 대비 % |

**[섹션 B] 기술적 상태 요약**

| 항목 | 데이터 출처 |
|------|------------|
| RSI (1D) | 인디케이터 엔진 |
| EMA20 위/아래 | 인디케이터 엔진 |
| 주요 지지/저항 레벨 | Swing High/Low |
| ICT Order Block 유효 여부 | ICT 플러그인 |
| Wyckoff Phase 추정 | Wyckoff 플러그인 |

**[섹션 C] AI 종합 해석** (기존 AI 해석 레이어 재활용)

- 하루 전체 신호를 컨텍스트로 넣어 Claude API 1회 호출
- "오늘 [AAPL]의 전반적 방향성과 내일 주목할 레벨" 형식
- AI 미설정 시 자동 생략

**[섹션 D] 전체 watchlist 방향 요약 테이블**

```
📊 2026-03-12 일일 리포트
────────────────────────
AAPL  ▲  +1.8%  🟢 BULL ×3  RSI 58
MSFT  ▼  -0.6%  🔴 BEAR ×1  RSI 44
NVDA  ─  +0.1%  ⚪ NEUTRAL   RSI 52
────────────────────────
```

**인프라 통합:**
현재 `internal/notifier/` 패키지에 Telegram 발송 로직이 있고,
`internal/storage/` 에 신호·OHLCV 조회 메서드가 있다.
신규 `internal/report/` 패키지를 추가해 독립 스케줄러로 운영 가능.

---

### 2-3. TraderAdvisor 의견

```
## TraderAdvisor 의견
- 평가 대상: 주식 전용 일일 리포트
- 실전 유용성: 높음
```

**이유:**

👍 **장점**
- 하루에 신호 알림을 5~10개 받다 보면 "결국 오늘 이 종목 방향은 뭐야?"를
  판단하기 어렵다. 종가 기준 1장짜리 요약은 **다음 날 계획 수립에 직결**된다.
- MTF 방향 합의 테이블이 있으면 "1H은 BULL인데 1D는 BEAR"라는
  상충 신호를 한 눈에 인지할 수 있다 → 실전에서 가장 많이 실수하는 지점.
- AI 해석이 Telegram 메시지에 있으면 차트를 열기 전에 "오늘 AAPL 주목할 레벨이
  $215라는데 일단 체크해보자" 식의 워크플로우가 생긴다.
- **암호화폐 제외는 옳다.** 코인은 밤새 움직여서 아침에 일일 요약을 받아도
  이미 상황이 바뀌어 있는 경우가 많다.

⚠️ **우려 사항**
1. **노이즈 문제:** 신호가 없었던 날도 리포트가 오면 "별 내용 없음" 메시지가
   스팸처럼 쌓인다. → **신호 발생 종목만 섹션 A에 포함**하고, 무신호 종목은
   간단 줄 요약으로 처리하는 것을 권고.
2. **타이밍:** 16:15 ET(새벽 5시 KST)는 현실적으로 알림 확인 불가.
   **09:00 KST 발송이 더 실용적** — 미국 pre-market(08:00~09:30 ET) 시작 직전이므로
   전날 종가 분석과 오늘 pre-market 방향을 합쳐 판단하기에 최적.
3. **섹션 C(AI 해석)의 비용:** watchlist 6종목 × 매일 = 월 180회 Claude API 호출.
   현재 AI_MIN_SCORE 로직처럼, **신호 스코어 합산이 일정 기준 이상인 종목만**
   AI 호출하는 필터를 권고.

**개선 제안:**
- 발송 시점 2개를 설정 가능하게: `DAILY_REPORT_TIME_1=09:00` (KST), `DAILY_REPORT_TIME_2` (선택)
- 리포트 수신 채널을 신호 알림과 분리 가능하게 (다른 Telegram 채팅방 옵션)

**Orchestrator 권고: 구현 진행** (단, 우려 사항 3개를 스펙에 반영 후)

---

### 2-4. Developer 의견

**안건 4 — 구현 난이도 및 아키텍처**

**구현 난이도: M (Medium)** — 예상 LOC ~400, 테스트 ~12개

**필요 신규 파일:**

```
internal/
└── report/
    ├── daily.go          ← 리포트 생성 로직 (신호 집계 + 포맷팅)
    ├── scheduler.go      ← cron 스타일 스케줄러 (time.AfterFunc 기반)
    └── daily_test.go     ← 테스트
```

**기존 파일 수정 (최소):**

```
cmd/server/main.go        ← report.Scheduler 등록 (3~5줄)
config/config.go          ← DAILY_REPORT_TIME, DAILY_REPORT_MIN_SIGNALS 추가
```

**구현 접근:**
1. `scheduler.go`: 매일 설정된 시간(KST)에 `daily.go` 트리거
2. `daily.go`:
   - `storage.GetSignalsByDate(date)` → 당일 신호 조회
   - `storage.GetOHLCV(symbol, "1D", date)` → 종가 조회
   - `indicator`패키지로 RSI/EMA 재계산
   - `interpreter` 패키지 호출 (AI 해석, 조건부)
   - `notifier.SendTelegram()` 호출
3. 테스트: 신호 없는 날, 신호 많은 날, AI 비활성화 상태 등 엣지케이스 커버

**외부 의존성 추가 없음.** 기존 패키지 조합만으로 구현 가능.

**추가 고려:** Go의 `time.Location`으로 KST(Asia/Seoul) 시간대 처리 필요.
`DAILY_REPORT_TIMEZONE` 환경변수로 설정 가능하게 설계.

**Quality Gate 3 충족 가능 여부: YES**

---

### 2-5. Designer 의견

**안건 5 — Telegram 메시지 포맷 설계**

Telegram 메시지는 HTML/Markdown 포맷을 지원한다.
기존 알림과의 일관성을 유지하면서 일일 리포트만의 식별 가능한 헤더를 제안한다.

**제안 포맷 (Telegram Markdown):**

```
📅 *일일 리포트 — 2026-03-12*
미국 주식 종가 기준 | 한국시간 09:00

━━━━━━━━━━━━━━━━━━━━

📈 *AAPL* +1.8% ($214.50)
  신호: 🟢 BULL ×3 / 🔴 BEAR ×1
  MTF: 1H🟢 4H🟢 1D🟢 1W⚪
  RSI(1D): 58 | Vol: 평균 대비 +32%
  🏅 최강 신호: ICT Order Block (Score 14.2)
  🤖 AI: "215달러 Order Block이 유효하며 상승 구조 유지 중.
         내일 $218 저항 돌파 여부가 핵심."

─────────────────────

📉 *MSFT* -0.6% ($412.20)
  신호: 🔴 BEAR ×2
  MTF: 1H🔴 4H🔴 1D⚪ 1W🟢
  RSI(1D): 44 | Vol: 평균 대비 -12%
  🏅 최강 신호: SMC CHoCH (Score 9.8)

─────────────────────

⚪ *NVDA* +0.1% ($881.00)
  신호 없음 (Score 최대 3.2)
  MTF: 1H⚪ 4H⚪ 1D⚪ 1W🟢

━━━━━━━━━━━━━━━━━━━━
📊 요약: BULL 2 / NEUTRAL 1 / BEAR 0
⚙️ Chartter v0.2 | 다음 리포트: 내일 09:00
```

**디자인 원칙 적용 (Telegram 한계 내):**
- 대시(`━`, `─`)로 섹션 구분 → 여백 최소화 원칙
- 이모지로 색상 계층 표현 (5색 팔레트의 Telegram 대응)
- uppercase 레이블 없이 볼드(`*`)로 강조

**추가 제안:**
- 종목별 섹션 토글은 Telegram에서 불가 → 항목 수가 많으면
  `DAILY_REPORT_COMPACT=true` 옵션으로 테이블 축약 버전 제공

---

### 2-6. Recorder 선제 검토

> 구현 확정 시 아래 문서를 갱신할 것임을 미리 기록.

- `PRD.md`: Phase 2-6 항목 추가 (`[TODO]` → 구현 완료 후 `[DONE]`)
- `CHANGELOG.md`: v0.6 항목으로 기록 예정
- `docs/STATUS.md`: 다음 할 일 목록 갱신
- `SKILLS.md`: 주식 일일 리포트 기능 항목 추가

---

## 3. Orchestrator 종합 판단

### 논의 결론 요약

| 항목 | 결론 |
|------|------|
| 주식 집중 근거 | ✅ 타당 — 시장 종가 구조 기반, 코인 제외 합리적 |
| 포함 내용 | ✅ 합의 — 종가·신호·MTF·RSI·AI 해석 5개 섹션 |
| 발송 시간 | ✅ 합의 — 기본값 09:00 KST, 환경변수로 변경 가능 |
| AI 호출 조건 | ✅ 합의 — 일일 신호 합산 스코어 ≥ 임계값인 종목만 |
| 무신호 종목 처리 | ✅ 합의 — 줄 요약으로 표시 (별도 섹션 미생성) |
| 구현 난이도 | M (Medium), 신규 의존성 없음 |
| 우선순위 | Phase 2 마지막 항목으로 즉시 착수 가능 |

### 잠재 리스크

| 리스크 | 대응 |
|--------|------|
| KST 시간대 처리 오류 | `time.LoadLocation("Asia/Seoul")` 검증 테스트 포함 |
| AI 비용 급증 | `DAILY_REPORT_AI_MIN_SCORE` 환경변수로 임계값 제어 |
| 노이즈 알림 피로 | `DAILY_REPORT_ONLY_IF_SIGNALS=true` 옵션 — 무신호 날 발송 스킵 |
| Telegram 메시지 길이 초과 | 4,096자 제한 → 종목당 섹션 분할 발송 로직 필요 |

### 환경변수 제안 (config 추가 목록)

```env
DAILY_REPORT_ENABLED=true           # 기능 ON/OFF
DAILY_REPORT_TIME=09:00             # 발송 시간 (HH:MM, KST 기준)
DAILY_REPORT_TIMEZONE=Asia/Seoul    # 시간대 (기본 KST)
DAILY_REPORT_AI_MIN_SCORE=8.0       # AI 해석 호출 최소 일일 스코어 합
DAILY_REPORT_ONLY_IF_SIGNALS=false  # true 시 무신호 날 발송 스킵
DAILY_REPORT_COMPACT=false          # true 시 축약 포맷
```

### Owner 승인 필요: YES

이유: 신규 기능 추가 (PRD Phase 변경에 준하는 사항)

---

## 4. 구현 스펙 (Owner 승인 후 Developer에게 전달)

> `docs/queue/APPROVED_daily_stock_report.md`로 이동 예정

### 구현 목표

장 마감 이후 매일 1회, watchlist 내 미국 주식 종목을 대상으로
당일 신호·기술적 상태·AI 해석을 종합한 일일 리포트를 Telegram으로 발송한다.
암호화폐(Binance 수집기 종목)는 이 리포트에서 제외한다.

### 구현 범위

| 파일 | 작업 | 예상 LOC |
|------|------|----------|
| `internal/report/daily.go` | 신규: 리포트 생성 메인 로직 | ~200 |
| `internal/report/scheduler.go` | 신규: KST 기반 cron 스케줄러 | ~80 |
| `internal/report/daily_test.go` | 신규: 테스트 (≥12케이스) | ~150 |
| `config/config.go` | 수정: 환경변수 6개 추가 | ~20 |
| `cmd/server/main.go` | 수정: 스케줄러 등록 | ~5 |

### 테스트 케이스 (최소)

1. 무신호 날 — `ONLY_IF_SIGNALS=false` → 발송, `=true` → 스킵
2. 신호 있는 날 — 정상 포맷 생성
3. AI 활성화 + 스코어 충족 — AI 해석 섹션 포함
4. AI 활성화 + 스코어 미달 — AI 섹션 없음
5. Telegram 길이 초과 (종목 6개 풀 리포트) — 분할 발송
6. `DAILY_REPORT_ENABLED=false` — 스케줄러 비활성
7. 시간대 파싱 오류 — 명확한 에러 로그
8. 주식 종목 필터 — Binance 코인 종목 제외 확인
9. 스케줄러 시간 계산 — 내일 09:00 KST 정확히 계산
10. COMPACT 모드 — 축약 포맷 생성
11. OHLCV 데이터 없음 — graceful skip
12. AI API 오류 — AI 없이 리포트 발송 (degraded mode)

### 완료 정의 (Definition of Done)

- [ ] `go test ./internal/report/...` 전체 PASS
- [ ] `go test ./...` 기존 테스트 깨지지 않음
- [ ] `DAILY_REPORT_ENABLED=true` 상태에서 `docker compose up` 후 지정 시간에 Telegram 메시지 수신 확인
- [ ] `DAILY_REPORT_ENABLED=false` 시 스케줄러 미동작 확인
- [ ] 코인 종목(BTC, ETH 등) 리포트에서 제외 확인

---

*논의 참여: Orchestrator, Researcher, MarketAnalyst, TraderAdvisor, Developer, Designer, Recorder*
*문서화: Recorder — 2026-03-12*
*상태: PENDING Owner 승인*
