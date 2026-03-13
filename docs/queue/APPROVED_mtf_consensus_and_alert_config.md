# APPROVED: MTF 합의 필터 + 알림 설정 웹 UI

- 승인일: 2026-03-13
- 배경: 페이퍼 트레이딩 승률 33.3% (QG2 기준 45% 미달) → Quant 분석 → Owner 승인
- 대상 에이전트: Developer

---

## Quant 분석 보고

- 분석 기간: 2026-03-08 ~ 2026-03-13 (5일)
- 샘플 수: 9건 (통계적 유의미성 미확보 — 방향성 지표로만 사용)

**핵심 문제:**
- 단일 TF 신호만으로도 포지션 생성 → 추세와 역행하는 1H SHORT 신호가 반복적으로 SL 도달
- BTCUSDT: SHORT 4건 중 3건 SL (-0.87%, -3.27%, -1.70%) → 1H 단독 역추세 진입 패턴
- 스코어 임계값(12.0) 고정 → 웹에서 조정 불가

**권고 파라미터:**

| 항목 | 현재값 | 권고값 | 근거 |
|------|--------|--------|------|
| MTF 합의 최소 TF 수 | 1 (필터 없음) | 2 | 2개 TF 합의 시 역추세 포지션 약 60% 감소 예상 |
| 스코어 임계값 | 12.0 (고정) | 12.0 (웹 조정 가능) | Owner가 데이터 보며 직접 튜닝할 수 있도록 |
| 쿨다운 | 4시간 (고정) | 4시간 (웹 조정 가능) | 동일 |

---

## Feature A: MTF 합의 필터 (pipeline)

### 핵심 로직

```
신호 생성 후, 같은 방향(LONG or SHORT)의 신호가
서로 다른 타임프레임에서 N개 이상 존재할 때만 알림/페이퍼 진입 허용.

예: BTCUSDT 분석 결과
  1H SHORT (score 8.5)  ← 1개 TF
  4H SHORT (score 10.1) ← 2개 TF → MTFConsensusMin=2 이상이면 통과
  → 합의 달성 → 알림 발송

  1H SHORT만 있을 때 → 합의 미달 → 필터링
```

### 코드 변경

#### `internal/pipeline/pipeline.go`

`Config`에 필드 추가:
```go
MTFConsensusMin int // ≥2 시 방향 합의 필터 활성. 1=비활성(기존 동작). 기본값 2
```

`DefaultConfig()`에서 `MTFConsensusMin: 2` 설정.

`analyzeSymbol`에서 enrichSignalLevels 직후, paper trader 호출 전에 필터 적용:
```go
if p.cfg.MTFConsensusMin > 1 {
    signals = filterMTFConsensus(signals, p.cfg.MTFConsensusMin)
}
if len(signals) == 0 {
    p.log.Debug().Str("symbol", sym).Msg("MTF 합의 미달 — 신호 필터링")
    return
}
```

새 함수 추가:
```go
// filterMTFConsensus returns only signals whose direction has signals
// from at least minTFs distinct timeframes. NEUTRAL signals are always kept.
func filterMTFConsensus(signals []models.Signal, minTFs int) []models.Signal {
    dirTFs := make(map[string]map[string]struct{})
    for _, sig := range signals {
        if sig.Direction == "NEUTRAL" {
            continue
        }
        if dirTFs[sig.Direction] == nil {
            dirTFs[sig.Direction] = make(map[string]struct{})
        }
        dirTFs[sig.Direction][sig.Timeframe] = struct{}{}
    }
    out := signals[:0]
    for _, sig := range signals {
        if sig.Direction == "NEUTRAL" || len(dirTFs[sig.Direction]) >= minTFs {
            out = append(out, sig)
        }
    }
    return out
}
```

**동적 업데이트 지원**: pipeline에 `SetAlertConfigHolder(*config.AlertConfigHolder)` 추가.
`analyzeSymbol` 시작 시 holder가 있으면 `cfg := holder.Get()` 로 읽어 `MTFConsensusMin` 적용.

---

## Feature B: 알림 설정 동적 업데이트 + 웹 UI

### config/alert.yaml (신규)

```yaml
score_threshold: 12.0
cooldown_hours: 4
mtf_consensus_min: 2
```

### internal/config/config.go 변경

```go
type AlertConfig struct {
    ScoreThreshold  float64 `yaml:"score_threshold"`
    CooldownHours   int     `yaml:"cooldown_hours"`
    MTFConsensusMin int     `yaml:"mtf_consensus_min"`
}

// AlertConfigHolder is a mutex-protected holder for live-updated AlertConfig.
type AlertConfigHolder struct {
    mu  sync.RWMutex
    cfg AlertConfig
}

func NewAlertConfigHolder(cfg AlertConfig) *AlertConfigHolder
func (h *AlertConfigHolder) Get() AlertConfig
func (h *AlertConfigHolder) Set(cfg AlertConfig)
```

`Config` 구조체에 `Alert AlertConfig` 추가.
`Load()`에서 `config/alert.yaml` 로드 (없으면 기본값).

### internal/notifier/notifier.go 변경

`Notifier`에 `alertHolder *config.AlertConfigHolder` 필드 추가.
`SetAlertConfigHolder(h *config.AlertConfigHolder)` 메서드 추가.

`Notify()` 내에서 holder가 설정되어 있으면:
```go
threshold := n.cfg.ScoreThreshold
if n.alertHolder != nil {
    ac := n.alertHolder.Get()
    threshold = ac.ScoreThreshold
    // cooldown 변경은 재시작 필요 (복잡도 회피)
}
```

### internal/pipeline/pipeline.go 변경

`Pipeline`에 `alertHolder *config.AlertConfigHolder` 추가.
`SetAlertConfigHolder(h)` 메서드 추가.

`analyzeSymbol` 시작 시:
```go
mtfMin := p.cfg.MTFConsensusMin
if p.alertHolder != nil {
    mtfMin = p.alertHolder.Get().MTFConsensusMin
}
```

### internal/api/server.go 변경

`Server`에 `alertHolder *config.AlertConfigHolder` 추가.
`WithAlertConfigHolder(h *config.AlertConfigHolder)` 추가.

라우트 등록:
```go
mux.HandleFunc("GET /api/alert/config", s.getAlertConfig)
mux.HandleFunc("PUT /api/alert/config", s.updateAlertConfig)
```

`getAlertConfig`: holder.Get() → JSON 반환.
`updateAlertConfig`: JSON 디코드 → 검증(score_threshold > 0, cooldown_hours > 0, mtf_consensus_min ≥ 1) → holder.Set() → alert.yaml 저장.

alert.yaml 읽기/쓰기 헬퍼:
```go
func (s *Server) alertCfgFile() string { return s.configDir + "/alert.yaml" }
func (s *Server) readAlertConfigLocked() (config.AlertConfig, error)
func (s *Server) writeAlertConfigLocked(cfg config.AlertConfig) error
```

### cmd/server/main.go 변경

```go
alertHolder := appconfig.NewAlertConfigHolder(cfg.Alert)
notif.SetAlertConfigHolder(alertHolder)
pipe.SetAlertConfigHolder(alertHolder)
apiSrv.WithAlertConfigHolder(alertHolder)
```

### web/src/App.tsx 변경

Tab 타입에 `'alert'` 추가.

`AlertConfig` 인터페이스:
```ts
interface AlertConfig {
  score_threshold: number
  cooldown_hours: number
  mtf_consensus_min: number
}
```

`AlertTab` 컴포넌트:
- 마운트 시 `GET /api/alert/config` 로드
- 3개 필드:
  1. **신호 스코어 임계값** — number input (step: 0.5, min: 5, 레이블: "최소 스코어 (현재 알림 발송 기준)")
  2. **쿨다운 (시간)** — number input (step: 1, min: 1, 레이블: "중복 방지 쿨다운 (시간)")
  3. **MTF 합의 최소 TF 수** — select (1=비활성, 2=2개 이상, 3=3개 이상, 4=전체, 레이블: "MTF 합의 필터")
- 저장 버튼 (`run-btn`) — PUT /api/alert/config
- 저장 성공 플래시 (`save-success` 클래스, 2초)
- 섹션 설명:
  - MTF 합의: "같은 방향 신호가 N개 타임프레임에서 동시 발생할 때만 알림 발송"

탭 네비게이션에 '알림' 버튼 추가.

---

## 완료 정의

- [ ] `go test ./...` 전체 PASS
- [ ] MTFConsensusMin=2 기본값 확인 (단일 TF 신호 필터링 테스트)
- [ ] GET /api/alert/config 응답 확인
- [ ] PUT /api/alert/config → alert.yaml 갱신 확인
- [ ] 웹 UI '알림' 탭 저장 후 즉시 적용 (다음 파이프라인 틱부터)

---

*승인: Orchestrator (Quant 분석 기반) — 2026-03-13*
