# docs/STATUS.md — 팀 현재 상태

> Orchestrator가 관리. 매 작업 완료 시 갱신.
> 최종 갱신: 2026-03-07

---

## 현재 Phase

**Phase 1: Core MVP — IN PROGRESS**

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

---

## 진행 중

| 항목 | 담당 | 상태 |
|------|------|------|
| Phase 1-4: 인디케이터 엔진 | Developer | 대기 중 |

---

## 블로커

없음.

---

## 다음 할 일 (우선순위 순)

1. **Phase 1-4**: 인디케이터 엔진 (RSI, MACD, EMA, SMA, BB, OBV, ATR, Fibonacci)
2. **Phase 1-5**: 룰 엔진 인터페이스 + YAML 로더 + 스코어링

---

## PENDING 항목

없음.
