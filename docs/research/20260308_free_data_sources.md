## Research Report: Yahoo Finance 대체 무료 데이터 소스

- 조사일: 2026-03-08
- 신뢰도: VERIFIED
- 배경: Bloomberg 유료 API 계약 불가 (Owner 결정). Yahoo Finance 비공식 API의 불안정성 보완 필요.

---

### 개념 요약

현재 플랫폼은 주식 데이터를 Yahoo Finance 비공식 REST API로 수집한다.
이 API는 공식 지원이 없어 언제든 차단될 수 있으며, 데이터 딜레이가 15~20분 발생한다.
아래는 무료 또는 저비용으로 교체 가능한 옵션이다.

---

### 옵션 비교

| 소스 | 자산 | 실시간 | 히스토리컬 | 무료 한도 | 구현 난이도 |
|------|------|--------|-----------|-----------|------------|
| **Yahoo Finance** (현재) | 주식 | ❌ (15분 딜레이) | ✅ 풍부 | 비공식 무제한 | 이미 구현 |
| **Polygon.io** | 미국 주식 | ✅ (유료 플랜) / ❌ (무료) | ✅ 2년+ | 무제한 REST (지연 15분) | M |
| **Alpaca Markets** | 미국 주식 | ✅ (계좌 필요) | ✅ 5년+ | IEX 무료 / SIP 유료 | M |
| **Tiingo** | 주식 + 코인 | ✅ WebSocket | ✅ | 500 req/hr 무료 | S |
| **Alpha Vantage** | 주식 + 코인 | ❌ (딜레이) | ✅ | 25 req/day 무료 | S |
| **Binance** (현재) | 코인 | ✅ WebSocket | ✅ | 무제한 | 이미 구현 |
| **CoinGecko** | 코인 | ❌ (딜레이) | ✅ | 30 req/min 무료 | S |

---

### 진입/청산 조건 (구현 관점)

#### 1순위 권고: Tiingo

**이유:**
- 공식 REST API + WebSocket 지원 → 현재 Yahoo/Binance 아키텍처와 유사
- 무료 플랜으로 주식 + 코인 모두 커버
- API Key 기반으로 안정적 (비공식 스크래핑 아님)
- 히스토리컬 데이터 풍부 (백테스트에 유용)

**기술 접근:**
- REST: `GET https://api.tiingo.com/tiingo/daily/{symbol}/prices` (일봉)
- WebSocket: `wss://api.tiingo.com/iex` (실시간 IEX 데이터)
- 인증: `Authorization: Token {API_KEY}` 헤더

**제한:**
- 분봉(1m/5m) 데이터는 유료 플랜 필요 → 현재 플랫폼 최소 TF가 1H이므로 무관

#### 2순위: Polygon.io 무료 플랜

**이유:**
- 미국 주식 전문, 데이터 품질 높음
- 무료 플랜은 지연 15분이나 Yahoo와 동일 수준
- WebSocket은 유료 → 현재는 폴링 방식 그대로 사용 가능

**제한:**
- 코인 데이터 없음 (주식 전용)
- 무료 플랜 API 레이트 리밋: 5 req/min → 현재 watchlist 4~6 종목 × 4 TF = 24 req → 5분당 24 req, 충분

---

### 필요 인디케이터

없음. 기존 indicator 패키지 완전 호환.

### 기존 방법론과의 관계

없음. 데이터 수집 레이어만 교체. `internal/collector/` 하위에 신규 파일 추가.

### 구현 난이도

**S (Small)** — Tiingo 기준:
- `internal/collector/tiingo.go` 신규 파일 (~100 LOC)
- `config/watchlist.yaml`에 `source: tiingo` 필드 추가
- `cmd/server/main.go`에서 Yahoo 대신 또는 병행하여 등록

### Researcher 의견

Yahoo Finance의 불안정성은 실제 리스크다. **Tiingo를 Yahoo의 백업 또는 주 소스**로
교체하는 것을 권고한다. 구현 규모가 작고(S), API Key 한 줄 설정으로 즉시 활성화 가능하다.
Polygon.io는 주식 전용으로 코인이 없으므로 단독 대체는 불가. Tiingo를 주 소스로,
코인은 Binance 유지하는 구조가 최적.

**다음 단계:** Owner가 Tiingo 무료 계정 생성 후 API Key를 `.env`의 `TIINGO_API_KEY`에 설정.
Developer는 `internal/collector/tiingo.go` 구현 시작 가능.
