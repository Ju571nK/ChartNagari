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

## Why automatic fitting is not yet safe

- The pipeline persists signals **after** HTF suppression and other filters.
  Suppressed opportunities are missing (selection bias).
- Stored HTF trend is the raw EMA context; the live filter can relax it through
  a Wyckoff override. An opposing raw trend does not prove that a penalty applied.
- Stored scores already contain adjustments; they cannot reconstruct original
  scores for replaying alternative penalties.
- Forward-return zero conflates missing outcomes with genuine zero returns.
- The current single-timeframe backtest does not replay the full live multi-timeframe
  filtering pipeline. Varying its scores alone would not validate live behavior.

## Remaining work before closing #32

1. Collect versioned **pre-filter** opportunities with effective HTF context,
   original score, ATR bucket, candle-close timestamp and explicit outcome validity.
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
