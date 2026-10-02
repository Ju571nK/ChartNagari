# Issue #32 — staged HTF calibration

## Phase 1: history inventory (implemented, not automatic calibration)

Open Backtest, select a symbol and 1H or 4H, expand **HTF calibration · history
inventory**, then select **Check history**. English, Korean and Japanese labels
are included. The request is read-only:

`GET /api/backtest/htf-readiness?symbol=SPCX&timeframe=1H`

The report aggregates the complete stored signal history, not the chart's
200-candle slice. It counts non-neutral signals opposing a known HTF direction
with an ATR percentile in [0,100], excludes future timestamps, and separates
symbols and timeframes. Deciles are half-open except the final [90,100] bucket.
Empty buckets retain null dates. Queries are cancellable and time-limited.

The initial history gate is two calendar years between first and last observations
and at least 30 distinct UTC dates in each bucket. Repeated signals on one date
do not satisfy the date count. This is a sparsity check, not a statistical test
or proof of continuous coverage. Counts are signals, not independent trades.

`ready` is always false in this phase, regardless of the gate. There is no save,
activation or order action, and no change to existing gradient/manual penalties.

## Phase 2a: versioned live opportunity collection

The server now writes `htf_opportunities` immediately after the rule engine,
before profile, score, timeframe, MTF, HTF, cooldown or notification filters.
Only directional 1H/4H outputs of enabled rules are collected. Original score
means the engine's weighted score, before pipeline adjustments.

Each row records collection version, symbol, timeframe, rule, direction, the
source candle's open timestamp, actual observation timestamp (both Unix
milliseconds), original score, raw and effective HTF trends, and daily ATR
percentile (`-1` when unavailable). The JSON snapshot contains the computed
indicators, latest candle per timeframe and Wyckoff phase. The effective context
uses the same helper as the live HTF filter, including weekly fallback and
Wyckoff relaxation.

The first observation per version/symbol/timeframe/rule/direction/candle wins.
Repeated ticks and restarts do not overwrite it; a new direction on the same
candle is a separate observation. Inserts are atomic per analysis batch and
storage failures are logged without preventing normal signal processing.

These are **live first-observation samples**, which may contain unfinished
candles. They must not be relabeled as closed-candle replay samples. Snapshots
are diagnostic context, not full historical replay inputs or configuration
provenance. Outcomes are stored separately by Phase 2b; missing outcomes
must never be treated as zero returns. Existing history inventory still reads
`signals` and remains blocked. No penalty setting is automatically changed.
Collection starts after deploying this version; existing signals are not
backfilled into the new table. Storage grows with distinct observed opportunities.

To inspect collection locally:

```sql
SELECT version, symbol, timeframe, COUNT(*) AS opportunities,
       COUNT(DISTINCT date(observed_at / 1000, 'unixepoch')) AS days
FROM htf_opportunities
GROUP BY version, symbol, timeframe;
```

## Phase 2b: forward outcomes and bucket performance

`GET /api/backtest/htf-performance?symbol=TEST&timeframe=1H` is a read-only
report with ten ATR deciles. It selects version-1 opportunities opposing the
**effective** HTF context, isolated by symbol and 1H/4H timeframe. Aligned,
unknown and Wyckoff-relaxed contexts, unknown ATR and future observations are
excluded. Counts include pending, missing-data and completed outcomes. Empty
averages are JSON null, while a genuinely flat gross return is numeric zero.
Dates in this endpoint are Unix milliseconds; distinct dates use UTC.

The immutable policy `first_available_open_5d_cost30bps_v1` uses:

- Entry: first stored same-timeframe candle open strictly after observation,
  at most seven calendar days later. The observation candle's price is not used.
- Exit: first stored same-timeframe candle open at least five calendar days
  after entry, within seven further calendar days. Weekends can extend holding.
- Both candles must have elapsed their nominal 1H/4H duration at calculation time.
- Gross return: directional price change relative to entry; SHORT reverses sign.
- Net return: gross return minus 0.30 percentage points, an illustrative round-trip
  assumption of 10 bps fee and 5 bps slippage per side. This is not venue-specific.
- No TP/SL, leverage, funding, borrow fees, sizing or portfolio compounding.

The pipeline processes up to 200 opportunities per tick with a ten-second
context timeout. Uncomputed records appear pending. Missing-data status applies
when the price window has elapsed or selected prices are invalid. Pending and
missing-data records retry no more often than hourly, ordered by oldest check;
backfilled prices can complete them. Completed results are frozen, so subsequent
OHLCV corrections require a separately versioned recalculation policy.

`htf_outcomes` stores policy, status, check time and, only on completion, entry/
exit times and prices plus gross/net returns. API reads never trigger writes.
Processing errors are logged and do not stop normal signal analysis.

These are descriptive **first-available-price forward returns**, not a trade
backtest. Missing intermediate market data can shift the selected entry/exit;
there is no market-calendar completeness guarantee. Opportunity samples may
overlap and are not independent. `history_sufficient` remains only the two-year /
30-distinct-observation-day sparsity check; it does not validate completed-outcome
coverage. `ready` remains false. Full replay, outcome-window purging and
chronological out-of-sample validation are still required before fitting penalties.

## Why automatic fitting is not yet safe

- The pipeline persists signals **after** HTF suppression and other filters.
  Suppressed opportunities are missing (selection bias).
- Stored HTF trend is the raw EMA context; the live filter can relax it through
  a Wyckoff override. An opposing raw trend does not prove that a penalty applied.
- Stored scores already contain adjustments; they cannot reconstruct original
  scores for replaying alternative penalties.
- Legacy signal forward-return zero conflates missing outcomes with genuine zero
  returns. Phase 2b resolves this for new opportunity outcomes only.
- The current single-timeframe backtest does not replay the full live multi-timeframe
  filtering pipeline. Varying its scores alone would not validate live behavior.

## Remaining work before closing #32

1. Extend the live pre-filter collection with closed-candle replay inputs,
   configuration provenance, candle-close timestamps and explicitly valid outcomes.
2. Replay identical closed-candle multi-timeframe rules and entry thresholds for
   candidate penalties; account for transaction costs and overlapping observations.
3. Split training/validation chronologically, purge overlapping outcome windows,
   and check minimum history/sample requirements separately per bucket and symbol.
4. Persist only validated, versioned candidates in YAML, expose review/activation
   in Settings, and retain the existing policy for missing, stale or invalid buckets.
5. Test explicit 0% and 100% behavior, rollback and drift before enabling application.

Local read-only inventory on 2026-09-12 found 1,425 stored signals and none with
both a valid LONG/SHORT HTF trend and a valid ATR percentile. This observation is
specific to the local database; it does not describe every installation.
