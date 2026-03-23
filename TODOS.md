# TODOS

## Design System

- **Create DESIGN.md**
  **Priority:** P3
  **What:** Document the app's design system — the 5-color token palette (`--bg`, `--green`, `--mint`, `--text`, `--muted`) plus the 4 semantic tokens added in v2.1.1 (`--danger`, `--warning`, `--safe`, `--slate`). Explain what each token is for, where it should (and shouldn't) be used.
  **Why:** Without DESIGN.md, new UI (like the Calendar tab) arrives with hardcoded hex values that break the token system. A one-page design document would prevent this. The tokens are already in App.css — this is just surfacing them.
  **How to apply:** Run `/design-consultation` or author manually. Store at `DESIGN.md` in the repo root.
  **Note:** Becomes critical when external contributors start submitting UI-touching PRs — ensure DESIGN.md exists before the first community UI contribution.

## Contribution Infrastructure

- **CI: CONTRIBUTING.md interface drift check**
  **Priority:** P3
  **What:** Add a CI step to `ci.yml` that greps `CONTRIBUTING.md` for the `AnalysisRule` method signatures and fails if they don't match `internal/rule/interface.go`.
  **Why:** The `rule.Rule` interface bug in CONTRIBUTING.md was only caught manually. If the interface ever changes, the doc will silently drift again, breaking new contributors' first experience.
  **Pros:** Zero-maintenance protection; prevents the same bug from recurring; ~30-line shell script.
  **Cons:** Slightly brittle grep patterns; needs updating if interface comments change.
  **How to apply:** Add a `check-docs` job to `.github/workflows/ci.yml` that runs `grep -q "Name() string" CONTRIBUTING.md && grep -q "RequiredIndicators() \[\]string" CONTRIBUTING.md && grep -q "Analyze(" CONTRIBUTING.md` — fails if any method signature is missing.
  **Depends on:** Phase 1 CONTRIBUTING.md rewrite (must be completed first so the correct signatures exist)

## Open Source Growth (Phase 2)

- **Phase 2: YAML/Script-based Rule System**
  **Priority:** P2
  **What:** Extend the rule system so new trading rules can be defined via YAML config without writing Go code.
  **Why:** Current rule additions require Go — ICT domain experts (non-coders) cannot contribute directly. Phase 2 YAML support opens direct contribution to target users.
  **Pros:** Opens code-free contribution to target users; differentiates from Freqtrade.
  **Cons:** Building without a community first is premature. Start after 100⭐ + first external PR merged.
  **How to apply:** Phase 2 of the open source growth plan. Gate: 100⭐ AND first external PR merged, or Week 8, whichever comes first.
  **Depends on:** Phase 1 community formation (good first issues, CONTRIBUTING.md, 100⭐)

## Completed

- **Phase 2: Wyckoff Phase Visualization + Backtest UI**
  **Completed:** v2.1.2.0 (2026-03-23)
  Wyckoff phase analyzer (`internal/wyckoff/`), API endpoint (`GET /api/wyckoff/{symbol}/{timeframe}`), ChartTab phase zone overlay with Spring/Upthrust markers, and BacktestTab candlestick trade chart all shipped.
