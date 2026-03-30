# TODOS

## Design System

- **Create DESIGN.md**
  **Priority:** P3
  **What:** Document the app's design system — the 5-color token palette (`--bg`, `--green`, `--mint`, `--text`, `--muted`) plus the 4 semantic tokens added in v2.1.1 (`--danger`, `--warning`, `--safe`, `--slate`). Explain what each token is for, where it should (and shouldn't) be used.
  **Why:** Without DESIGN.md, new UI (like the Calendar tab) arrives with hardcoded hex values that break the token system. A one-page design document would prevent this. The tokens are already in App.css — this is just surfacing them.
  **How to apply:** Run `/design-consultation` or author manually. Store at `DESIGN.md` in the repo root.
  **Note:** Becomes critical when external contributors start submitting UI-touching PRs — ensure DESIGN.md exists before the first community UI contribution.
  **Onboarding modal token reference (from design review 2026-03-28):** The OnboardingModal established the following token usage patterns — DESIGN.md should document these as canonical examples: completed state = `--safe`, error = `--danger`, warning = `--warning`, inactive/disabled = `--muted` @ 0.4 opacity, primary border/accent = `--green`, overlay = `rgba(0,0,0,0.72)`. No new tokens were added. Step indicator: 24px circles, `--green` border active, `--safe` fill complete, `--muted` inactive. All buttons use existing `.tab-btn` style (no new button variants).

## Contribution Infrastructure


## Onboarding & Discovery

- **데모 모드 시작 화면 (shadow mode)**
  **Priority:** P2
  **What:** `GET /api/demo/scan` 엔드포인트가 안정화된 후, 심볼 입력 없이 샘플 데이터
  차트를 보여주는 "shadow mode" 뷰를 추가. 최종 목표: GitHub 방문자가 클론/설치 없이
  웹에서도 제품을 체험 가능.
  **Why:** 설치 장벽이 높은 사용자(비개발자, 트레이더)가 제품 가치를 체험할 수 있는 유일한 경로.
  스타 전환율 개선.
  **Depends on:** `WithDemoEngine(*engine.RuleEngine)` 패턴으로 룰 엔진을 서버에 주입.
  현재 `server.go`는 methodology 패키지를 임포트하지 않으므로 `cmd/server/main.go`에서
  RuleEngine 빌드 + `Server.WithDemoEngine()` 주입 구현이 선행 필요.
  **Gate:** `WithDemoEngine` 패턴 구현 후. 온보딩 PR (3단계 모달)과 별개 PR로 진행.

## Open Source Growth (Phase 2)

- **Phase 2: YAML/Script-based Rule System**
  **Priority:** P2
  **What:** Extend the rule system so new trading rules can be defined via YAML config without writing Go code.
  **Why:** Current rule additions require Go — ICT domain experts (non-coders) cannot contribute directly. Phase 2 YAML support opens direct contribution to target users.
  **Pros:** Opens code-free contribution to target users; differentiates from Freqtrade.
  **Cons:** Building without a community first is premature. Start after 100⭐ + first external PR merged.
  **How to apply:** Phase 2 of the open source growth plan. Gate: 100⭐ AND first external PR merged, or Week 8, whichever comes first.
  **Depends on:** Phase 1 community formation (good first issues, CONTRIBUTING.md, 100⭐)

## Community Posts (Ready to Send)

- **Reddit / Show HN / r/selfhosted drafts**
  **Priority:** P1
  **What:** Post drafts at `docs/community-posts.md` — r/algotrading, r/golang, r/selfhosted, Show HN.
  **When:** Post after PR #11 (SVG demo) is live on main. Best time: Mon–Thu 9–11am US East.
  **HN tip:** Respond to all early comments within the first hour.

- **best-of-algorithmic-trading PR**
  **Priority:** P2
  **What:** Submit ChartNagari to algorithmic trading curated lists. Draft at `docs/best-of-algotrading-pr.md`.
  **Gate:** 10+ GitHub stars.
  **Targets:** best-of-algorithmic-trading, awesome-systematic-trading, awesome-quant.

## Completed

- **CI: CONTRIBUTING.md interface drift check** — Completed v2.1.1.0 (2026-03-23) — PR #10
- **Phase 1: Open source contributor infra** — Completed v2.1.1.0 (2026-03-22) — PR #4 (CONTRIBUTING.md, PR template, issue template, good first issues #5–#9)
- **GitHub Discussions** — Enabled 2026-03-22 (Ideas, Q&A, Show and Tell categories)
- **Phase 2: Wyckoff Phase Visualization + Backtest UI**
  **Completed:** v2.1.2.0 (2026-03-23)
  Wyckoff phase analyzer (`internal/wyckoff/`), API endpoint (`GET /api/wyckoff/{symbol}/{timeframe}`), ChartTab phase zone overlay with Spring/Upthrust markers, and BacktestTab candlestick trade chart all shipped.
