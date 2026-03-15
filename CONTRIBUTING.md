# Contributing to Chartter

Thank you for taking the time to contribute! This guide covers everything you need to get started.

---

## Development Setup

**Prerequisites:** Go 1.26+, Node.js 20+, Docker (optional)

```bash
git clone https://github.com/Ju571nK/Chatter.git
cd Chatter

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

## Adding a Rule

1. **Create the rule file** in `internal/methodology/<category>/my_rule.go`
   - Implement the `rule.Rule` interface (`ID()`, `Evaluate(candles []market.Candle) rule.Signal`)
   - Category examples: `ict`, `wyckoff`, `momentum`, `volume`

2. **Register in config** — add an entry to `config/rules.yaml`:
   ```yaml
   - id: my_rule
     category: momentum
     enabled: true
     params:
       period: 14
   ```

3. **Write tests** — add `my_rule_test.go` next to the rule file using table-driven tests.

4. **Verify** — `go test ./internal/methodology/...` must pass.

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

- All tests must pass: `go test ./...`
- New rule implementations require at least one table-driven test covering signal and no-signal cases
- Use the race detector locally before opening a PR: `go test -race ./...`
- Do **not** mock the database in integration tests — use an in-memory SQLite instance

---

## PR Checklist

Before opening a pull request, verify:

- [ ] `go test ./...` passes locally
- [ ] `go vet ./...` produces no warnings
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
