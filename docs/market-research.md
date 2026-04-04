# Market Research — Wyckoff/ICT Signal Detection Tools

## Koventium (koventium.com)

**조사일:** 2026-04-04
**출처:** Reddit u/PracticalOil9183 포스트 + 사이트 직접 조사

### 운영자
- **이름:** Arsalan Amiri
- **위치:** Frankfurt am Main, Germany
- **이메일:** info@koventium.com
- **법적 상태:** 개인 운영, BaFin 미등록 (교육/정보 도구)
- **데이터:** Twelve Data

### 서비스 개요
- Wyckoff accumulation 감지에 특화된 웹 기반 SaaS
- 무료 (향후 유료 전환 가능성 — footer에 "Kündigen" 해지 링크 존재)
- 229종목, 2006~2025 백테스트, ~28,000 시그널 (Reddit 기준)
- 2026년 라이브 트래킹 진행 중
- 코드 비공개, IP 보호 명시

### Scoring 방법론
- Volume structure
- Relative strength
- Price position
- Forward return 추적: 5일, 10일, 20일, 40일

### 기술 스택
- SPA (JavaScript 렌더링)
- Twelve Data API
- 온보딩: 초보자/경험자 모드 선택

### ChartNagari와의 비교

| 항목 | Koventium | ChartNagari |
|------|-----------|-------------|
| 방법론 | Wyckoff만 | ICT + Wyckoff + SMC + TA + Candlestick (33룰) |
| 호스팅 | 클라우드 SaaS | 셀프호스팅 (Docker) |
| 자산 | 주식 229개 | 주식 + 크립토 |
| 알림 | 불명 | Telegram + Discord |
| 가격 | 무료 (유료 전환 가능성) | 무료 오픈소스 (MIT) |
| 코드 | 비공개 | 오픈소스 |
| 데이터 | Twelve Data | Binance + Yahoo/Tiingo |
| AI 해석 | 없음 | Anthropic/OpenAI/Groq/Gemini |
| 백테스트 규모 | 229종목 × 20년 | 사용자 워치리스트 기준 |
| UX | 초보자/경험자 모드 | 단일 모드 |

### ChartNagari가 배울 수 있는 점
1. **Forward return tracking** (5/10/20/40일) — 시그널 이후 N일 수익률 추적. 백테스트에 추가 가치.
2. **초보자/경험자 모드** — 온보딩 시 사용자 수준에 따라 UI 복잡도 조절.
3. **대규모 백테스트 데이터** — 20년 × 229종목 규모의 검증은 신뢰도를 크게 높임.
4. **Scoring 세분화** — volume structure, relative strength, price position을 별도 팩터로 분리.

### ChartNagari의 차별점
1. **멀티 방법론 통합** — ICT/Wyckoff/SMC/TA를 하나의 파이프라인에서 교차 검증
2. **셀프호스팅** — 데이터가 로컬에 머물러 프라이버시 보장
3. **오픈소스** — 커뮤니티 기여, 코드 투명성, 커스터마이징 가능
4. **크립토 지원** — Binance WebSocket 실시간 데이터
5. **AI 해석 레이어** — LLM 기반 자연어 시그널 해석
6. **실시간 알림** — Telegram/Discord
7. **시그널 품질 점수** — 볼륨, 윅, 반전 강도 기반 다팩터 스코어링
8. **변동성 레짐** — ATR 퍼센타일 기반 시그널 조정
