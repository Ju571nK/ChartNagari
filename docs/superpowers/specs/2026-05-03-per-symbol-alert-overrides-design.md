# Per-Symbol Alert Overrides — Design Spec

**Date:** 2026-05-03
**Branch:** `feat/mcp-server` (or new `feat/symbol-alert-overrides` after spec approval)
**Status:** Approved (sections 1–5 confirmed by user)

---

## 1. Problem & Goal

ChartNagari runs in the background and pings Telegram/Discord when rules fire. Most users have ChartNagari on a side monitor while watching TradingView/broker on the main screen. **The product's value is the alert.** If users cannot tune *which* alerts fire and *how often*, they stop relying on it.

Today they can pick from three preset profiles (`crypto`, `large_cap_stock`, `small_cap_stock`) per symbol. Anything finer requires editing `config/symbol_profiles.yaml` by hand and **restarting the server**. There is no UI to change `score_threshold`, `cooldown_hours`, or `alert_limit_per_day` for a single symbol, and no way at all to filter by timeframe.

**Goal:** Give users a per-symbol panel where they can:
- Tighten/loosen the score threshold for one symbol without affecting others.
- Restrict alerts to specific timeframes (e.g. "TSLA only on 1D and 1W, ignore 1H/4H noise").
- Override cooldown and daily limit per symbol.
- Subset the firing rules per symbol.
- See changes take effect **immediately**, with no server restart.

**Non-goal:** Replacing the profile system. Profiles remain the base. Overrides only trim or extend on top.

---

## 2. Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│  Frontend (React)                                                │
│  ┌─────────────────────────┐    ┌────────────────────────────┐  │
│  │ Symbols tab — row expand │    │ Chart tab — ⚙ modal        │  │
│  └────────┬─────────────────┘    └────────┬───────────────────┘  │
│           └──── <SymbolOverrideEditor> ───┘                       │
│                          │                                        │
└──────────────────────────┼────────────────────────────────────────┘
                           │ GET / PUT / DELETE
                           ▼
┌──────────────────────────────────────────────────────────────────┐
│  Backend (Go)                                                     │
│                                                                   │
│  HTTP handler ─────► SymbolOverrideStore ─────► SQLite            │
│  (auth: Bearer)        (CRUD, JSON cols)        symbol_alert_     │
│                                                  overrides         │
│                                                                   │
│  Pipeline tick ──► EffectiveAlertConfig(symbol)                   │
│                      ├─ profileHolder.GetProfile(symbol)  (YAML)  │
│                      └─ overrideStore.Get(symbol)         (DB)    │
│                          │                                        │
│                          ▼                                        │
│                    Filter chain (score/TF/rules/cooldown/limit)   │
│                          │                                        │
│                          ▼                                        │
│                       Notifier (Telegram/Discord)                 │
└──────────────────────────────────────────────────────────────────┘
```

**Source of truth split:**
- `config/symbol_profiles.yaml` — system defaults (profile definitions + symbol→profile mapping). Static, versioned in Git.
- `symbol_alert_overrides` table — user runtime customization. Dynamic, edited via UI, hot-reloaded.

---

## 3. Data Model

### 3.1 SQLite table

```sql
CREATE TABLE IF NOT EXISTS symbol_alert_overrides (
  symbol               TEXT PRIMARY KEY,
  score_threshold      REAL,        -- NULL = inherit from profile
  cooldown_hours       INTEGER,     -- NULL = inherit
  alert_limit_per_day  INTEGER,     -- NULL = inherit
  timeframes           TEXT,        -- NULL = inherit. JSON array, e.g. '["1D","1W"]'
  allowed_rules        TEXT,        -- NULL = inherit. JSON array of rule names
  updated_at           INTEGER NOT NULL  -- unix seconds
);
```

**Rules:**
- `NULL` in any nullable field means "inherit from profile". This is the override semantics.
- `timeframes` and `allowed_rules` are stored as JSON text (small collections; no normalization needed).
- For `timeframes` and `allowed_rules`, server treats an empty array `[]` as semantically equivalent to `NULL` (inherit). The UI never sends `[]` — when the user deselects everything, it submits `null` for that field, which the backend stores as `NULL`. This avoids the trap of a fully-deselected list silently muting all alerts.

**Semantic conventions for numeric fields** (apply at both profile and override layer):
- `score_threshold = 0` → "no minimum score" (every signal passes the score check). Distinct from `NULL` which means "use profile value".
- `cooldown_hours = 0` → "no cooldown" (back-to-back duplicates allowed).
- `alert_limit_per_day = 0` → "unlimited per day".
- The row is dropped (DELETE) when **all** fields are `NULL`. Frontend can also do this explicitly via DELETE endpoint.

### 3.2 New profile field

`internal/config/symbol_profiles.go` adds an optional field to `Profile`:

```go
Timeframes []string `yaml:"timeframes,omitempty" json:"timeframes,omitempty"`
```

Existing YAML files without this key unmarshal to `nil`/empty → "all timeframes allowed" → identical to current behavior.

### 3.3 Resolved config struct

```go
// EffectiveConfig is the result of merging a profile with its override.
type EffectiveConfig struct {
    ScoreThreshold       float64
    CooldownHours        int
    AlertLimitPerDay     int
    Timeframes           []string  // empty = allow all
    AllowedRules         []string  // empty = inherit profile (which itself may be empty = all)
    AllowedMethodologies []string
    BlockedMethodologies []string
}
```

---

## 4. Backend

### 4.1 New files

| File | Responsibility |
|---|---|
| `internal/storage/symbol_overrides.go` | `SymbolOverrideStore` — CRUD over `symbol_alert_overrides` |
| `internal/storage/symbol_overrides_test.go` | Roundtrip tests (UPSERT/Get/Delete, JSON encoding) |
| `internal/config/effective_config.go` | `EffectiveAlertConfig(symbol, profileHolder, overrideStore) EffectiveConfig` |
| `internal/config/effective_config_test.go` | Table-driven merge tests |
| `internal/api/symbol_overrides_handler.go` | HTTP GET/PUT/DELETE handlers |
| `internal/api/symbol_overrides_handler_test.go` | Handler tests including auth + validation |

### 4.2 Modified files

| File | Change |
|---|---|
| `internal/storage/db.go` | Add `CREATE TABLE IF NOT EXISTS symbol_alert_overrides ...` to migrations list |
| `internal/config/symbol_profiles.go` | Add `Timeframes []string` field to `Profile` |
| `internal/pipeline/profile_filter.go` | Replace `profileScoreThreshold`/`profileCooldownHours` calls with `EffectiveAlertConfig` lookup; add timeframe filter step |
| `internal/pipeline/profile_filter_test.go` | Extend with override-aware cases (incl. hot-reload mid-test) |
| `internal/api/server.go` | Wire new routes; inject `SymbolOverrideStore` into pipeline construction |

### 4.3 Filter chain order

```
Signal in
  ▼ score < cfg.ScoreThreshold? → drop
  ▼ len(cfg.Timeframes) > 0 && sig.Timeframe ∉ cfg.Timeframes? → drop   ← NEW
  ▼ len(cfg.AllowedRules) > 0 && sig.Rule ∉ cfg.AllowedRules? → drop
  ▼ methodology blocked by cfg.BlockedMethodologies? → drop
  ▼ cooldown not elapsed (cfg.CooldownHours)? → drop
  ▼ today's alert count for symbol ≥ cfg.AlertLimitPerDay? → drop
  ▼ Pass to notifier
```

### 4.4 Hot-reload mechanics

**No cache.** Each call to `EffectiveAlertConfig` reads the override row from DB via primary-key SELECT (~10μs on SQLite). Override changes via UI take effect on the **next** signal evaluation, no restart, no cache invalidation.

If profiling later shows DB read is a bottleneck (unlikely at our volume of ~100–300 signals/day), add an in-memory cache invalidated on PUT/DELETE. Out of scope for v1.

### 4.5 Concurrency

- `SymbolOverrideStore` wraps `*sql.DB` (already concurrency-safe).
- UPSERT is atomic. Two simultaneous PUTs for the same symbol → last-write-wins, no corruption.
- Pipeline goroutine and HTTP handler share the store; no extra locking required.

### 4.6 Defensive behavior

If `overrideStore.Get` errors (DB corruption, disk full, etc.):
- Log WARN with symbol and error.
- Return profile-only config (treat as if no override exists).
- **Alert pipeline never breaks because of override read failure.**

If `EffectiveAlertConfig` itself panics: the surrounding pipeline recovers, logs ERROR, continues with pure profile. Alerts never silently halt.

---

## 5. API

All endpoints require `Authorization: Bearer $API_TOKEN` (same as other write endpoints). Missing/invalid token → 401.

### 5.1 GET /api/symbol-overrides/:symbol

Returns the override row, or 200 + empty object when none.

```json
{
  "symbol": "TSLA",
  "score_threshold": 14.0,
  "cooldown_hours": null,
  "alert_limit_per_day": null,
  "timeframes": ["1D", "1W"],
  "allowed_rules": null,
  "updated_at": 1714723200
}
```

### 5.2 PUT /api/symbol-overrides/:symbol

Full-state replace. Frontend always sends all 5 nullable fields. `null` = clear (inherit from profile). All-`null` body → row is auto-deleted.

**Request:**
```json
{
  "score_threshold": 14.0,
  "cooldown_hours": null,
  "alert_limit_per_day": null,
  "timeframes": ["1D", "1W"],
  "allowed_rules": null
}
```

**Response (200) — merged effective config with provenance:**
```json
{
  "symbol": "TSLA",
  "score_threshold": { "value": 14.0, "source": "override" },
  "cooldown_hours":  { "value": 8,    "source": "profile"  },
  "alert_limit_per_day": { "value": 2, "source": "profile" },
  "timeframes":      { "value": ["1D","1W"], "source": "override" },
  "allowed_rules":   { "value": ["ict_order_block","ict_liquidity_sweep","ict_kill_zone","smc_bos"], "source": "profile" }
}
```

`source` is `"override"` if that field was set explicitly (non-null in DB), else `"profile"`.

### 5.3 DELETE /api/symbol-overrides/:symbol

Drops the override row entirely. Returns 200 with the same effective-config shape (now all sources are `"profile"`).

### 5.4 Validation (400)

Server validates and returns first failure as `{ "error": "<message>" }`:

| Field | Rule |
|---|---|
| `score_threshold` | 0 ≤ x ≤ 50 |
| `cooldown_hours` | 0 ≤ x ≤ 168 |
| `alert_limit_per_day` | 0 ≤ x ≤ 100 |
| `timeframes` | each ∈ `{1H, 4H, 1D, 1W}`, no duplicates. Empty array `[]` is normalized to `NULL` (inherit). |
| `allowed_rules` | each must exist in `rules.yaml` registry. Empty array `[]` is normalized to `NULL` (inherit). |

Example: `{ "error": "invalid score_threshold: must be 0~50" }`

### 5.5 Out of scope for v1

- MCP tool `set_symbol_alert_override` — additive, ship as follow-up.
- Bulk endpoint (`PUT /api/symbol-overrides` with array body).
- Listing all overrides (`GET /api/symbol-overrides`) — not needed by current UI.

---

## 6. Frontend

### 6.1 New files

| File | Responsibility |
|---|---|
| `web/src/SymbolOverrideEditor.tsx` | Self-contained editor used in both Symbols tab and Chart modal |
| `web/src/SymbolOverrideEditor.test.tsx` | Vitest + Testing Library — debounce, reset, source indicator |

### 6.2 Modified files

| File | Change |
|---|---|
| `web/src/App.tsx` (`SymbolsTab`) | Each symbol row gets an expand chevron; expanded view renders `<SymbolOverrideEditor>` |
| `web/src/App.tsx` (`ChartTab`) | Add `⚙` button next to the symbol selector; opens modal containing `<SymbolOverrideEditor>` for the current symbol |
| `web/src/i18n/locales/{en,ko,ja}.json` | New keys: `override.score_threshold`, `override.cooldown_hours`, `override.alert_limit`, `override.timeframes`, `override.allowed_rules`, `override.profile_default`, `override.reset`, `override.reset_all`, `override.saved_ago`, `override.save_failed`, `override.tooltip_blank` |

### 6.3 Component contract

```tsx
interface SymbolOverrideEditorProps {
  symbol: string
  profile: ProfileInfo            // base profile, already loaded by parent
  onChange?: (effective: EffectiveResponse) => void  // optional callback
}
```

### 6.4 Field controls

| Field | Control | Range |
|---|---|---|
| `score_threshold` | Slider + numeric input | 0–50, step 0.5 |
| `cooldown_hours` | Slider + numeric input | 0–168, step 1 |
| `alert_limit_per_day` | Slider + numeric input | 0–20, step 1 |
| `timeframes` | 4 toggle chips: 1H / 4H / 1D / 1W | multi-select |
| `allowed_rules` | Methodology-grouped collapsible checkbox grid (ICT / Wyckoff / SMC / TA / Candlestick) | multi-select |

### 6.5 Source indicator

Each field shows current value plus a hint:
- **Override active** (DB value non-null): hint reads `(Profile default: X)` and a `↺ reset` button is shown.
- **Inheriting from profile** (DB value null): hint reads `(Profile default)` only, no reset button.
- Slider track and chip background use accent color when overridden, muted color when inheriting.

### 6.6 Save behavior — auto-save with debounce

- On any control change, schedule a PUT after **500 ms** of inactivity.
- Multiple rapid changes within the window → single PUT with the latest combined state.
- After PUT 200: show transient `Saved ✓ Ns ago` indicator; fade out after 5 s.
- After PUT 400/500: show `Save failed — <message>` toast; preserve in-memory edits so the user can retry.
- On unmount with pending changes: flush immediately (synchronous PUT).
- No explicit Save button.

### 6.7 Reset behavior

- Per-field `↺ reset`: sends a PUT with that field set to `null`, others unchanged. Effective response updates the source indicator to `"profile"`.
- Bottom button `Reset all to profile`: DELETE the row. All sources become `"profile"`. No confirmation dialog (the action is reversible by editing again).

### 6.8 Quick-access modal (Chart tab)

- Button: small `⚙` icon next to the symbol `<select>` in `ChartTab`.
- On click: opens a centered modal (reuse `OnboardingModal` style/scaffold) containing `<SymbolOverrideEditor symbol={currentSymbol} profile={...}/>`.
- Closing the modal does not need to "save" — auto-save already handled it.

### 6.9 i18n examples (Korean)

- `override.score_threshold` → "점수 임계값"
- `override.cooldown_hours` → "쿨다운 (시간)"
- `override.alert_limit` → "일일 알람 한도"
- `override.timeframes` → "타임프레임"
- `override.allowed_rules` → "사용 룰"
- `override.profile_default` → "프로파일 기본값"
- `override.reset` → "초기화"
- `override.reset_all` → "프로파일로 전체 초기화"
- `override.saved_ago` → "{{n}}초 전 저장됨"
- `override.save_failed` → "저장 실패"
- `override.tooltip_blank` → "잘 모르겠으면 비워두세요 — 프로파일 기본값을 따릅니다."

---

## 7. Testing

| Layer | File | Coverage |
|---|---|---|
| Storage CRUD | `internal/storage/symbol_overrides_test.go` | UPSERT/Get/Delete, JSON encode/decode roundtrip, missing-row returns `(nil, nil)`, all-null PUT auto-deletes |
| Merge logic | `internal/config/effective_config_test.go` | Table-driven: no override / partial / full / all-null override / symbol with no profile uses default, `Timeframes` propagation |
| Pipeline filter | `internal/pipeline/profile_filter_test.go` (extend) | Score cut, timeframe cut, allowed_rules cut, cooldown extension, hot-reload (write override → next eval uses new value) |
| API handler | `internal/api/symbol_overrides_handler_test.go` | GET 200 empty / 200 full, PUT 200 + effective, PUT 400 each invalid field, PUT 401 no token, DELETE 200, all-null PUT triggers DELETE behavior |
| Frontend component | `web/src/SymbolOverrideEditor.test.tsx` | Renders profile defaults, slider change → debounced PUT (fake timers), reset → PUT with null, Reset-all → DELETE, source indicator color/text changes correctly |
| i18n parity | `scripts/check_i18n.sh` | All 3 locales (en/ko/ja) have identical key sets |

**Pass bar:**
- `go test ./...` race-clean.
- `bun test` (vitest) 100%.
- `tsc --noEmit` clean.
- `vite build` clean.

---

## 8. Migration & Compatibility

- **DB:** New `CREATE TABLE IF NOT EXISTS` line in `internal/storage/db.go` migrations. Idempotent on existing installs.
- **Data:** No backfill needed. Empty table = every existing symbol behaves exactly like today.
- **YAML:** New optional `Timeframes []string` field in `Profile`. Absent in current YAMLs → unmarshals to `nil` → "all timeframes allowed" → identical to current behavior.
- **API:** All new endpoints are additive. Existing `/api/profiles/:symbol` unchanged.
- **Downgrade safety:** Rolling back from v2.8 → v2.7 leaves the unused table behind. Old code ignores it. No corruption.

---

## 9. Error Handling Summary

| Scenario | Behavior |
|---|---|
| DB read fails in `EffectiveAlertConfig` | WARN log, fall back to pure profile. Alerts continue. |
| DB write fails (PUT/DELETE) | 500 + `{error: "..."}`. Frontend toasts "Save failed", retains edits for retry. |
| Validation failure (PUT) | 400 + first invalid field message. Frontend shows inline error, halts auto-save until corrected. |
| Two browser tabs editing same symbol | Last-write-wins. Documented; no merge attempted. |
| `EffectiveAlertConfig` panic | Pipeline goroutine recovers, ERROR-logged, continues with profile. |
| Network failure on GET | Toast `Could not load overrides`, show retry button. Component still renders profile defaults. |

---

## 10. Observability

- INFO log on every override write: `override updated symbol=TSLA fields=[score_threshold,timeframes]`
- HTTP access logs (existing zerolog middleware) cover PUT/DELETE.
- No new metrics. Alert volume already tracked via existing pipeline counters.

---

## 11. Out of Scope (v1)

These are valid follow-ups but explicitly not v1:

1. **MCP tool** for setting overrides programmatically.
2. **Bulk operations** ("apply this override to multiple symbols").
3. **Override audit log** / version history.
4. **Time-bound overrides** ("strict during 9–16 KST").
5. **Import/Export** of override sets.
6. **Per-symbol custom price alerts integration** (the existing `PriceAlert` system stays separate; unifying is a future redesign).

---

## 12. Success Criteria

A user can:
1. Open Symbols tab, expand `TSLA`, drag the score-threshold slider from 10 → 14, see "Saved ✓" within 1 s.
2. Within the next pipeline tick (≤ 1 min for stocks, ≤ 5 s for crypto), a TSLA signal with score 12 is filtered out instead of alerting.
3. Click `Reset all to profile`, see TSLA immediately resume profile-only behavior — without restarting the server.
4. From Chart tab while viewing BTCUSDT, click `⚙`, untoggle 1H and 4H, close the modal, and stop receiving 1H/4H BTCUSDT alerts on the next tick.
5. None of the above touches `config/symbol_profiles.yaml` or requires editor access to the host machine.
