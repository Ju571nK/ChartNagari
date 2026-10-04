# Forex (FX) Support — Design Spec

**Date:** 2026-10-04
**Target version:** v2.14.0.0 ("ICT for Forex")
**Branch:** `feat/forex-support`
**Status:** Implemented in the working tree; pending final review and release
validation. This remains unreleased.
**Scope decisions (confirmed by user):** Scope **B** (analysis + alerts + backtest + FX-specific visuals). Data source **A** (Yahoo keyless default, OANDA optional candle provider, subject to account eligibility).

---

## 1. Problem & Goal

ChartNagari detects ICT / Wyckoff / SMC / TA signals for US stocks and crypto. **ICT originated in the forex market**, and most ICT practitioners trade FX (EURUSD, GBPUSD, XAUUSD). Today those users cannot use ChartNagari at all, so the project is missing its largest natural audience (repo at ⭐23 / 3 forks as of 2026-10-02).

The codebase treats the asset class as a binary `crypto | stock`, and several rules assume real exchange volume. FX cannot be added just by adding a symbol.

**Goal:** A forex trader can install ChartNagari, add EURUSD in under 5 minutes **without any API key**, and get:
- correct FX-session-aware ICT/TA signals and alerts,
- session boxes, a currency strength meter and a DXY panel on the chart,
- backtests that report results in pips and include spread cost.

Users with an OANDA account eligible for v20 API access may configure a personal access token for OANDA candles (tick volume and NY-close-aligned bars). Eligibility and available history vary by region/account; a demo/practice account alone does not guarantee API access. Yahoo remains the keyless default. See the shipped [Forex guide](../../FOREX.md) for provider selection and current eligibility notes.

**Growth goal:** Reposition the project as *"the self-hosted ICT toolkit for Forex, Crypto and Stocks"* and ship a launch package (demo, GIF, guide, posts). See §9.

### Non-goals (v1)
- Order execution for FX (OANDA order plugin). The architecture should allow it later through the order plugin kit; it is not built here.
- OANDA streaming pricing endpoint. v1 polls candles.
- Exotic pairs, CFDs and indices beyond DXY. Metals are limited to XAUUSD and XAGUSD.
- Dukascopy/historical tick import (a later backfill enhancement).
- Swap/rollover cost modelling in backtests.
- Lot-size/position-size calculator.

---

## 2. Current-State Findings (what blocks FX)

| # | Location | Today | FX problem |
|---|----------|-------|-----------|
| F1 | `internal/pipeline/pipeline.go:257` | Non-crypto symbols skipped outside NYSE hours | FX would be paused ~17.5h/day |
| F2 | `pipeline.go:399`, `config/alert.yaml` | TP/SL multipliers only `crypto_*` / `stock_*` | No FX multipliers; no pip display |
| F3 | `internal/config/config.go:175` | Watchlist sections `crypto/stocks/indices` | No `forex` section |
| F4 | `internal/api/server.go:1307,1763` | `type` must be `crypto` or `stock` | API and validation reject FX |
| F5 | `internal/mcp/tools.go:51`, `plugins/alpaca/mapper.go:38` | String switches on asset class | Must recognise/reject `forex` |
| F6 | `methodology/wyckoff/*`, `general_ta/volume_spike.go`, `vsa_effort.go`, `ict/fair_value_gap.go`, `ict/liquidity_sweep.go` | Use bar volume | Yahoo FX volume = 0; OANDA volume = tick volume. Signals silently degrade or misfire |
| F7 | `methodology/ict/kill_zone.go` | Fixed UTC windows; hardcoded Korean message | Wrong by 1h for half the year (DST); not i18n |
| F8 | `internal/collector/yahoo.go:123` | 4H rebuilt from 1H on UTC boundaries | FX convention aligns candles to 17:00 New York |
| F9 | `internal/calendar/fetcher.go:180,253` | Keeps only `Country == "US"` events | EUR/GBP/JPY/... events dropped |
| F10 | `pkg/models/signal.go:53` | `AnalysisContext` has no asset-class info | Rules cannot adapt per market |

Assets we can reuse: the Yahoo collector (supports `EURUSD=X`), the economic calendar, the index collector path (`^VIX`, which DXY can follow), `demoApi.ts` and the order plugin kit.

---

## 3. Requirements

Requirement IDs are referenced by the implementation plan and tests. Each requirement has acceptance criteria (AC).

### 3.1 Asset-class foundation

**R1 — Asset class model.** Add `models.AssetClass` (`crypto | stock | index | forex`) and a resolver that maps symbol → class from the watchlist. All binary `isCrypto` checks (F1, F2, F4, F5) go through the resolver.
- AC1: `pipeline.isCrypto` is removed; a table-driven test covers every class through `assetClass(sym)`.
- AC2: `AnalysisContext` carries `AssetClass` and `VolumeQuality` (see R7).
- AC3: Unknown symbols default to `stock` (preserves current behaviour).

**R2 — Watchlist `forex` section.** `config/watchlist.yaml` gains `symbols.forex[]` (`symbol`, `enabled`, optional `provider_symbol`). Canonical symbol format: 6 uppercase letters (`EURUSD`, `XAUUSD`).
- AC1: Old watchlists without `forex` load unchanged (migration test).
- AC2: `POST /api/symbols` accepts `type: "forex"`; validation rejects non-6-letter, non-ISO-4217 pairs (XAU/XAG allowed).
- AC3: MCP `list_symbols` reports `forex`; the Alpaca mapper rejects `forex` with an explicit error.

**R3 — FX market hours.** `market.IsOpen(class, t)`:
- forex: open Sunday 17:00 to Friday 17:00 America/New_York, closed 25 Dec and 1 Jan;
- stock: existing NYSE logic;
- crypto: always open.
- AC1: The pipeline analyses FX symbols whenever FX is open, independent of NYSE.
- AC2: Tests cover Sunday 16:59/17:00, Friday 16:59/17:00, and both DST transitions.

**R4 — Instrument specs.** `market.Instrument(symbol)` returns pip size, display precision and default spread (in pips).
- JPY quote currency → 0.01; XAUUSD → 0.1; XAGUSD → 0.01; otherwise 0.0001.
- Default spreads (overridable per symbol in `symbol_profiles.yaml`): majors 1.0, crosses 2.0, XAUUSD 3.0 (in its pips).
- AC1: Table-driven tests for majors, JPY crosses and metals.

**R5 — FX TP/SL.** `alert.yaml` gains `forex_tp_mult` (default 1.5) and `forex_sl_mult` (default 1.0), with the same ATR mechanism as today. Alerts and the UI show the TP/SL distance in pips next to price.
- AC1: An existing `alert.yaml` without the keys loads the defaults.
- AC2: A Telegram/Discord alert for EURUSD shows e.g. `SL 1.08420 (−18.4 pips)`.

### 3.2 Data

**R6 — Yahoo FX collection (default, keyless).**
- Canonical `EURUSD` maps to Yahoo `EURUSD=X`.
- Metals have no reliable spot symbol on Yahoo, so XAUUSD maps to `GC=F` and XAGUSD to `SI=F`. These are futures proxies, and the UI labels them "proxy" whenever Yahoo is the provider.
- The FX collector polls on FX hours (R3), not NYSE hours.
- AC1: With no keys, adding EURUSD produces 1H/4H/1D/1W bars in `ohlcv` within one poll cycle.
- AC2: The "proxy" label appears on charts and alerts for metals on Yahoo.

**R7 — Volume quality.** Each FX bar series has a `VolumeQuality`: `real` (stocks/crypto), `tick` (OANDA) or `none` (Yahoo FX, all-zero volume).
- Pure-volume rules (`volume_spike`, `vsa_effort`, `wyckoff_volume_anomaly`) are skipped when quality is `none` and are allowed on `tick`.
- Rules that use volume only as confirmation (Wyckoff accumulation/distribution/spring/upthrust, ICT FVG, liquidity sweep) skip the volume check when quality is `none` and record `volume_unconfirmed` in signal metadata.
- `rules.yaml` gains optional `asset_classes: [...]` per rule for explicit control.
- AC1: No volume-based signal fires for Yahoo FX in a test with zero-volume bars.
- AC2: Every rule has a test case with `VolumeQuality=none`.

**R8 — OANDA provider (optional).**
- New `internal/collector/oanda.go` uses the v20 REST endpoint `GET /v3/instruments/{EUR_USD}/candles` with `price=M`, `dailyAlignment=17`, `alignmentTimezone=America/New_York` and `weeklyAlignment=Friday`.
- Granularities H1, H4, D and W are fetched natively, so 4H is not rebuilt.
- Settings: `settings.yaml` gets `forex.provider: auto|yahoo|oanda`, `forex.oanda.token` (masked like other keys) and `forex.oanda.environment: practice|live`. `auto` means OANDA if a token is set, otherwise Yahoo.
- AC1: An `httptest` server covers the candle mapping, incomplete-candle exclusion, a 401 and a 429.
- AC2: A 401 shows a clear error in Market Data Status and stops FX collection. There is **no silent fallback to Yahoo**, because mixing sources corrupts bar alignment.
- AC3: When the effective FX provider changes, existing FX `ohlcv` rows are purged and re-backfilled. The purge is logged and shown in the UI.

**R9 — NY-close alignment for Yahoo FX.** When Yahoo is the provider, 4H bars for FX are rebuilt from 1H on 17:00 New York boundaries (17, 21, 01, 05, 09, 13 NY). Yahoo's own 1D/1W bars are used as-is, and their alignment is documented.
- AC1: A test shows 4H bucket boundaries shift correctly across a DST change.

### 3.3 FX-aware methodology

**R10 — DST-aware kill zones.** `ict_kill_zone` uses America/New_York local windows: Asia 20:00–00:00, London 02:00–05:00, NY AM 07:00–10:00, London Close 10:00–12:00. The message becomes an i18n key plus params, replacing the hardcoded Korean string.
- **Behaviour change:** this applies to all asset classes, because the current UTC windows are wrong half the year. It is called out in the CHANGELOG.
- AC1: Tests at the same UTC instant in January vs July give different results.

**R11 — Session model (shared).** `market.Sessions(date)` returns Asia/London/NY session ranges. The Asian range high/low is also exposed in `AnalysisContext.Session` for rules (e.g. a later "Asian range sweep" rule). R14 uses the same data for drawing.

**R12 — Multi-currency economic calendar.** The calendar keeps events for every currency present in the watchlist. Mapping: USD→US, EUR→EU (plus DE/FR/IT), GBP→GB, JPY→JP, CHF→CH, CAD→CA, AUD→AU, NZD→NZ, CNY→CN.
- FX alerts add a line when a high-impact event for either currency is within ±60 min (configurable), e.g. `⚠ USD CPI in 42m`.
- AC1: A watchlist with only stocks/crypto keeps today's US-only behaviour.
- AC2: An EURUSD alert 30 minutes before an ECB rate decision includes the warning.

**R13 — Backtest/paper in pips with spread.** For forex, entries pay half the spread on each side (R4 default or override). Stats add `NetPips` and `AvgPips` next to the existing `WinRate`, `AvgRR` and `ProfitFactor`.
- AC1: A known fixture backtest gives the expected pip totals with and without spread.

### 3.4 FX visuals (Scope B)

**R14 — Session boxes on chart.** A chart overlay draws shaded Asia/London/NY boxes and Asian range high/low lines on intraday timeframes. It can be toggled in both beginner and expert mode, uses DESIGN.md tokens, and is available for crypto too.
- AC1: Vitest checks box positions for a fixture spanning a DST change.
- AC2: Boxes are hidden on 1D/1W.

**R15 — Currency strength meter.**
- Backend: `GET /api/forex/strength?tf=4H&lookback=20`. For each currency, take the % change over the lookback on every enabled pair containing it (sign-adjusted for base or quote side) and average the results, normalised to −100…+100.
- Currencies covered by fewer than 2 pairs are returned with `coverage: "low"`. The UI greys them out and explains why.
- Frontend: a ranked horizontal bar meter in the Analysis tab and a compact version beside the chart.
- One-click presets in the Symbols tab:
  - **Majors (7):** EURUSD, GBPUSD, USDJPY, USDCHF, AUDUSD, USDCAD, NZDUSD
  - **Majors + Gold (8)**
  - **All 28 crosses** (with an OANDA suitability hint because of Yahoo polling volume; provider access varies by account)
- AC1: Unit tests on synthetic series give the expected ranking and coverage flags.
- AC2: With only EURUSD enabled, the meter shows the low-coverage state, not misleading numbers.

**R16 — DXY panel.**
- DXY (`DX-Y.NYB`, Yahoo, 1D) is collected through the existing index path. It is added automatically when the first USD pair is enabled, and the user can remove it.
- The panel shows a DXY mini-chart and the rolling correlation (20 and 60 daily bars) between the selected USD pair and DXY. A strong negative correlation is expected for EURUSD.
- AC1: Correlation unit tests.
- AC2: The panel is hidden for non-USD symbols.

### 3.5 Product surface

**R17 — Frontend.** The Symbols tab gets a "Forex" type with presets and a pair validator. Chart prices use instrument precision. Settings gets an FX data-provider card (provider select, masked OANDA token, environment, test-connection button). Market Data Status shows the FX provider and proxy/volume-quality notes. All new strings are in en/ko/ja.

**R18 — Demo.** `demoApi.ts` ships FX sample data (EURUSD, XAUUSD), session boxes, a strength meter (sample majors) and a DXY panel, so the GitHub Pages demo shows FX with zero setup.

**R19 — Docs.**
- New `docs/FOREX.md` covers quick start, Yahoo vs OANDA, volume caveats, sessions/DST and pips/spread.
- The README, ko and ja READMEs and `docs/getting-started.md` are updated.
- `CHANGELOG.md` gets an entry that includes the kill-zone behaviour change.

---

## 4. Architecture

```
watchlist.yaml (forex[]) ──► AssetClass resolver ◄── settings.yaml (forex.provider)
                                   │
          ┌────────────────────────┼─────────────────────────────┐
          ▼                        ▼                             ▼
  FX collector selector     market.IsOpen(class,t)        market.Instrument(sym)
  ├─ YahooFX (=X / proxy)   market.Sessions(date)          pip, precision, spread
  └─ OANDA v20 (optional)          │                             │
          │ ohlcv (+VolumeQuality) │                             │
          ▼                        ▼                             ▼
                 pipeline.analyzeSymbol(ctx{AssetClass, VolumeQuality, Session})
                                   │
             rules (asset_classes filter, volume gating) ──► signals
                                   │                   │
             calendar proximity ◄──┘                   ├─► notifier (pips, event warning)
                                                       ├─► backtest/paper (spread, pips)
                                                       └─► API: /api/forex/strength, DXY corr
                                                                │
                                                     React: session boxes, strength meter, DXY panel
```

### New / changed units
| Unit | Responsibility | Depends on |
|------|----------------|-----------|
| `pkg/models/asset.go` (new) | `AssetClass`, `VolumeQuality` types | — |
| `internal/market/hours.go` (extend) | `IsOpen(class,t)`, FX hours | tz data |
| `internal/market/sessions.go` (new) | Session windows, Asian range | hours |
| `internal/market/instrument.go` (new) | pip/precision/spread | symbol profiles |
| `internal/collector/yahoo_fx.go` (new) | Yahoo FX mapping, NY-aligned 4H | yahoo.go helpers |
| `internal/collector/oanda.go` (new) | OANDA v20 candles | settings |
| `internal/forex/strength.go` (new) | Strength + coverage | storage |
| `internal/forex/correlation.go` (new) | Rolling correlation vs DXY | storage |
| `internal/pipeline` (change) | Resolver, IsOpen, FX mults, calendar proximity | above |
| `internal/rule` + rules (change) | `asset_classes` filter, volume gating | models |
| `internal/calendar` (change) | Multi-country filter | watchlist |
| `internal/backtest`, `internal/paper` (change) | Spread + pips | instrument |
| `web/src` (change/new) | Forex symbols, provider card, overlays, meter, DXY panel, demo data | API |

Each `internal/forex` unit is pure (input series → output) so it can be tested without the DB.

---

## 5. Error Handling

| Situation | Behaviour |
|-----------|-----------|
| OANDA 401/403 | FX collection stops; Market Data Status shows "OANDA token rejected"; no fallback |
| OANDA 429 / 5xx | Exponential backoff (existing retry pattern); status shows "degraded" |
| Yahoo returns no data for a pair | Symbol marked "no data" in UI, as for stocks today |
| Provider change | Purge and re-backfill FX bars (R8 AC3), logged with zerolog |
| Weekend | FX collector polls only 1D/1W, mirroring the stock off-hours behaviour |
| Strength meter low coverage | Return `coverage: low`; never fabricate a value |
| Metal on Yahoo | Proxy label everywhere it is displayed |

---

## 6. Testing Strategy

- Go table-driven tests per requirement AC (hours/DST, instruments, resolver, volume gating per rule, kill zones, calendar mapping, strength, correlation, spread backtest).
- `httptest` fakes for Yahoo FX and OANDA.
- Config migration tests for old watchlist, alert and settings files.
- Vitest + Testing Library for the Symbols presets, provider card, session overlay, strength meter, DXY panel and demo API data.
- `go test ./...` and the web test suite must pass on each phase PR.
- Manual QA (`/qa`): add the Majors preset keyless, confirm signals during London session, then switch to OANDA practice and confirm re-backfill.

---

## 7. Delivery Phases

Each phase is a separate, shippable PR to `main`.

| Phase | Contents | Requirements | Shippable outcome |
|-------|----------|--------------|-------------------|
| P1 Foundation | Asset class, watchlist/API/UI type, FX hours, instruments, Yahoo FX, volume gating, FX TP/SL | R1–R7, R9, R17 (partial) | EURUSD works end-to-end keyless |
| P2 OANDA | OANDA collector, provider settings card, purge/backfill | R8, R17 (provider card) | Optional provider available to eligible accounts |
| P3 FX methodology | DST kill zones, sessions model, multi-currency calendar, pips/spread in backtest/paper | R10–R13 | Credible for FX traders |
| P4 Visuals | Session boxes, strength meter + presets, DXY panel | R14–R16 | Screenshot-worthy features |
| P5 Launch | Demo data, FOREX.md, READMEs, GIF, release notes | R18–R19, §9 | v2.14.0.0 release |

P1→P2 and P1→P3 can run in parallel after P1. P4 depends on P1 and the R11 part of P3.

---

## 8. Decisions Made (reviewer may override)

1. **Kill-zone DST fix applies to all asset classes** (R10). Rationale: the current behaviour is wrong; FX traders will notice immediately.
2. **Yahoo metals use futures proxies** (GC=F/SI=F) with a visible label, instead of dropping gold support for keyless users. Gold is one of the most-traded ICT instruments.
3. **No silent provider fallback** (R8). Data integrity is more important than continuity.
4. **Strength meter uses only the user's enabled pairs.** There is no hidden background collection; coverage is made explicit.
5. **Canonical symbol is 6 letters without a separator** (`EURUSD`). Provider symbols (`EURUSD=X`, `EUR_USD`) are mapped internally.

---

## 9. Open-Source Growth Plan

**Positioning.**
- Repo description: *"Self-hosted ICT/SMC/Wyckoff signal detector for Forex, Crypto & Stocks — sessions, kill zones, currency strength, Telegram/Discord alerts. Go + React."*
- GitHub topics: `forex`, `ict`, `smart-money-concepts`, `trading-signals`, `kill-zones`, `currency-strength`, `self-hosted`, `golang`, `react`.

**Launch assets (P5).**
1. GitHub Pages demo opens on **EURUSD 1H with session boxes and the strength meter visible**. This is the first impression.
2. A 15-second GIF at the top of the README: London kill zone → liquidity sweep signal → Telegram alert with pips and a "USD CPI in 42m" warning.
3. `docs/FOREX.md` "5-minute keyless quick start".
4. Release notes `docs/releases/v2.14.0.0.md` with a "for ICT traders" section.

**Channels (sequenced, one per few days).**
- r/Forex and r/ICTTrading: demo-first post ("free self-hosted ICT kill-zone + strength meter").
- r/algotrading: architecture/backtest angle (spread-aware pips, volume-quality gating).
- Show HN: "Self-hosted ICT signal engine in Go".
- ForexFactory "Platform Tech" forum.
- Ask the existing community contact (r/technicalanalysis ICT practitioner) for beta feedback before launch.

**Contributor funnel.**
- Open `good first issue`s that P1 makes easy: add pip/spread specs for more pairs, new session presets (Sydney), translations, and a new FX rule (e.g. Asian range sweep using R11).
- Add a CONTRIBUTING section "Adding an FX rule".

**Success metrics (30 days after release).**
- Stars ⭐23 → ≥120.
- ≥5 FX-related issues or discussions from new users.
- ≥3 external PRs touching FX.
- These are targets, not commitments, tracked with `gh` in a `/retro`.

---

## 10. Open Questions

None are blocking. Possible follow-ups after v2.14:
- OANDA order plugin (scope C).
- Streaming prices.
- Dukascopy backfill for deep backtests.
- Swap costs.
