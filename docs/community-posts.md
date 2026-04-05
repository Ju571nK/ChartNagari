# Community Post Drafts

## 1. r/algotrading

**Title:**
> I built an open-source ICT + Wyckoff signal detector in Go — self-hosted, real-time Telegram alerts, 14+ rules (no subscriptions, no cloud)

**Body:**
---
I've been studying ICT and Wyckoff methodology for a while and got frustrated that there were no good open-source tools for automating signal detection. The best ICT library on GitHub is a bag of Python functions — no UI, no alerts, no multi-timeframe. The best Wyckoff repo has 17 stars.

So I built one.

**ChartNagari** (Go + React, MIT) — [github.com/Ju571nK/ChartNagari](https://github.com/Ju571nK/ChartNagari)

**What it does:**
- Scans US stocks + crypto across 1W / 1D / 4H / 1H simultaneously
- Detects 14+ rules: ICT Order Blocks, Fair Value Gaps, Liquidity Sweeps, Wyckoff Spring/Upthrust, Accumulation/Distribution phases, RSI, MACD, etc.
- Multi-timeframe consensus scoring — signals ranked by how many timeframes agree
- Fires Telegram or Discord alerts with optional AI interpretation (Anthropic/OpenAI/Groq/Gemini)
- Backtest on historical data
- Runs entirely locally — one `docker compose up`, no cloud accounts needed

**Why Go?** Fast enough to scan 50+ symbols across 4 timeframes in parallel without breaking a sweat. The rule interface is simple enough that adding a new signal is ~50 lines.

**Current signal categories:**
- ICT: Order Blocks, FVG, Breaker Blocks (coming soon), OTE
- Wyckoff: Spring, Upthrust, Accumulation Phase, Distribution Phase
- TA: RSI divergence, MACD cross, EMA cross, volume

**Setup (Docker):**
```bash
git clone https://github.com/Ju571nK/ChartNagari.git
cd ChartNagari
cp .env.example .env
# add your Telegram token + chat ID
docker compose up -d
# open http://localhost:8080
```

The project has a `CONTRIBUTING.md` with good-first-issue labels if you want to add a rule — the interface is designed to be easy to extend.

Happy to answer questions about the architecture or the signal logic.

---

**Suggested subreddits to also post:**
- r/algotrading ← primary
- r/stocks (mention the screener angle)
- r/golang (Go + architecture angle)
- r/selfhosted (self-hosted, local-first angle)

---

## 2. r/golang

**Title:**
> Show r/golang: I built a real-time stock/crypto signal engine in Go — multi-timeframe, plugin-style rule interface, 14+ TA rules

**Body:**
---
Been working on a project that might interest Go folks — it's a technical analysis signal detector that was a good excuse to explore some Go patterns I wanted to experiment with.

**ChartNagari** — [github.com/Ju571nK/ChartNagari](https://github.com/Ju571nK/ChartNagari)

**The interesting Go bits:**
- Rule evaluation uses a simple interface (`AnalysisRule`) — anyone can implement a new rule in ~50 lines with no framework knowledge required
- Multi-timeframe scanning runs concurrent goroutines per symbol/timeframe; signals are aggregated and scored
- Context propagation throughout; zerolog for structured logging
- SQLite via `modernc.org/sqlite` (pure Go, no CGo dependency)
- CI automatically checks that every interface method is documented in CONTRIBUTING.md — the `check-docs` job extracts method names from the interface file with `awk` and greps the docs

**Rule interface:**
```go
type AnalysisRule interface {
    ID() string
    Name() string
    Category() string
    Evaluate(candles []Candle, params map[string]float64) (*Signal, error)
    DefaultParams() map[string]float64
    RequiredCandles() int
}
```

Adding a new indicator is: implement the interface → register in `config/rules.yaml` → add table-driven tests.

**Stack:** Go 1.26 backend, TypeScript/React/Vite frontend, SQLite, Docker.

Open to feedback on the architecture, especially the multi-timeframe scoring — that part feels a bit overengineered and I'd love a second opinion.

---

## 3. Show HN (Hacker News)

**Prerequisites:** HN account must have submit access (currently restricted for new accounts — need more comment activity first)
**Post timing:** Mon-Thu, US East 9-11am (KST 10pm-midnight)

**Title:**
> Show HN: ChartNagari -- Open-source ICT/Wyckoff signal detector, self-hosted, Go + React

**URL:** https://github.com/Ju571nK/ChartNagari

**First comment (post immediately after submitting):**
---
I spent months looking for an open-source tool that could automatically detect ICT (Inner Circle Trader) and Wyckoff methodology signals across multiple timeframes. Nothing good existed, so I built one.

It runs locally with `docker compose up`. Scans stocks + crypto across 1W/1D/4H/1H simultaneously, detects 33 signal rules, and fires Telegram/Discord alerts.

What makes it different from a typical screener:

- Signal quality scoring — not all signals are equal. Liquidity sweeps are scored by volume ratio, wick depth, and reversal strength. FVGs are scored by gap size vs ATR and impulse strength. Low-quality noise gets filtered before it reaches you.

- Top-down HTF context filter — a 1H LONG signal gets penalized if the daily trend is clearly bearish. Uses ADX for trend strength rather than binary EMA crossover.

- Signal sequence tracking — a liquidity sweep followed by displacement in the same direction gets a bonus score. The system tracks multi-signal patterns, not just isolated events.

- Volatility regime detection — ATR percentile ranking classifies markets as low/normal/high vol. Scoring adjusts automatically.

- Per-symbol profiles — crypto gets ICT+SMC rules, large caps get Wyckoff+TA, small caps get candlestick+volume only.

Signal categories: ICT (Order Blocks, FVG, Liquidity Sweeps, Breaker Blocks, OTE, AMD Session), Wyckoff (Spring, Upthrust, Accumulation/Distribution), SMC (BOS, CHoCH), General TA (RSI, EMA, volume, VSA), and 14 candlestick patterns.

Tech: Go 1.26, React 18 + Vite, SQLite, MIT license. Optional AI interpretation via Anthropic/OpenAI/Groq/Gemini.

Would love feedback on:
1. The multi-timeframe scoring is still heuristic-based. Anyone approached this more rigorously?
2. Is automating ICT/Wyckoff concepts even worth it, or is the subjectivity too high?

---

**HN posting tips:**
- Post Monday–Thursday, 9–11am US East time (9–11am KST is midnight ET — better to schedule)
- "Show HN" posts get most traction with a genuine question at the end
- Respond to every early comment within the first hour — HN rewards active threads
- Don't post the same day as a big tech news cycle

---

## 4. r/selfhosted

**Flair:** New Project Friday
**Post date:** Friday only (Rule 6 — project < 3 months old, first commit 2026-03-07)
**Target date:** 2026-04-10 (Friday)

**Title:**
> Self-hosted stock/crypto signal detector — ICT + Wyckoff rules, Telegram alerts, runs locally (Go + SQLite, Docker)

**Body:**
---
I wanted a trading signal scanner that ran locally — no subscriptions, no cloud, no data leaving my machine. Built one.

**ChartNagari** — [github.com/Ju571nK/ChartNagari](https://github.com/Ju571nK/ChartNagari)

**Self-hosted highlights:**
- Single `docker compose up` — no external services required
- Data: Binance WebSocket (crypto, free), Yahoo Finance (stocks, free fallback), Tiingo (stocks, better free tier)
- SQLite database — no Postgres, no Redis
- ~50MB RAM idle; runs fine on a Pi 4
- Everything in `.env` — no web UI required to configure (but there is one)

**What it does:** scans your watchlist across 1W/1D/4H/1H, fires Telegram/Discord alerts when 33 signal rules fire (ICT Order Blocks, Wyckoff Spring, RSI divergence, candlestick patterns, etc.)

**Recent additions:**
- Signal quality scoring — not all signals are equal, each is scored by volume, wick depth, reversal strength
- Per-symbol signal profiles — different rules for crypto vs large cap vs small cap
- Chart overlay toggles — FVG zones, Order Block zones, Wyckoff phases on/off
- Volatility regime detection — ATR percentile bucketing adjusts scoring in quiet vs active markets
- Settings UI for tuning all parameters without touching config files

Optional: plug in an LLM API key (Anthropic/OpenAI/Groq/Gemini) for AI commentary on signals. Entirely optional — the core engine runs without it.

Go backend, React frontend, MIT licensed. Docker image included.

---

## 5. best-of-algorithmic-trading (README PR)

See: `docs/best-of-algotrading-pr.md`
