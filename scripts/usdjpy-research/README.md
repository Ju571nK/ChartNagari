# Offline USDJPY research

These tools evaluate existing rule implementations without opening the application
DB, loading credentials, editing live settings or sending notifications. Run from
the repository root with Go and Python 3; Python uses the standard library only.

```sh
mkdir -p tmp/usdjpy-15m-research
curl --fail --location --user-agent 'Mozilla/5.0' \
  'https://query1.finance.yahoo.com/v8/finance/chart/USDJPY=X?interval=15m&range=60d' \
  --output tmp/usdjpy-15m-research/yahoo-15m.json
curl --fail --location --user-agent 'Mozilla/5.0' \
  'https://query1.finance.yahoo.com/v8/finance/chart/USDJPY=X?interval=1h&range=1y' \
  --output tmp/usdjpy-15m-research/yahoo-1y.json
go run ./scripts/usdjpy-research --input-15m tmp/usdjpy-15m-research/yahoo-15m.json \
  --input-1h tmp/usdjpy-15m-research/yahoo-1y.json
python3 scripts/usdjpy-research/analyze.py --input tmp/usdjpy-15m-research/replay.json \
  --output tmp/usdjpy-15m-research/comparison.json
python3 -m unittest discover -s scripts/usdjpy-research -p 'test_*.py'
```

The native Yahoo 15M response is limited to recent history (currently at most about 60 days); the 1Y hourly file supplies enough warmup for the aligned 4H comparison. All three timeframes share the 15M evaluation window. The replay uses a roughly five-hour maximum holding horizon: 20x15M, 5x1H and 2x4H bars. The 4H path is necessarily coarse. The Yahoo response is a point-in-time input, not a stable fixture. Save the
snapshot and hashes when comparing runs. Yahoo availability/history can change.
The earlier 1H/4H report is [2026-10-10](../../docs/backtests/20261010_usdjpy_backtest.md); the 15M/1H/4H comparison is [2026-10-14](../../docs/backtests/20261014_usdjpy_15m_comparison.md).

`main.go` accepts `--input-15m`, `--input-1h`, `--output` and `--rules`; `--input` remains as a legacy hourly-input alias. It requires USDJPY=X (Yahoo may return its JPY=X alias) quoted in JPY, drops null/incomplete candles, rejects malformed OHLC or unordered timestamps, rebuilds complete 4H candles with the application's New York alignment and calls the existing backtest engine. Rule registration and strength weights match the server at v2.15.0.0; future rule additions require this research registry to be updated. It uses the Forex methodology restriction, 200-bar warmup, TP 1.5 ATR, SL 1 ATR and a roughly five-hour horizon (20x15M, 5x1H, 2x4H bars). Rule YAML parameters other than activation, strength weights and asset classes retain the existing server behavior; this tool does not tune rule internals.

`analyze.py` independently verifies eligible TP-first/1-pip outcomes against the application, then evaluates TP-first and conservative SL-first policies with 1/2/3-pip round-trip spreads. It compares six fixed methodology groups and score thresholds 0/4/8/12/16. All timeframes use a common calendar interval and chronological 70/30 split. Training-boundary purge and final-bar exclusion use each timeframe's exit horizon (20x15M, 5x1H, 2x4H). Earlier bars remain available as indicator warmup.

There are two modes: overlapping individual-rule diagnostics and a one-position
proxy. The latter prioritizes higher scores at a candle close, admits no overlapping position, applies a per-rule 4-hour cooldown and a per-symbol cap of three entries per UTC day. It is not a replay of production notification delivery. Live MTF,
HTF, volatility/context adjustments, event handling and cross-timeframe alert
competition are not reproduced. Threshold 0 is exploratory and is below the
server's normal notification floor; do not copy it into live settings.

Candidates are ranked only on training net R, with at least 30 training trades.
Holdout results are not used to select a new configuration. Weekly block bootstrap
intervals use 10,000 resamples with seed 571 and include zero-trade calendar weeks.
These descriptive intervals do not eliminate multiple-comparison or model bias.

Net pips assume fixed quantity. R normalizes each outcome by its own initial
mid-price-to-SL distance; drawdown R uses cumulative fixed-risk outcomes, not
account equity or a leveraged percentage return. Raw overlapping-trade drawdown
must not be interpreted as portfolio drawdown. Spread changes execution PnL only,
matching the inherited mid-price TP/SL trigger convention. Swaps, commissions,
slippage, next-open entry gaps and broker-specific execution are not modeled.
