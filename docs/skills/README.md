# ChartNagari Skills Guide

gstack skills 사용 가이드 — 어떤 작업에 어떤 skill을 쓸지 정리한 참조 문서.

## 빠른 참조

| 작업 | Skill | 에이전트 |
|------|-------|---------|
| 새 기능 설계 상담 | `/office-hours` | — |
| 제품 방향 / 스코프 결정 | `/plan-ceo-review` | — |
| 구현 계획 아키텍처 검토 | `/plan-eng-review` | — |
| UI/UX 설계 검토 | `/plan-design-review` | react-frontend |
| PR 생성 + 테스트 + 배포 | `/ship` | release-engineer |
| 코드 리뷰 (diff 스코프) | `/review` | — |
| 웹 QA 테스트 | `/qa` | — |
| 버그 디버깅 | `/investigate` | — |
| 시각적 디자인 검토 | `/design-review` | react-frontend |
| 문서 업데이트 | `/document-release` | — |

---

## 워크플로별 가이드

### 새 트레이딩 룰 추가

```
1. /office-hours     — 룰 로직 설계 상담 (무엇을 감지할지)
2. /plan-eng-review  — 구현 계획 검토 (인터페이스, 테스트 커버리지)
3. [구현]             — trading-analyst 에이전트 사용
4. /review           — PR 올리기 전 diff 검토
5. /ship             — 테스트 → CHANGELOG → PR 자동화
```

### 프론트엔드 기능 추가

```
1. /office-hours        — UX 아이디어 상담
2. /plan-design-review  — 디자인 토큰 사용, 빈 상태, 인터랙션 상태 검토
3. /plan-eng-review     — Vitest 커버리지, 컴포넌트 경계 검토
4. [구현]               — react-frontend 에이전트 사용
5. /design-review       — 시각적 QA (라이브 사이트 대상)
6. /ship                — PR 생성
```

### 버그 수정

```
1. /investigate  — 근본 원인 분석
2. [수정]        — go-backend 또는 react-frontend 에이전트 사용
3. /ship         — 수정 + 회귀 테스트 포함 PR
```

### 릴리즈 준비

```
1. /review       — 최종 diff 검토
2. /ship         — VERSION 범프, CHANGELOG, PR 전자동
3. [merge 후]    — /document-release 로 docs 동기화
```

---

## 에이전트 역할 분담

| 에이전트 | 파일 | 담당 영역 |
|---------|------|---------|
| `go-backend` | `.claude/agents/go-backend.md` | `cmd/`, `internal/` 전체 Go 코드 |
| `react-frontend` | `.claude/agents/react-frontend.md` | `web/src/` 전체 React/TypeScript |
| `trading-analyst` | `.claude/agents/trading-analyst.md` | `internal/methodology/` 트레이딩 룰 |
| `release-engineer` | `.claude/agents/release-engineer.md` | VERSION, CHANGELOG, PR, CI |

---

## Skills 상세

### /office-hours
- **목적**: 기능 설계 전 YC 오피스아워 스타일 상담. 문제 정의 → 전제 검토 → 대안 도출 → 디자인 문서 생성.
- **언제**: 코드 작성 전, 무엇을 만들지 확실하지 않을 때
- **결과물**: `~/.gstack/projects/` 에 저장되는 설계 문서

### /plan-ceo-review
- **목적**: 스코프 · 전략 검토. 제품 방향 도전, 10x 가능성 탐색.
- **언제**: 큰 기능 시작 전, 스코프 결정이 필요할 때
- **모드**: SCOPE_EXPANSION / SELECTIVE_EXPANSION / HOLD_SCOPE / SCOPE_REDUCTION

### /plan-eng-review
- **목적**: 구현 계획 아키텍처 검토. 테스트 커버리지 다이어그램, 엣지 케이스, 성능.
- **언제**: 구현 시작 전, 계획이 확정된 후
- **필수**: `/ship` 전에 Eng Review가 통과되어야 함

### /plan-design-review
- **목적**: 디자인 계획 검토. 정보 계층, 인터랙션 상태, AI 슬롭 리스크.
- **언제**: UI 변경 포함 기능의 구현 전
- **결과**: 계획에 디자인 결정 사항 직접 추가

### /ship
- **목적**: 전자동 릴리즈 파이프라인.
- **실행 순서**: 테스트 → 코드 리뷰 → VERSION 범프 → CHANGELOG → TODOS.md → 커밋 → 푸시 → PR
- **주의**: main 브랜치에서 실행 불가. feature 브랜치에서 실행.

### /review
- **목적**: diff 스코프 사전 착륙 검토. SQL 안전성, LLM 신뢰 경계, 조건부 사이드 이펙트.
- **언제**: `/ship` 이 자동으로 실행하지만, 수동으로도 사용 가능

### /qa
- **목적**: 웹앱 QA 테스트 + 버그 수정.
- **언제**: 프론트엔드 기능 구현 후 (서버 실행 중이어야 함)

### /investigate
- **목적**: 체계적 버그 디버깅. 근본 원인 없이 수정 금지.
- **언제**: 버그 재현 가능하지만 원인 불명일 때

### /design-review
- **목적**: 라이브 사이트 시각적 QA. 토큰 불일치, 간격 문제, AI 슬롭 패턴 탐지.
- **언제**: 프론트엔드 기능 구현 완료 후 (브라우저 접근 필요)

### /document-release
- **목적**: PR merge 후 문서 동기화. README, ARCHITECTURE, CONTRIBUTING, CLAUDE.md 업데이트.
- **언제**: `/ship` 이 자동으로 실행하지만, 문서만 업데이트할 때도 사용 가능

---

## ChartNagari 특이사항

- **Go 테스트**: `go test ./...` — PR 전 필수
- **프론트 테스트**: `cd web && npm test` — PR 전 필수
- **룰 추가**: `internal/methodology/<카테고리>/` + `config/rules.yaml` 등록 필수
- **디자인 토큰**: hex 값 직접 사용 금지, 항상 CSS 변수 사용
- **i18n**: 하드코딩 영문 금지, `t('key')` 사용 + en/ko/ja 3개 로케일 모두 추가
- **로깅**: `fmt.Println` 금지, zerolog 사용 (`log.Info().Str(...)`)
