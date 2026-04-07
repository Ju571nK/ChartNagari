# 기획서: Issue #22 + #32 구현 계획

## 개요

두 이슈 모두 "인프라 먼저" 접근으로 구현. 데이터가 쌓이면 자동 활성화.

---

## Issue #22: 실현 vs 내재 변동성 비교

### 목적
실현 변동성(ATR 기반)과 내재 변동성(VIX)을 비교하여 "coiled" 시장(급격한 레짐 전환 임박)을 감지.

### 핵심 인사이트
- 실현 vol << 내재 vol → 시장이 눌려있고 곧 터진다
- 이 상태에서 시그널이 발생하면 신뢰도가 높다 (큰 움직임 예상)
- VIX (CBOE Volatility Index)를 시장 전체 내재 변동성 프록시로 사용

---

### Phase 1: VIX 데이터 수집 + 기본 UI (Backend + Frontend)

**Effort:** S | **시기:** 즉시

#### Backend

**config/watchlist.yaml 확장:**
```yaml
symbols:
    crypto:
        - ...
    stocks:
        - ...
    indices:              # 새로운 섹션
        - symbol: "^VIX"
          exchange: cboe
          enabled: true
```

**internal/config/config.go:**
- `EnabledIndexSymbols() []string` 메서드 추가
- watchlist 파싱에 `indices` 섹션 추가

**cmd/server/main.go:**
- indices 심볼을 Yahoo 수집기에 추가 (crypto/stock과 동일 패턴)
- indices는 파이프라인 분석 대상에서 **제외** (수집만)

**internal/collector/yahoo.go:**
- `^VIX` → URL 인코딩 처리 확인 (`%5EVIX`)
- 1D 타임프레임만 수집 (VIX는 인트라데이 의미 없음)

**API 엔드포인트:**
- `GET /api/vix/current` → 최신 VIX 값 + 20일 평균 + 트렌드 방향 반환

#### Frontend

**Status 탭에 VIX 위젯 추가:**
```
Market Volatility
─────────────────────────
VIX:          18.5  ↓
20d Average:  21.3
Status:       Normal

[VIX 수집이 비활성이면 이 섹션 숨김]
```

- VIX < 15: `--safe` "Low" 
- VIX 15-25: `--text` "Normal"
- VIX 25-35: `--warning` "Elevated"
- VIX > 35: `--danger` "High"

**Settings 탭:**
- "Market Indices" 섹션에 VIX 수집 토글 (watchlist.yaml의 indices.enabled 연동)

**i18n (en/ko/ja):**
- `market_volatility`, `vix_current`, `vix_average`, `vix_low`, `vix_normal`, `vix_elevated`, `vix_high`

---

### Phase 2: Coiled 감지 + 시그널 보너스 + UI (Backend + Frontend)

**Effort:** S | **시기:** Phase 1 완료 후

#### Backend

**internal/indicator/realized_vol.go (신규):**
```go
// realizedVol computes annualized realized volatility from close prices.
// Uses log returns over N periods, annualized by √252.
func realizedVol(closes []float64, period int) (float64, bool)
```

**internal/indicator/indicator.go:**
- `{TF}:REALIZED_VOL_20` 인디케이터 추가

**internal/pipeline/regime.go에 추가:**
```go
// CoiledState detects when realized vol is significantly below implied vol (VIX).
type CoiledState struct {
    IsCoiled    bool
    RealizedVol float64
    ImpliedVol  float64  // VIX value
    Ratio       float64  // realized / implied (< 0.7 = coiled)
}

func detectCoiledMarket(indicators map[string]float64, db OHLCVReader) CoiledState
```

**파이프라인 통합:**
- Coiled 상태 감지 시 모든 시그널에 보너스 적용
- 보너스 비율은 signal_tuning.yaml에서 설정

**config/signal_tuning.yaml 확장:**
```yaml
coiled_market:
    enabled: true
    ratio_threshold: 0.7    # realized/implied < 0.7 → coiled
    bonus_pct: 10           # coiled 시 시그널 보너스 %
```

**internal/config/signal_tuning.go:**
```go
type CoiledMarketConfig struct {
    Enabled        bool `yaml:"enabled" json:"enabled"`
    RatioThreshold int  `yaml:"ratio_threshold" json:"ratio_threshold"` // 0-100, represents 0.0-1.0
    BonusPct       int  `yaml:"bonus_pct" json:"bonus_pct"`
}
```

**API 확장:**
- `GET /api/vix/current` → CoiledState도 포함

#### Frontend

**Status 탭 VIX 위젯 확장:**
```
Market Volatility
─────────────────────────
VIX (Implied):    18.5  ↓
Realized (20d):   12.3
Ratio:            0.66  ⚡ COILED
Status:           Coiled — breakout likely

[Coiled 상태면 --warning 배경 + 번개 아이콘]
[Normal이면 기본 배경]
```

**Signal Tuning 섹션에 추가:**
```
Coiled Market
─────────────────────────
☑ Enable coiled detection
Ratio threshold    [====|======] 0.70
Coiled bonus       [====|======] 10%
```

**Chart 탭 (선택):**
- VIX를 별도 pane에 라인 차트로 표시 (오버레이 토글 "VIX")

**i18n 추가:**
- `coiled_market`, `coiled_detected`, `ratio_threshold`, `coiled_bonus`, `realized_vol`, `implied_vol`, `breakout_likely`

---

### Phase 3: Chart VIX 오버레이 (Frontend)

**Effort:** S | **시기:** Phase 2 완료 후 (선택)

#### Frontend

**Chart 탭 오버레이 토글에 "VIX" 버튼 추가:**
- 기존 FVG/OB/Zones 토글 옆에 배치
- 별도 pane (차트 하단, 볼륨 히스토그램 옆)에 VIX 라인 차트
- lightweight-charts의 별도 price scale에 렌더링
- 색상: `--slate` (#94a3b8)
- Coiled 구간 하이라이트 (배경색 변경)

**API:**
- `GET /api/ohlcv/^VIX/1D?limit=200` — 기존 OHLCV 엔드포인트 재사용

**i18n:**
- `overlay_vix`: "VIX" / "VIX" / "VIX"

---

### #22 리스크
- Yahoo Finance ^VIX API 안정성 (rate limit, 데이터 지연)
- VIX는 S&P 500 기반이므로 개별 주식의 IV와 다를 수 있음
- 크립토에는 VIX 대안이 없음 (BVOL은 무료 API 부재) → 크립토는 coiled 감지 skip

---
---

## Issue #32: Auto-calibrate HTF Penalty

### 목적
백테스트 결과에서 각 ATR 퍼센타일 버킷의 counter-trend 시그널 성과를 분석하여 최적 패널티를 자동 설정.

### 전제 조건
- 버킷당 최소 50개 counter-trend 시그널 (통계적 유의성)
- **최소 1년, 권장 2년** 시그널 히스토리
- Forward return tracking (#23) 구현 완료 ✅
- HTF 방향 + ATR 퍼센타일이 시그널에 저장되어야 함 → Phase 1에서 추가

---

### Phase 1: DB 인프라 + 데이터 기록 (Backend)

**Effort:** XS | **시기:** 즉시

#### Backend

**internal/storage/db.go — signals 테이블 마이그레이션:**
```sql
ALTER TABLE signals ADD COLUMN htf_trend TEXT DEFAULT '';
ALTER TABLE signals ADD COLUMN atr_percentile REAL DEFAULT -1;
```

**internal/storage/signals.go:**
- SaveSignal에 htf_trend, atr_percentile 컬럼 포함
- GetSignals 등 조회 메서드에도 반영

**pkg/models/signal.go:**
```go
HTFTrend      string  `json:"htf_trend,omitempty"`      // "LONG", "SHORT", ""
ATRPercentile float64 `json:"atr_percentile,omitempty"` // 0-100, -1 if unknown
```

**internal/pipeline/pipeline.go:**
- 시그널 저장 시 현재 HTF 트렌드 방향과 ATR 퍼센타일을 Signal에 기록
- 이미 `wyckoffPhase`와 `htfPenaltyPct` 계산 시점에서 값을 알고 있으므로 채우기만 하면 됨

#### Frontend

없음 (데이터 기록만, 사용자에게 보이지 않음)

---

### Phase 2: 캘리브레이션 로직 + API (Backend)

**Effort:** M | **시기:** 6개월 후 (시그널 히스토리 축적)

#### Backend

**internal/backtest/calibrate.go (신규):**
```go
type CalibrationResult struct {
    Bucket          int     `json:"bucket"`           // ATR percentile decile (0-9)
    BucketRange     string  `json:"bucket_range"`     // "0-10%", "10-20%", ...
    CounterTrendN   int     `json:"counter_trend_n"`  // 시그널 수
    AvgReturn5d     float64 `json:"avg_return_5d"`    // 평균 5일 수익률
    AvgReturn20d    float64 `json:"avg_return_20d"`   // 평균 20일 수익률
    CurrentPenalty  int     `json:"current_penalty"`  // 현재 gradient 공식의 값
    OptimalPenalty  int     `json:"optimal_penalty"`  // 추천 값
    Confidence      string  `json:"confidence"`       // "insufficient" | "low" | "medium" | "high"
}

func CalibrateHTFPenalty(db CalibrationDB) ([]CalibrationResult, error)
```

**Confidence 기준:**
- N < 30 → "insufficient" (캘리브레이션 불가)
- 30 ≤ N < 50 → "low"
- 50 ≤ N < 100 → "medium"
- N ≥ 100 → "high"

**OptimalPenalty 계산 로직:**
```
avg_return_5d > 0  → penalty를 낮춤 (counter-trend이 수익성 있음)
avg_return_5d < 0  → penalty를 높임 (counter-trend이 손실)
avg_return_5d ≈ 0  → 현재 gradient 값 유지
```

**API 엔드포인트:**
- `GET /api/calibrate/htf-penalty` → CalibrationResult[] 반환
- `POST /api/calibrate/htf-penalty/apply` → 추천값을 signal_tuning.yaml에 적용

---

### Phase 3: 캘리브레이션 UI (Frontend)

**Effort:** M | **시기:** Phase 2와 동시

#### Frontend

**Settings 탭 Signal Tuning 섹션에 추가:**
```
Auto-Calibration
────────────────────────────────────────────────────────
[Run Calibration]

Bucket    Signals  5d Return  Current  Recommended  Confidence
0-10%        12       —         50%        —         insufficient
10-20%        8       —         47%        —         insufficient
20-30%       45     -1.2%       44%       55%        low
30-40%       62     -0.8%       41%       48%        medium
40-50%       78     +0.3%       38%       32%        medium
50-60%       85     +0.5%       35%       28%        medium
60-70%       91     +0.8%       32%       22%        medium
70-80%       67     +1.4%       29%       15%        medium
80-90%       38     +2.1%       26%        8%        low
90-100%      22     +1.8%       23%        5%        insufficient

[Apply Recommended]  [Keep Current]
────────────────────────────────────────────────────────
```

**디자인:**
- 테이블: 기존 backtest-table 스타일 재사용
- Confidence 색상: insufficient = `--muted` 0.4, low = `--warning`, medium = `--safe`, high = `--mint`
- "Apply Recommended" 버튼: medium/high confidence 버킷만 적용
- insufficient 행은 회색 + 이탤릭
- "Run Calibration" 클릭 시 로딩 스피너
- 데이터 부족 시 안내 메시지: "Need at least 6 months of signal history for calibration. Current: N signals (X months)."

**Settings 탭 VIX 관련 UI (from #22)와 나란히 배치:**
```
Signal Tuning
├── HTF Counter-Trend Penalty (기존)
│   ├── Gradient mode toggle + sliders
│   └── Per-regime sliders (legacy mode)
├── Auto-Calibration (신규, #32)
│   ├── Run Calibration button
│   ├── Results table
│   └── Apply/Keep buttons
├── Coiled Market (신규, #22)
│   ├── Enable toggle
│   ├── Ratio threshold slider
│   └── Bonus % slider
├── Volatility Regime (기존)
└── ATR Slope (기존)
```

**i18n (en/ko/ja):**
- `auto_calibration`: "Auto-Calibration" / "자동 캘리브레이션" / "自動キャリブレーション"
- `run_calibration`: "Run Calibration" / "캘리브레이션 실행" / "キャリブレーション実行"
- `apply_recommended`: "Apply Recommended" / "추천값 적용" / "推奨値を適用"
- `keep_current`: "Keep Current" / "현재값 유지" / "現在値を維持"
- `insufficient_data`: "Insufficient data" / "데이터 부족" / "データ不足"
- `calibration_hint`: "Need at least 6 months of signal history" / "최소 6개월의 시그널 히스토리 필요" / "最低6ヶ月のシグナル履歴が必要"
- `confidence`: "Confidence" / "신뢰도" / "信頼度"
- `recommended`: "Recommended" / "추천" / "推奨"

---

### Phase 4: 자동 실행 (Backend, 장기)

**Effort:** S | **시기:** 1년 후

#### Backend

**internal/pipeline/pipeline.go 또는 신규 scheduler:**
- 매주 일요일 UTC 00:00에 자동 캘리브레이션 실행
- 결과가 이전 대비 10% 이상 변동 시 Telegram/Discord 알림
- 자동 적용은 하지 않음 → 알림만, 사용자가 Settings UI에서 확인 후 적용

**config/signal_tuning.yaml:**
```yaml
auto_calibration:
    enabled: false          # 사용자가 Settings에서 활성화
    schedule: "weekly"      # weekly | monthly
    auto_apply: false       # true면 자동 적용, false면 알림만
    min_confidence: "medium" # 자동 적용 시 최소 confidence
```

#### Frontend

**Settings 탭 Auto-Calibration 섹션에 추가:**
```
☑ Auto-run weekly
☐ Auto-apply (medium+ confidence only)
Last run: 2026-10-05 — 8/10 buckets calibrated
```

---

## 전체 구현 순서

```
즉시:
  #22 Phase 1 (VIX 수집 + Status UI)
  #32 Phase 1 (DB 필드 추가)
       ↓
1주 후:
  #22 Phase 2 (Coiled 감지 + Signal Tuning UI)
       ↓
2주 후:
  #22 Phase 3 (Chart VIX 오버레이) [선택]
       ↓
6개월 후 (시그널 축적):
  #32 Phase 2 (캘리브레이션 로직 + API)
  #32 Phase 3 (캘리브레이션 UI)
       ↓
1년 후:
  #32 Phase 4 (자동 실행 + 알림)
```

## Effort 총정리

| Phase | Issue | Backend | Frontend | Effort | 시기 |
|-------|-------|---------|----------|--------|------|
| #22-P1 | VIX 수집 + Status UI | watchlist 확장, Yahoo 수집, API | Status 탭 VIX 위젯, Settings 토글 | S | 즉시 |
| #22-P2 | Coiled 감지 + Tuning UI | realized_vol, coiled 로직, pipeline | Signal Tuning coiled 섹션 | S | 1주 후 |
| #22-P3 | Chart VIX 오버레이 | 없음 (기존 API 재사용) | Chart 탭 VIX 라인 pane | S | 2주 후 |
| #32-P1 | DB 인프라 | 마이그레이션 + pipeline 기록 | 없음 | XS | 즉시 |
| #32-P2 | 캘리브레이션 로직 | calibrate.go + API | 없음 | M | 6개월 |
| #32-P3 | 캘리브레이션 UI | 없음 | Settings 탭 테이블 + 버튼 | M | 6개월 |
| #32-P4 | 자동 실행 | scheduler + 알림 | Settings 토글 | S | 1년 |

## 리스크

| 리스크 | 영향 | 대응 |
|--------|------|------|
| Yahoo ^VIX API 불안정 | VIX 수집 실패 | graceful skip, Status 탭에 "VIX unavailable" 표시 |
| VIX ≠ 개별 종목 IV | Coiled 판단 부정확 | "시장 전체" 프록시로 제한, 개별 IV는 장기 과제 |
| 크립토에 VIX 없음 | 크립토 coiled 미감지 | 크립토는 coiled skip, 향후 BVOL 연동 |
| 캘리브레이션 과적합 | 미래에 유효하지 않은 패널티 | confidence 게이트 + 최소 샘플 요구 |
| 시장 레짐 변화 | 과거 캘리브레이션 무효화 | rolling window (최근 1년만) 사용 |
