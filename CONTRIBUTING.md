# Contributing to ChartNagari

Thank you for taking the time to contribute!
This guide covers everything you need — whether you write Go code or not.

---

## Ways to Contribute (No Code Required)

You don't need to write a single line of Go to help ChartNagari grow:

- **Share chart screenshots** — If you spot an Order Block, FVG, or Wyckoff phase that
  ChartNagari missed or got wrong, open an issue with a screenshot. This is invaluable
  test-case data for improving detection accuracy.
- **Report false positives / false negatives** — Run the platform and see a signal that
  looks wrong? File a bug report with the symbol, timeframe, and what you observed.
- **Propose new rules** — In [GitHub Discussions → Rules & Methodology](../../discussions),
  describe an ICT or Wyckoff pattern you'd like to see automated. You don't need to
  implement it — just describe what the setup looks like and when it fires.
- **Translate the README** — The platform already supports `LLM_LANGUAGE: en | ko | ja`.
  A community translation of `README.md` into Korean or Japanese helps ICT traders who
  aren't comfortable reading English docs.

---

## Development Setup

**Prerequisites:** Go 1.26+, Node.js 20+, Docker (optional)

```bash
git clone https://github.com/Ju571nK/ChartNagari.git
cd ChartNagari

# Install Go dependencies
go mod download

# Install frontend dependencies
cd web && npm install && cd ..

# Copy and configure environment
cp .env.example .env
# Edit .env — see README for variable descriptions

# Start the backend (port 8080)
go run ./cmd/server

# Start the frontend dev server (port 5173, proxies API to 8080)
cd web && npm run dev
```

Or use the Makefile shortcuts:

```bash
make build-all   # build frontend then backend binary
make run         # build and start server
make test        # run all Go tests
```

---

## Project Structure

| Package | Purpose |
|---|---|
| `cmd/server/` | Entry point — wires all components together |
| `internal/api/` | HTTP server and REST handlers |
| `internal/collector/` | Data source connectors (Binance, Tiingo, Yahoo) |
| `internal/engine/` | Rule evaluation loop |
| `internal/methodology/` | Trading rule implementations (ICT, Wyckoff, general TA) |
| `internal/rule/` | Rule interface and registry |
| `internal/indicator/` | Technical indicator calculations |
| `internal/interpreter/` | Multi-timeframe signal scoring |
| `internal/pipeline/` | Orchestrates collector → engine → notifier |
| `internal/notifier/` | Telegram and Discord alert dispatch |
| `internal/calendar/` | Economic calendar fetcher (FMP/Finnhub) and pre-event alert watcher |
| `internal/wyckoff/` | Wyckoff phase analyzer — detects Markup/Accumulation/Distribution/Markdown/Ranging phases and Spring/Upthrust events from OHLCV bars |
| `internal/report/` | Daily summary report generation |
| `internal/history/` | Historical OHLCV storage |
| `internal/storage/` | SQLite persistence layer |
| `internal/backtest/` | Historical backtesting |
| `internal/paper/` | Paper trading simulation |
| `internal/llm/` | LLM provider abstraction (Anthropic, OpenAI, Groq, Gemini) |
| `internal/analyst/` | AI analysis layer |
| `internal/config/` | Configuration loading |
| `internal/market/` | Market session helpers |
| `config/` | YAML configuration files (rules, symbols, timeframes) |
| `web/` | TypeScript + React 18 + Vite frontend |

---

## Adding a New Rule — Step-by-Step

Every trading rule implements the `AnalysisRule` interface defined in
`internal/rule/interface.go`:

```go
type AnalysisRule interface {
    // Name returns the unique identifier for this rule (must match rules.yaml key).
    Name() string

    // RequiredIndicators returns the list of indicator keys this rule needs.
    // The engine will ensure these are computed before calling Analyze.
    RequiredIndicators() []string

    // Analyze evaluates the rule against the given context and returns a Signal.
    // Returns nil, nil when no signal condition is met (not an error).
    Analyze(ctx models.AnalysisContext) (*models.Signal, error)
}
```

### Step 1 — Create the rule file

Create `internal/methodology/<category>/my_rule.go`.
Category examples: `ict`, `wyckoff`, `momentum`, `volume`.

Use `internal/methodology/ict/order_block.go` as the reference implementation.

```go
package ict // or wyckoff, momentum, volume, etc.

import (
    "github.com/Ju571nK/Chatter/internal/rule"
    "github.com/Ju571nK/Chatter/pkg/models"
)

// Compile-time check: fails to build if any AnalysisRule method is missing.
// Add this line to every rule file — it catches interface drift instantly.
var _ rule.AnalysisRule = (*MyRule)(nil)

type MyRule struct{}

func (r *MyRule) Name() string { return "my_rule" }

func (r *MyRule) RequiredIndicators() []string {
    return nil // or []string{"RSI_14"} if indicators are needed
}

func (r *MyRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
    // ctx.Timeframes["1H"] returns []models.Candle for the 1H timeframe.
    // Return nil, nil when no signal is detected (not an error).
    bars, ok := ctx.Timeframes["1H"]
    if !ok || len(bars) < 5 {
        return nil, nil
    }

    // ... your detection logic ...

    return &models.Signal{
        RuleName:  r.Name(),
        Direction: "LONG", // or "SHORT"
        Score:     1.0,
    }, nil
}
```

> **Why the compile-time check?**
> Go only validates interface compliance at assignment sites. Without
> `var _ rule.AnalysisRule = (*MyRule)(nil)`, a rule with a missing method
> compiles silently and only panics at runtime. This one line turns that
> into a build error.

### Step 2 — Register in config

Add an entry to `config/rules.yaml`:

```yaml
- id: my_rule
  category: ict         # must match the package directory name
  enabled: true
  params:
    period: 14          # any rule-specific parameters
```

The `id` field must exactly match what `Name()` returns.

### Step 3 — Write tests

Add `my_rule_test.go` next to the rule file. Use table-driven tests:

```go
package ict

import (
    "testing"
    "github.com/Ju571nK/Chatter/pkg/models"
)

func TestMyRule(t *testing.T) {
    rule := &MyRule{}

    tests := []struct {
        name    string
        bars    []models.Candle
        wantSig bool
    }{
        {
            name:    "fires on setup",
            bars:    makeBullishCandles(10), // build your test fixture
            wantSig: true,
        },
        {
            name:    "no signal on flat market",
            bars:    makeFlatCandles(10),
            wantSig: false,
        },
        {
            name:    "too few bars",
            bars:    makeFlatCandles(2),
            wantSig: false,
        },
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            ctx := models.AnalysisContext{
                Timeframes: map[string][]models.Candle{"1H": tc.bars},
            }
            sig, err := rule.Analyze(ctx)
            if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
            if (sig != nil) != tc.wantSig {
                t.Errorf("got signal=%v, want signal=%v", sig != nil, tc.wantSig)
            }
        })
    }
}
```

### Step 4 — Verify locally

```bash
go build ./internal/methodology/...   # catches interface violations
go test ./internal/methodology/...    # must pass
go vet ./...                          # no warnings
```

The CI will run the same commands automatically when you open a PR that touches
`internal/methodology/`.

---

## Code Style

- **Formatting:** `gofmt` — run before committing (`go fmt ./...`)
- **Vetting:** `go vet ./...` — must produce no warnings
- **Logging:** Use `zerolog` structured logging; pass `context.Context` through the call stack
- **Error handling:** Wrap errors with `fmt.Errorf("...: %w", err)`; never swallow errors silently
- **No globals:** Prefer dependency injection over package-level variables
- **Frontend:** Follow the existing TypeScript patterns; no `any` types

---

## Tests

**Go (backend):**
- All tests must pass: `go test ./...`
- New rule implementations require at least one table-driven test covering signal and no-signal cases
- Use the race detector locally before opening a PR: `go test -race ./...`
- Do **not** mock the database in integration tests — use an in-memory SQLite instance

**Frontend (Vitest):**
- All frontend tests must pass: `cd web && npm test`
- New React components require Vitest tests using `@testing-library/react`
- Mock `fetch` via `globalThis.fetch = vi.fn()` — do not use `msw` or other interceptors
- Reference `web/src/OnboardingModal.test.tsx` for patterns (mock queue ordering, async `waitFor`, `act`)

---

## PR Checklist

Before opening a pull request, verify:

- [ ] `go test ./...` passes locally
- [ ] `cd web && npm test` passes locally (frontend Vitest suite)
- [ ] `go vet ./...` produces no warnings
- [ ] `var _ rule.AnalysisRule = (*MyRule)(nil)` added to rule file (if adding a rule)
- [ ] Rule registered in `config/rules.yaml` (if adding a rule)
- [ ] `cd web && npm run build` succeeds (if frontend was changed)
- [ ] `.env.example` is updated if a new environment variable was added
- [ ] `CHANGELOG.md` has a brief entry under the current version/date
- [ ] No `.env` file or real API keys are included in the diff

---

## Issue Reporting

- Search existing issues before opening a new one
- For bugs, include: OS, Go version, steps to reproduce, and relevant log output
- For feature requests, describe the use case and the trading methodology involved
- Security vulnerabilities: see [SECURITY.md](SECURITY.md) — do **not** open a public issue
