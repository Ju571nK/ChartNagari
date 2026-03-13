# APPROVED: 코인/주식 별도 TP/SL 배율 + 룰별 성과 리포트

- 승인일: 2026-03-13
- 배경: 페이퍼 트레이딩 승률 개선 + 신호 품질 가시성 확보
- 대상 에이전트: Developer

---

## Feature C: 코인/주식 별도 TP/SL 배율 (프론트엔드 조정 가능)

### config/alert.yaml 추가 필드

```yaml
# 기존 필드 유지 + 아래 추가
crypto_tp_mult: 1.5
crypto_sl_mult: 0.75
stock_tp_mult: 2.0
stock_sl_mult: 1.0
```

### internal/config/config.go 변경

`AlertConfig`에 4개 필드 추가:
```go
CryptoTPMult float64 `yaml:"crypto_tp_mult"`
CryptoSLMult float64 `yaml:"crypto_sl_mult"`
StockTPMult  float64 `yaml:"stock_tp_mult"`
StockSLMult  float64 `yaml:"stock_sl_mult"`
```

`Load()` 기본값 설정:
```go
cfg.Alert = AlertConfig{
    ScoreThreshold:  12.0,
    CooldownHours:   4,
    MTFConsensusMin: 2,
    CryptoTPMult:    1.5,
    CryptoSLMult:    0.75,
    StockTPMult:     2.0,
    StockSLMult:     1.0,
}
```

### internal/pipeline/pipeline.go 변경

`Pipeline` 구조체에 `cryptoSyms map[string]bool` 추가.

메서드 추가:
```go
func (p *Pipeline) SetCryptoSymbols(syms []string) {
    p.cryptoSyms = make(map[string]bool, len(syms))
    for _, s := range syms { p.cryptoSyms[s] = true }
}
func (p *Pipeline) isCrypto(sym string) bool {
    return p.cryptoSyms != nil && p.cryptoSyms[sym]
}
```

`enrichSignalLevels` 시그니처 변경:
```go
func enrichSignalLevels(
    sig *models.Signal,
    allBars map[string][]models.OHLCV,
    indicators map[string]float64,
    tpMult, slMult float64,   // 추가
)
```

내부 하드코딩 2.0/1.0 → 파라미터로 교체.

`analyzeSymbol`에서 루프 변경:
```go
// TP/SL 배율 결정 (코인/주식 분리)
tpMult, slMult := 2.0, 1.0
if p.alertHolder != nil {
    ac := p.alertHolder.Get()
    if p.isCrypto(sym) {
        tpMult, slMult = ac.CryptoTPMult, ac.CryptoSLMult
    } else {
        tpMult, slMult = ac.StockTPMult, ac.StockSLMult
    }
}
for i := range signals {
    enrichSignalLevels(&signals[i], allBars, indicators, tpMult, slMult)
}
```

### cmd/server/main.go 변경

pipeline 생성 후 추가:
```go
pipe.SetCryptoSymbols(cryptoSymbols)
```

### web/src/App.tsx — AlertTab 변경

기존 3개 필드 아래에 "TP/SL 배율" 섹션 추가:

```
─── TP/SL 배율 설정 ─────────────────────────
  코인 TP 배율   [1.5] (number, step 0.25, min 0.5)
  코인 SL 배율   [0.75] (number, step 0.25, min 0.25)
  주식 TP 배율   [2.0] (number, step 0.25, min 0.5)
  주식 SL 배율   [1.0] (number, step 0.25, min 0.25)
섹션 설명: "TP = 진입가 ± ATR × 배율. 낮을수록 빠른 청산, 높을수록 큰 목표가"
```

---

## Feature D: 룰별 성과 리포트

### internal/backtest/engine.go 변경

메서드 추가:
```go
// RuleNames returns the names of all rules registered in this engine.
func (e *Engine) RuleNames() []string {
    names := make([]string, len(e.rules))
    for i, r := range e.rules { names[i] = r.Name() }
    return names
}
```

### internal/backtest/runner.go 변경

새 타입 추가:
```go
// RuleStats summarizes backtest performance for a single rule.
type RuleStats struct {
    Rule         string  `json:"rule"`
    Trades       int     `json:"trades"`
    WinRate      float64 `json:"win_rate"`
    AvgRR        float64 `json:"avg_rr"`
    ProfitFactor float64 `json:"profit_factor"`
    MaxDrawdown  float64 `json:"max_drawdown"`
    TotalReturn  float64 `json:"total_return_pct"`
}
```

새 메서드 추가:
```go
// RunPerRule runs a backtest for each individual rule and returns per-rule stats.
// Rules with 0 trades are excluded from results.
// Results are sorted by WinRate descending.
func (r *Runner) RunPerRule(symbol, timeframe string, tpMult, slMult float64) ([]RuleStats, error) {
    bars, err := r.store.GetOHLCVAll(symbol, timeframe)
    if err != nil {
        return nil, err
    }

    eng := r.engine
    if tpMult > 0 || slMult > 0 {
        cfg := r.engine.cfg
        if tpMult > 0 { cfg.TPATRMultiplier = tpMult }
        if slMult > 0 { cfg.SLATRMultiplier = slMult }
        eng = r.engine.Clone(cfg)
    }

    ruleNames := r.engine.RuleNames()
    var stats []RuleStats
    for _, name := range ruleNames {
        result := eng.Run(symbol, timeframe, name, bars)
        if result.Trades == 0 { continue }
        stats = append(stats, RuleStats{
            Rule:         name,
            Trades:       result.Trades,
            WinRate:      result.Stats.WinRate,
            AvgRR:        result.Stats.AvgRR,
            ProfitFactor: result.Stats.ProfitFactor,
            MaxDrawdown:  result.Stats.MaxDrawdown,
            TotalReturn:  result.Stats.TotalReturnPct,
        })
    }
    sort.Slice(stats, func(i, j int) bool {
        return stats[i].WinRate > stats[j].WinRate
    })
    return stats, nil
}
```
`sort` 패키지 import 추가.

### internal/api/server.go 변경

`BacktestRunner` 인터페이스에 추가:
```go
RunPerRule(symbol, timeframe string, tpMult, slMult float64) ([]backtest.RuleStats, error)
```

라우트 등록 (`Handler()`에 추가):
```go
mux.HandleFunc("GET /api/backtest/rules", s.runPerRuleBacktest)
```

핸들러 구현:
```go
func (s *Server) runPerRuleBacktest(w http.ResponseWriter, r *http.Request) {
    if s.backtestRunner == nil {
        http.Error(w, "backtest runner not configured", http.StatusServiceUnavailable)
        return
    }
    symbol := r.URL.Query().Get("symbol")
    timeframe := r.URL.Query().Get("timeframe")
    if symbol == "" || timeframe == "" {
        http.Error(w, "symbol and timeframe are required", http.StatusBadRequest)
        return
    }
    var tpMult, slMult float64
    if v := r.URL.Query().Get("tp_mult"); v != "" {
        tpMult, _ = strconv.ParseFloat(v, 64)
    }
    if v := r.URL.Query().Get("sl_mult"); v != "" {
        slMult, _ = strconv.ParseFloat(v, 64)
    }
    stats, err := s.backtestRunner.RunPerRule(symbol, timeframe, tpMult, slMult)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    if stats == nil { stats = []backtest.RuleStats{} }
    jsonOK(w, stats)
}
```

### web/src/App.tsx — BacktestTab 변경

`RuleStats` 인터페이스 추가:
```ts
interface RuleStats {
  rule: string
  trades: number
  win_rate: number
  avg_rr: number
  profit_factor: number
  max_drawdown: number
  total_return_pct: number
}
```

`BacktestTab` 상태 추가:
```ts
const [ruleStats, setRuleStats] = useState<RuleStats[] | null>(null)
const [rulesLoading, setRulesLoading] = useState(false)
```

기존 "실행" 버튼 옆에 "룰별 분석" 버튼 추가:
```tsx
<button
  className="run-btn"
  style={{ background: 'rgba(91,146,121,0.15)', color: 'var(--green)' }}
  onClick={runPerRule}
  disabled={rulesLoading || !symbol}
>
  {rulesLoading ? '분석 중...' : '룰별 분석'}
</button>
```

`runPerRule` 콜백:
```ts
const runPerRule = useCallback(async () => {
  if (!symbol) return
  setRulesLoading(true)
  setRuleStats(null)
  try {
    const stats = await apiFetch<RuleStats[]>(
      `/backtest/rules?symbol=${encodeURIComponent(symbol)}&timeframe=${tf}&tp_mult=${tpMult}&sl_mult=${slMult}`
    )
    setRuleStats(stats)
  } catch (e: unknown) {
    setError(e instanceof Error ? e.message : '알 수 없는 오류')
  } finally {
    setRulesLoading(false)
  }
}, [symbol, tf, tpMult, slMult])
```

백테스트 결과(result) 아래에 룰별 분석 테이블 렌더링:
```tsx
{ruleStats && ruleStats.length > 0 && (
  <>
    <p className="section-title" style={{ marginTop: 24 }}>
      룰별 성과 분석 — {symbol} {tf} | TP×{tpMult} SL×{slMult}
    </p>
    <div className="backtest-table-wrap">
      <table className="backtest-table">
        <thead>
          <tr>
            <th>룰</th>
            <th>거래 수</th>
            <th>승률</th>
            <th>평균 RR</th>
            <th>수익 팩터</th>
            <th>누적 수익</th>
          </tr>
        </thead>
        <tbody>
          {ruleStats.map((s) => (
            <tr key={s.rule}>
              <td style={{ color: 'var(--muted)', maxWidth: 160, overflow: 'hidden', textOverflow: 'ellipsis' }}>
                {s.rule}
              </td>
              <td>{s.trades}</td>
              <td style={{ color: s.win_rate >= 0.45 ? 'var(--mint)' : 'var(--muted)', fontWeight: 600 }}>
                {(s.win_rate * 100).toFixed(1)}%
              </td>
              <td>{s.avg_rr.toFixed(2)}</td>
              <td>{s.profit_factor.toFixed(2)}</td>
              <td className={s.total_return_pct >= 0 ? 'pnl-win' : 'pnl-loss'}>
                {s.total_return_pct >= 0 ? '+' : ''}{s.total_return_pct.toFixed(2)}%
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
    <p className="item-meta" style={{ marginTop: 8 }}>
      🟢 승률 ≥ 45% 기준 달성 | 🔴 기준 미달 — 해당 룰 OFF 또는 임계값 상향 고려
    </p>
  </>
)}
{ruleStats && ruleStats.length === 0 && (
  <p className="loading" style={{ marginTop: 16 }}>거래 데이터 없음 — OHLCV 수집 기간을 늘려주세요.</p>
)}
```

---

## 테스트 케이스

- `Engine.RuleNames()` — 등록된 룰 이름 반환 확인
- `Runner.RunPerRule()` — 빈 bars → 빈 결과, 정상 데이터 → stats 정렬 확인
- `enrichSignalLevels` — tpMult/slMult 파라미터 반영 확인

---

*승인: Orchestrator (Quant 설계 기반) — 2026-03-13*
