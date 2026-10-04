# DRAFT — Forex launch post copy

OANDA wording in these drafts describes an optional provider, not a universally
free API. Eligibility varies by region/account; direct readers to the Forex
guide for current details. Yahoo remains the keyless default.

These are local drafts for review only. Do not treat them as published posts.
The linked Forex recording is a sample-data interface demo, not a live provider
verification or release QA sign-off.

## r/Forex or r/ICTTrading

**Title:** A self-hosted ICT chart and signal toolkit now supports Forex

I’ve added Forex analysis to ChartNagari, an open-source research app for ICT,
SMC, Wyckoff and technical signals. The keyless setup uses Yahoo candles; the
chart includes New York based session context, and the UI includes currency
strength and DXY context. Backtests report pips and subtract a spread assumption.

Yahoo has no meaningful FX volume, and its gold/silver futures symbols are
proxies. OANDA is optional for candles and tick volume. Backtests omit swap,
commissions and slippage, and the app does not place FX orders. Feedback on data
alignment and useful FX rules is welcome.

Project: https://github.com/Ju571nK/ChartNagari
Guide: docs/FOREX.md
Sample interface demo: docs/assets/forex-demo.gif

## r/algotrading

**Title:** Adding FX to a self-hosted signal engine: provider quality, sessions,
and spread-aware pip stats

ChartNagari now accepts supported FX pairs for analysis and alerts. Yahoo is the
keyless provider; optional OANDA candles carry tick volume. The implementation
tracks volume quality so zero-volume Yahoo candles do not masquerade as observed
volume, and the FX backtest subtracts a round-trip spread assumption in pips.

This remains a research tool: swap, commissions and slippage are out of scope,
Yahoo metals are futures proxies, and no FX execution adapter is included. I’d
appreciate review of the assumptions and edge cases.

Project: https://github.com/Ju571nK/ChartNagari

## Show HN

**Title:** Show HN: ChartNagari — self-hosted ICT signals for Forex, crypto and
stocks

ChartNagari is a Go and React research workspace for charts, technical signals,
alerts and backtests. Forex support includes keyless Yahoo candles, optional
OANDA data, New York session context and spread-adjusted pips. The source and
setup guide are available at https://github.com/Ju571nK/ChartNagari.

Known limits: no FX order execution or swap modeling; Yahoo FX has no meaningful
volume and Yahoo metals are futures proxies. The linked recording uses sample
data and does not demonstrate a live provider connection.
