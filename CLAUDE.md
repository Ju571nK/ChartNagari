# CLAUDE.md — Chart Analyzer 프로젝트 진입점

> Claude Code가 이 프로젝트를 열면 **반드시 이 파일을 먼저 읽고 시작합니다.**

## 첫 번째로 할 일

1. `AGENTS.md` 를 읽어 팀 구조와 역할을 파악한다
2. `docs/STATUS.md` 를 읽어 현재 팀 상태를 파악한다 (없으면 신규 프로젝트)
3. `docs/pending/` 폴더를 확인한다 — Owner 응답 대기 항목이 있으면 먼저 알린다
4. `PRD.md` 를 읽어 현재 Phase와 목표를 파악한다
5. Owner의 요청을 Orchestrator로서 판단하고 적절한 에이전트를 호출한다

## 프로젝트 한 줄 요약

미국 주식 및 암호화폐를 대상으로 ICT/Wyckoff 등 방법론을 플러그인 방식으로
탑재하여 MTF(1W/1D/4H/1H) 신호를 자동 감지하고 Telegram으로 알림을 발송하는
로컬 실행 플랫폼. Go 백엔드 + TypeScript 프론트엔드.

## 기술 스택

- Backend  : Go 1.22+
- Frontend : TypeScript + React 18 + Vite
- DB       : SQLite (로컬)
- 알림     : Telegram Bot / Discord Webhook
- 데이터   : Yahoo Finance (주식) / Binance WebSocket (코인)

## 핵심 원칙

- Orchestrator 없이 직접 구현 시작 금지
- Quality Gate 3단계 통과 후에만 코드 반영
- Owner는 중요 결정만 판단 — 나머지는 팀이 자율 처리
