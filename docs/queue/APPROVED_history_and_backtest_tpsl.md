# APPROVED: 신호 히스토리 탭 + 백테스트 TP/SL 배율 수동 설정

- 승인일: 2026-03-13 (TraderAdvisor 권고 → Orchestrator 자율 승인)
- Owner 승인 불필요: 기존 기능 강화 범위
- 대상 에이전트: Developer

---

## Feature 1: 신호 히스토리 탭

### 목적
웹 UI에서 과거 발생 신호를 종목·방향·건수로 필터링하여 테이블로 조회한다.
Telegram 알림 히스토리를 앱에서 재검토할 수 있게 한다.

### 백엔드 변경

#### storage — GetSignalsFiltered 추가
```go
// GetSignalsFiltered returns signals with optional filters.
// symbol="" or "ALL" → all symbols. direction="" or "ALL" → all directions.
func (db *DB) GetSignalsFiltered(symbol, direction string, limit int) ([]models.Signal, error)
```
Query:
```sql
SELECT id, symbol, timeframe, rule, direction, score, message, ai_interpretation, created_at
FROM signals
WHERE (? = 'ALL' OR symbol = ?)
  AND (? = 'ALL' OR direction = ?)
ORDER BY created_at DESC
LIMIT ?
```

#### api/server.go — GET /api/history 엔드포인트 추가
```
GET /api/history?symbol=ALL&direction=ALL&limit=100
```
- symbol: 특정 심볼 or "ALL" (기본 ALL)
- direction: LONG / SHORT / ALL (기본 ALL)
- limit: 1~200 (기본 100)
- 응답: SignalBar[] (기존 타입 재사용, symbol 필드 추가)

ChartStore 인터페이스에 GetSignalsFiltered 추가:
```go
type ChartStore interface {
    GetOHLCV(symbol, timeframe string, limit int) ([]models.OHLCV, error)
    GetSignals(symbol string, limit int) ([]models.Signal, error)
    GetSignalsFiltered(symbol, direction string, limit int) ([]models.Signal, error)  // 추가
}
```

SignalBar에 symbol 필드 추가:
```go
type SignalBar struct {
    Symbol           string  `json:"symbol"`   // 추가
    Time             int64   `json:"time"`
    ... (기존 필드 유지)
}
```

### 프론트엔드 변경 (App.tsx)

Tab 타입에 `'history'` 추가.

`HistoryTab` 컴포넌트:
- 마운트 시 `/api/symbols`로 심볼 목록 로드 (ALL 옵션 포함)
- 필터: symbol 셀렉터 (ALL, AAPL, ...) + direction 셀렉터 (ALL, LONG, SHORT) + limit 셀렉터 (50/100/200)
- 필터 변경 시 자동 재조회 (`GET /api/history?...`)
- 결과 테이블:

| 시간 | 종목 | TF | 방향 | 룰 | 스코어 | AI 해석 |
|------|------|-----|------|-----|--------|---------|

- 방향 셀: `dir-long` / `dir-short` 클래스
- AI 해석: 있으면 축약 표시 (max 80자, 전체는 title tooltip)
- 결과 없음: "신호 없음 — 수집 기간이 짧거나 필터 조건 확인" 안내문

CSS: 기존 `.backtest-table` 스타일 재사용 (신규 클래스 불필요)

탭 네비게이션에 '히스토리' 버튼 추가.

---

## Feature 2: 백테스트 TP/SL ATR 배율 수동 입력

### 목적
백테스트 폼에서 TP 배율과 SL 배율을 직접 입력하여
자신의 트레이딩 전략에 맞는 손익비로 백테스트한다.

### 백엔드 변경

#### backtest/runner.go — RunBacktest 시그니처 변경
```go
// RunBacktest loads historical bars and runs the backtest engine.
// tpMult > 0 overrides the engine's default TPATRMultiplier.
// slMult > 0 overrides the engine's default SLATRMultiplier.
func (r *Runner) RunBacktest(symbol, timeframe, ruleFilter string, tpMult, slMult float64) (*BacktestResult, error) {
    bars, err := r.store.GetOHLCVAll(symbol, timeframe)
    if err != nil {
        return nil, err
    }
    cfg := r.engine.cfg
    if tpMult > 0 {
        cfg.TPATRMultiplier = tpMult
    }
    if slMult > 0 {
        cfg.SLATRMultiplier = slMult
    }
    // Temporarily run with overridden config
    tmpEngine := NewWithConfig(r.engine.rules, r.engine.ruleCfg, cfg)
    res := tmpEngine.Run(symbol, timeframe, ruleFilter, bars)
    return &res, nil
}
```

**Engine 변경**: `cfg`, `rules`, `ruleCfg` 필드를 패키지 내 접근 가능하게(소문자 유지, 같은 패키지 내 접근) + `NewWithConfig` 함수 추가:
```go
func NewWithConfig(rules []rule.AnalysisRule, ruleCfg engine.RuleConfig, cfg Config) *Engine
```

#### api/server.go — runBacktest 핸들러 변경
요청 바디에 `tp_mult`, `sl_mult` 추가:
```go
var req struct {
    Symbol    string  `json:"symbol"`
    Timeframe string  `json:"timeframe"`
    Rule      string  `json:"rule"`
    TPMult    float64 `json:"tp_mult"` // 0 = use default
    SLMult    float64 `json:"sl_mult"` // 0 = use default
}
```
Runner 호출: `r.backtestRunner.RunBacktest(req.Symbol, req.Timeframe, req.Rule, req.TPMult, req.SLMult)`

#### api/server.go — BacktestRunner 인터페이스 변경
```go
type BacktestRunner interface {
    RunBacktest(symbol, timeframe, ruleFilter string, tpMult, slMult float64) (*backtest.BacktestResult, error)
}
```

### 프론트엔드 변경 (App.tsx)

`BacktestTab`의 controls에 두 개 number input 추가:
- TP 배율: `<input type="number" step="0.5" min="0.5" max="10" defaultValue="2.0">`  (레이블: "TP 배율")
- SL 배율: `<input type="number" step="0.5" min="0.5" max="10" defaultValue="1.0">` (레이블: "SL 배율")

POST /api/backtest 바디에 `tp_mult`, `sl_mult` 포함.

백테스트 결과 헤더에 배율 표시:
```
결과 — AAPL 1H (200 바, 32 거래) | TP×2.0 SL×1.0
```

CSS: `number` 타입 input은 기존 `.symbol-input` 클래스에 `width: 80px` 추가 스타일 인라인 적용. 레이블은 `.item-meta` 클래스 활용.

---

## 완료 정의

- [ ] `go test ./...` 전체 PASS
- [ ] GET /api/history?symbol=ALL 응답 확인
- [ ] 백테스트 TP 배율=3.0, SL 배율=1.5로 실행 후 결과 변화 확인 (수동)
- [ ] 히스토리 탭에서 필터 변경 시 테이블 자동 갱신 확인

---

*승인: Orchestrator (TraderAdvisor 권고 기반) — 2026-03-13*
