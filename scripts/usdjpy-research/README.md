# Offline USDJPY research

These tools evaluate existing rule implementations without opening the application
DB, loading credentials, editing live settings or sending notifications. Run from
the repository root with Go and Python 3; Python uses the standard library only.

```sh
mkdir -p tmp/usdjpy-research
curl --fail --location --user-agent 'Mozilla/5.0' \
  'https://query1.finance.yahoo.com/v8/finance/chart/USDJPY=X?interval=1h&range=1y' \
  --output tmp/usdjpy-research/yahoo-1y.json
go run ./scripts/usdjpy-research
python3 scripts/usdjpy-research/analyze.py
python3 -m unittest discover -s scripts/usdjpy-research -p 'test_*.py'
```

The Yahoo response is a point-in-time input, not a stable fixture. Save the
snapshot and hashes when comparing runs. Yahoo availability/history can change.
The report from 2026-10-10 is in
[docs/backtests](../../docs/backtests/20261010_usdjpy_backtest.md).

`main.go` accepts `--input`, `--output` and `--rules`. It requires the USDJPY=X instrument (Yahoo may return its JPY=X alias)
quoted in JPY, drops null/incomplete hourly candles, rejects malformed
OHLC or unordered timestamps, rebuilds complete 4H candles with the application's
New York alignment and calls the existing backtest engine. Rule registration and
strength weights match the server at v2.15.0.0; future rule additions require this
research registry to be updated. It uses the Forex methodology restriction,
200-bar warmup, TP 1.5 ATR, SL 1 ATR and a 20-bar exit horizon. Rule YAML parameters
other than activation, strength weights and asset classes have the same existing
server behavior; this tool does not claim to tune rule internals.

`analyze.py` independently verifies all eligible TP-first/1-pip outcomes against
the application, then evaluates TP-first and conservative SL-first policies with
1/2/3-pip round-trip spreads. It compares six fixed methodology groups and score
thresholds 0/4/8/12/16. Both timeframes use a common calendar interval, chronological
70/30 split and a 20-bar purge at the training boundary. The final 20 bars are not
eligible entry candles. Earlier bars remain available as indicator warmup.

There are two modes: overlapping individual-rule diagnostics and a one-position
proxy. The latter prioritizes higher scores at a candle close, admits no overlapping
position, applies a per-rule 4-hour cooldown and a per-symbol cap of three entries
per UTC day. It is not a replay of production notification delivery. Live MTF,
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
