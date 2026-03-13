# docs/STATUS.md — 팀 현재 상태

> Orchestrator가 관리. 매 작업 완료 시 갱신.
> 최종 갱신: 2026-03-13

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
| 2026-03-13 | TIINGO_API_KEY .env 등록 완료 | Owner |
| 2026-03-08 | 페이퍼 트레이딩 엔진 구현 (paper/trader + storage/paper + API 3개 + 프론트 탭, 10개 테스트 PASS) | Developer |
| 2026-03-13 | Phase 2-6: 주식 전용 일일 리포트 (report 패키지 + 웹 UI 설정 탭 + API, 14개 테스트 PASS) | Developer |
| 2026-03-13 | 신호 히스토리 탭 + 백테스트 TP/SL 배율 수동 입력 (전체 테스트 PASS) | Developer |
| 2026-03-13 | Quant 에이전트 팀 합류 (AGENTS.md v0.4) | Orchestrator/Recorder |
| 2026-03-13 | MTF 합의 필터(기본값 2 TF) + 알림 설정 웹 UI '알림' 탭 (전체 테스트 PASS) | Quant/Developer |

---

## 진행 중

**Phase 2: Enhancement — 전 항목 완료 (2-5 BLOCKED 제외). Phase 3 클라우드 배포 대기 중**

---

## 블로커

없음.

---

## 다음 할 일 (우선순위 순)

1. **서버 재시작** — MTF 합의 필터 적용을 위해 필요
2. **관찰 기간** — 2주 페이퍼 트레이딩 후 Quant가 승률 재분석 예정
3. Phase 3 클라우드 배포 — 보안 강화 후 진행 예정 (Owner 결정 대기)

---

## PENDING 항목

없음.
