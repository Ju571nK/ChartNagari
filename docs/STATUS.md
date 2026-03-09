# docs/STATUS.md — 팀 현재 상태

> Orchestrator가 관리. 매 작업 완료 시 갱신.
> 최종 갱신: 2026-03-08

---

## 현재 Phase

**Phase 2: Enhancement — IN PROGRESS**

---

## 완료된 작업

| 날짜 | 항목 | 처리 에이전트 |
|------|------|-------------|
| 2026-03-07 | 초기 문서 세트 생성 | Recorder |
| 2026-03-07 | docs/ 하위 폴더 구조 생성 | Developer |
| 2026-03-07 | GitHub 원격 저장소 연결 + 초기 push | Developer |
| 2026-03-07 | Phase 0: Go scaffold + Docker Compose | Developer |
| 2026-03-07 | Phase 1-1: 프로젝트 구조 (모듈, 인터페이스, 모델) | Developer |
| 2026-03-07 | Phase 1-2: Binance WebSocket 수집기 | Developer |
| 2026-03-07 | Phase 1-3: Yahoo Finance 주식 수집기 | Developer |
| 2026-03-07 | SQLite storage 레이어 (WAL, 스키마, CRUD) | Developer |
| 2026-03-07 | TF 재구성 유틸리티 (1H→4H/1D/1W) | Developer |
| 2026-03-07 | 테스트 11개 전체 PASS | Developer |
| 2026-03-07 | Phase 1-4: 인디케이터 엔진 (14개 테스트 PASS) | Developer |
| 2026-03-07 | Phase 1-5: 룰 엔진 (10개 테스트 PASS) | Developer |
| 2026-03-07 | Phase 1-6: 일반 기술적분석 플러그인 6종 (18개 테스트 PASS) | Developer |
| 2026-03-07 | Phase 1-8: ICT 방법론 플러그인 5종 (15개 테스트 PASS) | Developer |
| 2026-03-07 | Phase 1-9: Wyckoff 방법론 플러그인 5종 (14개 테스트 PASS) | Developer |
| 2026-03-07 | Phase 1-7: Telegram/Discord 알림 시스템 (18개 테스트 PASS) | Developer |
| 2026-03-07 | Phase 1-10: React + TypeScript 설정 UI + Go REST API (16개 테스트 PASS) | Developer |
| 2026-03-08 | Phase 2-1: Claude AI 해석 레이어 + 분석 파이프라인 (13개 테스트 PASS) | Developer |
| 2026-03-08 | Phase 2-2: SMC 방법론 (BOS, CHoCH) 플러그인 2종 (14개 테스트 PASS) | Developer |
| 2026-03-08 | Phase 2-3: 차트 대시보드 (TradingView + 신호 영속성 + API 엔드포인트) | Developer |
| 2026-03-08 | Phase 2-4: 백테스팅 엔진 (engine/stats/runner + POST /api/backtest + 프론트 탭, 10개 테스트 PASS) | Developer |
| 2026-03-08 | TraderAdvisor 합류 + AGENTS.md v0.3 갱신 | Orchestrator/Recorder |
| 2026-03-08 | 알림(Telegram/Discord)에 진입가/TP/SL 추가 (3개 테스트 추가, 전체 PASS) | Developer |
| 2026-03-08 | 무료 데이터 소스 리서치 — Tiingo 1순위 권고 (VERIFIED) | Researcher |
| 2026-03-08 | Tiingo 수집기 구현 (internal/collector/tiingo.go + config + main.go 연결) | Developer |
| 2026-03-08 | 페이퍼 트레이딩 엔진 구현 (paper/trader + storage/paper + API 3개 + 프론트 탭, 10개 테스트 PASS) | Developer |

---

## 진행 중

**Phase 2: Enhancement — 2-4 완료, 2-5(Bloomberg/유료 데이터) 대기 중**

---

## 블로커

없음.

---

## 다음 할 일 (우선순위 순)

1. **Owner 액션**: `.env`에 `TIINGO_API_KEY={발급받은 키}` 추가 → 서버 재시작 시 자동 활성화
2. TraderAdvisor 🟡 추가 권고 (알림 히스토리 뷰, 백테스트 TP/SL 최적화)
3. Phase 3: 클라우드 배포

---

## PENDING 항목

없음.
