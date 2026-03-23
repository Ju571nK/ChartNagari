# TODOS

## Design System

- **Create DESIGN.md**
  **Priority:** P3
  **What:** Document the app's design system — the 5-color token palette (`--bg`, `--green`, `--mint`, `--text`, `--muted`) plus the 4 semantic tokens added in v2.1.1 (`--danger`, `--warning`, `--safe`, `--slate`). Explain what each token is for, where it should (and shouldn't) be used.
  **Why:** Without DESIGN.md, new UI (like the Calendar tab) arrives with hardcoded hex values that break the token system. A one-page design document would prevent this. The tokens are already in App.css — this is just surfacing them.
  **How to apply:** Run `/design-consultation` or author manually. Store at `DESIGN.md` in the repo root.
  **Note:** Becomes critical when external contributors start submitting UI-touching PRs — ensure DESIGN.md exists before the first community UI contribution.

## Contribution Infrastructure


## Open Source Growth (Phase 2)

- **Phase 2: YAML/Script-based Rule System**
  **Priority:** P2
  **What:** Extend the rule system so new trading rules can be defined via YAML config without writing Go code.
  **Why:** Current rule additions require Go — ICT domain experts (non-coders) cannot contribute directly. Phase 2 YAML support opens direct contribution to target users.
  **Pros:** Opens code-free contribution to target users; differentiates from Freqtrade.
  **Cons:** Building without a community first is premature. Start after 100⭐ + first external PR merged.
  **How to apply:** Phase 2 of the open source growth plan. Gate: 100⭐ AND first external PR merged, or Week 8, whichever comes first.
  **Depends on:** Phase 1 community formation (good first issues, CONTRIBUTING.md, 100⭐)

- **Phase 2: Wyckoff Phase Visualization + Backtest UI**
  **Priority:** P1
  **What:** Expose `internal/backtest/` module via web dashboard tab. Add Wyckoff phase (Spring/Upthrust/Distribution/Accumulation) visualization to chart view.
  **Why:** All 6 Wyckoff visualization repos on GitHub total 17⭐. Real-time visualization = immediate #1 position in Wyckoff OSS space. Also a strong hiring signal demo.
  **Pros:** "Whoa" moment for new visitors; recruiter-visible demo.
  **Cons:** Must ship Phase 1 (discoverability + contribution infra) first or feature stays undiscovered.
  **How to apply:** Implement after Phase 1 is complete (CONTRIBUTING.md + 5 good first issues + README reframe).
  **Depends on:** Phase 1 completion (100⭐ target)

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
