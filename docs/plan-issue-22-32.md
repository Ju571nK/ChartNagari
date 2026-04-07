# 기획서: Issue #22 + #32 구현 계획

## 개요

두 이슈 모두 "지금 구현 불가"로 분류되었으나, 인프라를 먼저 구축하고 데이터가 쌓이면 자동으로 활성화되는 접근이 가능하다.

---

## Issue #22: 실현 vs 내재 변동성 비교

### 목적
실현 변동성(ATR 기반)과 내재 변동성(옵션 시장)을 비교하여 "coiled" 시장(급격한 레짐 전환 임박)을 감지.

### 핵심 인사이트
- 실현 vol << 내재 vol → 시장이 눌려있고 곧 터진다
- 이 상태에서 시그널이 발생하면 신뢰도가 높다 (큰 움직임 예상)

### 구현 전략: VIX를 프록시로 사용

개별 종목의 implied volatility는 옵션 체인 데이터가 필요하지만, **시장 전체의 내재 변동성인 VIX**는 Yahoo Finance에서 `^VIX` 심볼로 무료 수집 가능.

#### Phase 1: VIX 데이터 수집 (인프라)

**watchlist.yaml 확장:**
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
- `EnabledIndexSymbols()` 메서드 추가 (EnabledCryptoSymbols, EnabledStockSymbols 패턴)

**cmd/server/main.go:**
- indices 섹션의 심볼을 Yahoo 수집기에 추가
- VIX는 시그널 분석 대상이 아님 → 파이프라인에서 제외 (수집만)

**주의사항:**
- Yahoo Finance에서 `^VIX`는 URL 인코딩 필요 (`%5EVIX`)
- Yahoo 수집기의 `strings.ToUpper()` 처리 확인 필요
- `^VIX`는 1H 데이터 불가 → 1D만 수집

#### Phase 2: Realized vs VIX 비교 로직

**internal/indicator/indicator.go:**
```
{TF}:REALIZED_VOL_20 = 20일 실현 변동성 (close-to-close 표준편차 × √252)
```
이미 ATR이 있지만, 전통적 realized vol은 log return의 표준편차.

**internal/pipeline/regime.go에 추가:**
```go
func isMarketCoiled(indicators map[string]float64, vixBars []models.OHLCV) bool {
    realizedVol := indicators["1D:REALIZED_VOL_20"]
    currentVIX := vixBars[0].Close  // 최신 VIX 값
    
    // VIX는 연율화된 % (예: 18.5 = 18.5%)
    // Realized vol도 연율화
    // Coiled = realized < VIX * 0.7 (30% 이상 괴리)
    return realizedVol < currentVIX * 0.7
}
```

**파이프라인 통합:**
- Coiled 상태 감지 시 모든 시그널에 +10% 보너스
- Signal Tuning UI에 "Coiled market bonus" 슬라이더 추가
- VIX 데이터 없으면 자동 skip (기존 동작 유지)

#### Phase 3: 프론트엔드 (선택)

- Status 탭에 VIX 현재 값 + Coiled 상태 표시
- Chart 탭에 VIX 오버레이 옵션 (별도 pane)

### 의존성
- Yahoo Finance에서 ^VIX 수집 가능 확인 필요 (테스트)
- watchlist.yaml에 indices 섹션 추가
- 크립토에는 VIX 대안 필요 (BVOL 또는 skip)

### Effort 추정
- Phase 1 (VIX 수집): S — yaml 파싱 + Yahoo 수집기에 심볼 추가
- Phase 2 (비교 로직): S — indicator + pipeline 함수 추가
- Phase 3 (프론트): M — Status 탭 + Chart 오버레이
- 전체: M

### 리스크
- Yahoo Finance ^VIX API 안정성 (rate limit, 데이터 지연)
- VIX는 S&P 500 기반이므로 개별 주식의 IV와 다를 수 있음
- 크립토에는 VIX 대안이 없음 (Bitcoin Volatility Index가 있지만 무료 API 부재)

---

## Issue #32: Auto-calibrate HTF Penalty

### 목적
백테스트 결과에서 각 ATR 퍼센타일 버킷의 counter-trend 시그널 성과를 분석하여 최적 패널티를 자동 설정.

### 핵심 인사이트
- 현재 gradient 공식 (`base * (1 - scaling * atr_pct)`)의 파라미터가 수동 설정
- 충분한 데이터가 있으면 각 버킷의 실제 성과에서 최적값을 역산 가능

### 전제 조건
- 버킷당 최소 50개 counter-trend 시그널 (통계적 유의성)
- 10개 종목 × 하루 2개 시그널 × 365일 = ~7,300 시그널/년
- Counter-trend 비율 ~30% → 연 ~2,200개
- 10 ATR 분위수(decile) → 버킷당 ~220개/년
- **최소 1년, 권장 2년** 데이터 필요

### 구현 전략

#### Phase 1: 데이터 충분성 체크 (인프라)

**internal/backtest/calibrate.go (신규):**
```go
type CalibrationResult struct {
    Bucket          int     // ATR percentile bucket (0-9 for deciles)
    CounterTrendN   int     // number of counter-trend signals in this bucket
    AvgReturn       float64 // average return of counter-trend signals
    OptimalPenalty  int     // 0-100, penalty that maximizes total return
    Confidence      string  // "insufficient" | "low" | "medium" | "high"
}

func CalibrateHTFPenalty(db storage.DB) ([]CalibrationResult, error) {
    // 1. 모든 시그널 조회 (created_at, direction, score, forward_return_5d)
    // 2. 각 시그널의 entry 시점 ATR percentile 계산
    // 3. Counter-trend 시그널만 필터 (HTF 방향과 반대)
    // 4. Decile 버킷별 그룹화
    // 5. 각 버킷의 forward return 분석
    // 6. OptimalPenalty 계산: return > 0이면 penalty 낮춤, return < 0이면 높임
}
```

**Confidence 기준:**
- N < 30 → "insufficient" (캘리브레이션 불가)
- 30 ≤ N < 50 → "low"
- 50 ≤ N < 100 → "medium"
- N ≥ 100 → "high"

#### Phase 2: API + UI

**API:**
- `GET /api/calibrate/htf-penalty` → CalibrationResult 배열 반환
- `POST /api/calibrate/htf-penalty/apply` → 결과를 signal_tuning.yaml에 적용

**Settings UI:**
```
Auto-Calibration
────────────────────────────────
[Run Calibration]

Bucket 0-10%:  N=12  insufficient data
Bucket 10-20%: N=8   insufficient data
Bucket 20-30%: N=45  low confidence → penalty 65%
...
Bucket 90-100%: N=38  low confidence → penalty 18%

[Apply Recommended] [Keep Current]
```

- 데이터 부족 버킷은 회색으로 표시
- "high" confidence 버킷만 자동 적용 권장
- "Apply Recommended" → confidence가 medium 이상인 버킷만 적용

#### Phase 3: 자동 실행 (장기)

- 매주 일요일 자동 캘리브레이션 실행
- 결과가 이전과 크게 다르면 Telegram 알림
- 사용자 승인 후 적용

### 의존성
- #23 Forward Return Tracking이 이미 구현됨 (forward_return_5d 필요)
- 충분한 시그널 히스토리 (최소 1년)
- HTF 방향 정보가 시그널에 저장되어 있어야 함 → 현재 미저장, 추가 필요

### 새로운 DB 필드 필요
signals 테이블에 추가:
```sql
ALTER TABLE signals ADD COLUMN htf_trend TEXT DEFAULT '';  -- "LONG", "SHORT", ""
ALTER TABLE signals ADD COLUMN atr_percentile REAL DEFAULT -1;
```
파이프라인에서 시그널 저장 시 같이 기록.

### Effort 추정
- Phase 1 (캘리브레이션 로직): M
- Phase 2 (API + UI): M
- Phase 3 (자동 실행): S
- DB 필드 추가: XS
- 전체: L

### 리스크
- 샘플 사이즈 부족으로 과적합 (overfitting) 위험
- 시장 레짐 자체가 변하면 과거 캘리브레이션이 미래에 유효하지 않을 수 있음
- Counter-trend 정의가 현재 HTF filter에 의존 → HTF filter 변경 시 재캘리브레이션 필요

---

## 구현 순서 권장

```
#22 Phase 1 (VIX 수집) ──→ #22 Phase 2 (비교 로직) ──→ #22 Phase 3 (UI)
                                                              │
#32 DB 필드 추가 ──→ (데이터 축적 대기) ──→ #32 Phase 1 ──→ #32 Phase 2
```

**즉시 구현 가능:** #22 Phase 1+2, #32 DB 필드 추가
**데이터 축적 후:** #32 Phase 1+2+3
**선택:** #22 Phase 3 (프론트엔드)

## 타임라인

| 시점 | 작업 |
|------|------|
| 지금 | #22 Phase 1+2 (VIX 수집 + 비교 로직) |
| 지금 | #32 DB 필드 추가 (htf_trend, atr_percentile) |
| 1개월 후 | #22 Phase 3 (UI) — VIX 데이터 확인 후 |
| 6개월 후 | #32 Phase 1 (캘리브레이션 로직) — 시그널 히스토리 축적 |
| 1년 후 | #32 Phase 2+3 (UI + 자동 실행) — 통계적 유의성 확보 |
