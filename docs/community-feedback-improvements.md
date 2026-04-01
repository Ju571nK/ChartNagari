# Community Feedback — Signal Detection Improvements

Reddit/커뮤니티 댓글에서 추출한 구현 개선 포인트 정리.

> 원문 요약: "FVG와 sweep은 자동화 가능하지만, OB와 Wyckoff는 주관적이라 어렵다.
> 정량화 가능한 부분(FVG, sweep)을 자동화하고 필터로 활용. Volume profile과 결합하면 단독보다 유용."

## 구현 항목

### 1. Sweep Quality Score (스윕 품질 점수) — P1

**현재:** `rawScore = 1.0` 고정. 모든 스윕이 동일 점수.
**개선:** 스윕의 품질을 정량화하여 진짜 스탑 헌팅 vs 진성 돌파를 구분.

**평가 요소:**

- **볼륨 비율:** 스윕 캔들의 거래량 / 20MA 거래량. 높을수록 기관 활동 가능성 높음.
- **반전 속도:** 스윕 후 몇 캔들 안에 반전했는지. 빠를수록 진짜 스윕.
- **윅 비율:** 스윕 캔들의 윅 길이 / 전체 캔들 길이. 윅이 길수록 강한 거부.

**구현 파일:** `internal/methodology/ict/liquidity_sweep.go`
**상태:** 구현 진행

---

### 2. FVG Relevance Filter (FVG 유효성 필터) — P2

**현재:** 모든 FVG 패턴 동일 `rawScore = 1.0`. 1분봉에서 수십 개 발생, 대부분 즉시 채워짐.
**개선:** FVG 크기, 형성 속도, 시간 경과에 따른 유효성을 점수화.

**평가 요소:**

- **갭 크기 vs ATR:** 갭이 ATR 대비 클수록 의미 있음. 너무 작은 갭 필터링.
- **형성 임펄스 강도:** 갭을 만든 캔들(bars[i+1])의 body 크기와 볼륨.
- **미채워진 시간:** 오래 미채워진 FVG일수록 강한 레벨.

**구현 파일:** `internal/methodology/ict/fair_value_gap.go`
**상태:** ✅ 완료 — `fvgRelevance()` 함수 구현. gap size vs ATR(35%), impulse strength(35%), unfilled duration(30%).

---

### 3. Volume Profile Integration (볼륨 프로파일 통합) — P2

**현재:** VOLUME_MA_20 단일 지표만 사용. 볼륨 프로파일(가격대별 거래량 분포) 없음.
**개선:** 스윕/FVG 레벨과 고/저 볼륨 노드 일치 여부를 시그널 보강 필터로 활용.

**평가 요소:**

- **HVN (High Volume Node):** 스윕 레벨이 HVN 근처면 강한 지지/저항 → 스윕 신뢰도 상승
- **LVN (Low Volume Node):** FVG가 LVN 안에 있으면 빠른 가격 이동 영역 → FVG 유효성 상승

**구현 파일:** `internal/indicator/volume_profile.go` + `indicator.go`의 `Compute()`에 통합
**상태:** ✅ 완료 — `volumeProfile()` 함수 구현. 20-bin 분포 계산, `VP_POC`, `VP_HVN_1~3`, `VP_LVN_1~3` 인디케이터로 제공. 테스트 포함.

---

### 4. Sweep vs Breakout Classifier (스윕/돌파 분류기) — P3

**현재:** 스윕 레벨을 돌파한 후 되돌아오면 스윕, 안 오면 미감지.
**개선:** 돌파 후 N캔들 확인 기간을 두고, 되돌아오지 않으면 "진성 돌파"로 분류하여 스윕 시그널 취소.

**평가 요소:**

- **확인 기간:** 3~5캔들 뒤에도 레벨 위/아래 유지 시 돌파로 재분류
- **후속 캔들 body:** 돌파 방향으로 연속 양봉/음봉 → 진성 돌파 확률 높음

**구현 파일:** `internal/methodology/ict/liquidity_sweep.go` 확장
**상태:** 미착수

---

### 5. Order Block Definition Standardization (오더블록 정의 표준화) — P3

**현재:** "임펄스 이전 마지막 역방향 캔들" 단일 정의. 실무자마다 다른 캔들을 선택.
**개선:** 하나의 명확한 정의를 채택하되, mitigation 여부 추적 추가.

**평가 요소:**

- **Mitigation 추적:** OB 영역에 가격이 재진입하면 "mitigated" 처리 → 재사용 금지
- **임펄스 강도 필터:** OB를 만든 임펄스의 크기가 ATR의 N배 이상일 때만 유효

**구현 파일:** `internal/methodology/ict/order_block.go`
**상태:** ✅ 완료 — `isMitigated()` 함수로 mitigation 추적 + impulse strength filter (combined body ≥ 1.5x ATR).

---

## 시그널 구조 개선 항목

두 번째 커뮤니티 피드백에서 추출. 단일 패턴 매칭이 아닌 구조적 접근법.

> 원문 요약: "정의의 일관성이 가장 큰 벽. MTF 정렬은 top-down context first 규칙이 효과적.
> 단일 시그널보다 시퀀스(sweep + displacement + retest) 추적이 유용.
> ICT/Wyckoff를 완전 객관화하기보다 자신의 edge를 형식화하는 도구로 접근."

### 9. Top-Down HTF Context Filter (상위 TF 구조 필터) — P1

**현재:** MTF 컨센서스 = "같은 방향 시그널이 N개 TF에서 발생했는가" (방향 카운트).
상위 TF의 구조(추세/횡보)를 고려하지 않음. 1H 스윕이 1D 하락 추세에 역행해도 통과.

**개선:** 하위 TF 시그널을 상위 TF의 구조 컨텍스트로 필터링.
- 1W/1D가 명확한 상승 추세면 → 1H/4H의 LONG 시그널만 통과
- 1W/1D가 명확한 하락 추세면 → 1H/4H의 SHORT 시그널만 통과
- 1W/1D가 횡보면 → 양방향 모두 통과 (레인지 트레이딩)

**구조 판별 기준:**
- EMA_50 vs EMA_200 관계 (골든크로스/데드크로스)
- 현재가 vs EMA_50 위치 (위=상승 컨텍스트, 아래=하락)
- Wyckoff phase (accumulation/distribution이면 횡보로 분류)

**구현 파일:** `internal/pipeline/pipeline.go` — `filterMTFConsensus` 확장 또는 별도 필터 함수
**의존:** `ctx.Indicators`에 `1D:EMA_50`, `1D:EMA_200` 이미 존재 확인 필요
**상태:** 미착수

---

### 10. Signal Sequence Tracking (시그널 시퀀스 추적) — P2

**현재:** 각 룰이 독립적으로 단일 패턴만 감지. "sweep 발생" 단독 이벤트로 끝남.
**개선:** 복합 시퀀스를 추적하여 시그널 신뢰도를 보강.

**추적 시퀀스 예시:**
- **Sweep → Displacement → Retest:** sweep 발생 후, 강한 임펄스(displacement)가 이어지고,
  sweep 레벨로 재테스트가 오면 높은 확신 진입점. 세 단계 모두 완료 시 보너스 점수.
- **FVG 형성 → FVG 재진입:** FVG가 생기고 나중에 가격이 그 갭으로 돌아오면 진입 시그널.
- **OB 형성 → OB 리테스트:** 오더블록이 형성된 후 가격이 되돌아와 터치하면 진입.

**구현 접근:**
- `internal/pipeline/` 또는 신규 `internal/sequence/` 패키지
- 심볼별로 최근 N개 시그널 이력을 메모리에 유지 (ring buffer)
- 새 시그널 발생 시 이전 시그널과 시퀀스 매칭 → 매치되면 점수 보너스

**구현 파일:** 신규 `internal/sequence/tracker.go` + pipeline 연동
**상태:** 미착수

---

### 11. Wyckoff Phase를 컨텍스트 레이블로 사용 — P3

**현재:** Wyckoff accumulation/distribution/spring/upthrust가 독립 시그널로 발생.
**개선:** Wyckoff phase를 시그널이 아닌 "컨텍스트 레이블"로 활용.

**근거:** 댓글의 핵심 인사이트 — "ICT/Wyckoff를 완전 객관화하려 하지 말고, 자신의 edge를
형식화하는 도구로 접근하라." Wyckoff phase는 독립 시그널보다 다른 시그널의 필터/부스터로 가치 있음.

**적용 방식:**
- Wyckoff accumulation 감지 중이면 → 같은 심볼의 LONG 시그널 점수 +20% 부스트
- Wyckoff distribution 감지 중이면 → SHORT 시그널 점수 +20% 부스트
- 단독 시그널 발생은 유지하되, 엔진 레벨에서 컨텍스트 부스팅 추가

**구현 파일:** `internal/engine/engine.go` — Wyckoff phase 기반 시그널 부스팅 로직
**상태:** 미착수

---

## UI/UX 개선 항목

백엔드 품질 점수 개선이 사용자에게 체감되려면 프론트엔드에서 시각적 피드백이 필요하다.
기존 UI는 score 숫자만 표시하므로, 같은 룰에서 점수가 달라져도 사용자가 이유를 알 수 없다.

### 6. Signal Log 품질 시각화 — P2

**현재:** Signal Log 테이블의 score 컬럼이 숫자만 표시 (예: `4.5`).
**개선:** 점수 옆에 색상 도트 또는 미니 바를 추가하여 품질 레벨을 시각 구분.

**구현 범위:**
- `web/src/App.tsx` Signal Log 테이블 score 셀 (`s.score.toFixed(1)` 부분)
- 높음(≥0.7): `--safe` 색상 도트, 중간(0.4~0.7): `--warning`, 낮음(<0.4): `--danger`
- 기존 숫자 표시 유지 + 좌측에 8px 컬러 도트 추가

**상태:** 미착수

---

### 7. Chart 마커 품질 차별화 — P3

**현재:** 차트의 시그널 마커가 모두 동일 크기/투명도.
**개선:** 스윕 품질 점수에 따라 마커 투명도를 차별화. 고품질 시그널이 시각적으로 돋보임.

**구현 범위:**
- `web/src/App.tsx` ChartTab 시그널 마커 렌더링 부분
- 높은 score: opacity 1.0, 낮은 score: opacity 0.4~0.6
- 마커 크기는 동일 유지 (크기 차별화는 혼잡해질 수 있음)

**상태:** 미착수

---

### 8. 라이브 토스트 품질 레이블 — P3

**현재:** 실시간 시그널 토스트에 score 숫자만 표시.
**개선:** score 옆에 "HIGH" / "MED" / "LOW" 텍스트 레이블 + 색상 추가.

**구현 범위:**
- `web/src/App.tsx` 라이브 시그널 토스트 (`ws-toast-score` 클래스)
- HIGH(≥0.7): `--safe` + "HIGH", MED(0.4~0.7): `--warning` + "MED", LOW(<0.4): `--muted` + "LOW"

**상태:** 미착수

---

## 우선순위 정리

| #   | 항목                            | Priority | Effort | 영향도                     | 상태 |
| --- | ----------------------------- | -------- | ------ | ----------------------- | ---- |
| 1   | Sweep Quality Score           | P1       | S      | 높음 — 가장 직접적인 노이즈 감소     | ✅ 완료 |
| 9   | Top-Down HTF Context Filter   | P1       | M      | 높음 — 역추세 시그널 제거          | ✅ 완료 |
| 2   | FVG Relevance Filter          | P2       | S      | 중간 — FVG 정확도 향상         | ✅ 완료 |
| 3   | Volume Profile Integration    | P2       | M      | 중간 — 전체 시그널 보강           | ✅ 완료 |
| 10  | Signal Sequence Tracking      | P2       | L      | 높음 — 복합 패턴 진입점 신뢰도     | ✅ 완료 |
| 6   | Signal Log 품질 시각화         | P2       | XS     | 중간 — 품질 점수의 사용자 체감      | ✅ 완료 |
| 4   | Sweep vs Breakout Classifier  | P3       | S      | 중간 — 위양성 감소              | ✅ 완료 |
| 5   | OB Definition Standardization | P3       | M      | 낮음 — OB 자체가 주관적          | ✅ 완료 |
| 7   | Chart 마커 품질 차별화          | P3       | XS     | 낮음 — 시각적 정보 밀도 개선        | ✅ 완료 |
| 8   | 라이브 토스트 품질 레이블         | P3       | XS     | 낮음 — 실시간 알림의 정보 품질 향상   | ✅ 완료 |
| 11  | Wyckoff Phase 컨텍스트 레이블   | P3       | M      | 중간 — Wyckoff를 필터로 활용     | ✅ 완료 |


