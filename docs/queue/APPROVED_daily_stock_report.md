# APPROVED: 주식 전용 일일 리포트

- 승인일: 2026-03-13
- Owner 추가 요구사항: **웹 UI에서 설정 변경 가능**
- 근거 문서: `docs/research/20260312_daily_stock_report_discussion.md`
- 대상 에이전트: Developer

---

## 구현 목표

장 마감 이후 매일 1회, watchlist 내 미국 주식 종목을 대상으로
당일 신호·기술적 상태·AI 해석을 종합한 일일 리포트를 Telegram으로 발송한다.
암호화폐(Binance 수집기 종목)는 이 리포트에서 제외한다.
**모든 설정은 웹 UI(설정 탭)에서 변경·저장 가능해야 한다.**

---

## 구현 범위

### 신규 파일

| 파일 | 작업 | 예상 LOC |
|------|------|----------|
| `config/report.yaml` | 신규: 일일 리포트 기본 설정 | ~15 |
| `internal/report/daily.go` | 신규: 리포트 생성 메인 로직 | ~200 |
| `internal/report/scheduler.go` | 신규: KST 기반 cron 스케줄러 | ~80 |
| `internal/report/daily_test.go` | 신규: 테스트 (≥12케이스) | ~150 |

### 기존 파일 수정

| 파일 | 변경 내용 | 예상 LOC |
|------|-----------|----------|
| `internal/config/config.go` | DailyReportConfig 구조체 + loadReportConfig 추가 | ~30 |
| `internal/api/server.go` | GET/PUT /api/report/config 엔드포인트 추가 | ~60 |
| `cmd/server/main.go` | report.Scheduler 등록 (5줄) | ~5 |
| `web/src/App.tsx` | ReportTab 컴포넌트 추가 + '리포트' 탭 추가 | ~120 |

---

## 설정 구조 (config/report.yaml)

```yaml
enabled: true
time: "09:00"           # HH:MM (KST 기준)
timezone: "Asia/Seoul"
ai_min_score: 8.0       # AI 해석 호출 최소 일일 스코어 합
only_if_signals: false  # true 시 무신호 날 발송 스킵
compact: false          # true 시 축약 포맷
```

---

## API 엔드포인트 (웹 UI 연동)

```
GET  /api/report/config  → ReportConfig JSON 반환
PUT  /api/report/config  → 설정 갱신 후 report.yaml 저장 + 스케줄러 재설정
```

### ReportConfig JSON 스키마

```json
{
  "enabled": true,
  "time": "09:00",
  "timezone": "Asia/Seoul",
  "ai_min_score": 8.0,
  "only_if_signals": false,
  "compact": false
}
```

---

## 웹 UI 스펙 (Designer 제공)

탭 이름: **리포트** (Tab key: `'report'`)

### 섹션 구성

1. **리포트 활성화** — enabled 토글 (기존 Toggle 컴포넌트 재사용)
2. **발송 시간** — HH:MM 텍스트 입력 (placeholder: "09:00")
3. **시간대** — 텍스트 표시 (Asia/Seoul 고정, 변경 불가 — 향후 확장)
4. **AI 호출 최소 스코어** — number input (step: 0.5, min: 0)
5. **무신호 날 스킵** — only_if_signals 토글
6. **축약 모드** — compact 토글
7. **저장 버튼** — 변경사항 PUT /api/report/config 전송

### 색상 규칙 (Designer 팔레트 준수)

- 섹션 레이블: `--muted` uppercase
- 입력 테두리: `rgba(91,146,121,0.25)` → hover: `rgba(91,146,121,0.5)`
- 저장 버튼: 기존 `.run-btn` 클래스 재사용
- 활성 토글: `--green`
- 저장 성공 메시지: `--mint`

---

## 내부 구현 상세

### internal/report/daily.go 핵심 로직

```go
type DailyReporter struct {
    store    ReportStore     // storage.DB (신호 + OHLCV 조회)
    notif    Notifier        // notifier.Notifier (Telegram 발송)
    interp   Interpreter     // interpreter.Interpreter (AI 해석, nil 허용)
    cfg      *ReportConfig   // 현재 설정 포인터
    log      zerolog.Logger
}

type ReportStore interface {
    GetSignalsByDate(symbol string, date time.Time) ([]models.Signal, error)
    GetOHLCV(symbol, timeframe string, limit int) ([]models.OHLCV, error)
    GetStockSymbols() ([]string, error)
}

func (r *DailyReporter) Generate(ctx context.Context, date time.Time) error
```

### internal/report/scheduler.go 핵심 로직

```go
type Scheduler struct {
    reporter *DailyReporter
    cfg      *ReportConfig
    log      zerolog.Logger
    timer    *time.Timer
    mu       sync.Mutex
}

// Start는 ctx가 취소될 때까지 매일 cfg.Time KST에 Generate를 호출한다.
func (s *Scheduler) Start(ctx context.Context)

// Reset은 새 설정으로 스케줄러를 재설정한다 (PUT /api/report/config 호출 시).
func (s *Scheduler) Reset(cfg ReportConfig)
```

---

## 테스트 케이스 (최소 12개)

1. 무신호 날 — `only_if_signals=false` → 발송, `=true` → 스킵
2. 신호 있는 날 — 정상 포맷 생성
3. AI 활성화 + 스코어 충족 — AI 해석 섹션 포함
4. AI 활성화 + 스코어 미달 — AI 섹션 없음
5. Telegram 길이 초과 (종목 6개 풀 리포트) — 분할 발송
6. `enabled=false` — 스케줄러 비활성
7. 시간대 파싱 오류 — 명확한 에러 로그
8. 주식 종목 필터 — Binance 코인 종목 제외 확인
9. 스케줄러 시간 계산 — 내일 09:00 KST 정확히 계산
10. COMPACT 모드 — 축약 포맷 생성
11. OHLCV 데이터 없음 — graceful skip
12. AI API 오류 — AI 없이 리포트 발송 (degraded mode)

---

## 완료 정의 (Definition of Done)

- [ ] `go test ./internal/report/...` 전체 PASS
- [ ] `go test ./...` 기존 테스트 깨지지 않음
- [ ] `GET /api/report/config` 정상 응답
- [ ] `PUT /api/report/config` 후 report.yaml 파일 갱신 확인
- [ ] 웹 UI 리포트 탭에서 설정 변경 후 저장 버튼 작동 확인
- [ ] `enabled=true` 상태에서 지정 시간에 Telegram 메시지 수신 확인 (수동 검증)
- [ ] 코인 종목(BTC, ETH 등) 리포트에서 제외 확인

---

*승인: Owner (2026-03-13)*
*스펙 작성: Orchestrator + Designer*
