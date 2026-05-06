# ChartNagari MCP Server — Design Spec

**Status:** Draft v1
**Date:** 2026-04-22
**Target version:** 2.7.0.0

## Problem

ChartNagari runs locally and already computes multi-timeframe chart analysis (ICT, Wyckoff, SMC, general TA) for a user's watchlist. Users running Claude Desktop / Claude Code / Codex CLI for daily trading research currently have no efficient way to inject this pre-computed analysis into LLM sessions. They either paste screenshots, describe levels manually, or ask the LLM to fetch raw OHLCV from external tools — all of which burn large amounts of tokens on data retrieval rather than reasoning.

## Goal

Expose ChartNagari as a local MCP (Model Context Protocol) server so LLM clients can query pre-computed chart analysis directly. The target is a **daily workflow**: user opens Claude/Codex, asks "analyze my watchlist in light of this news", and the LLM pulls structured analysis for 10 symbols × 4 timeframes in seconds using 80%+ fewer tokens than external-fetch alternatives.

## Non-goals (v1)

- **Not a public API.** Local-only. Bind to `127.0.0.1`. No CORS, no rate limiting beyond sanity caps.
- **Not a write surface.** No `add_symbol`, `run_backtest`, `place_order` tools. Read-only v1.
- **Not a batch orchestrator.** No dedicated `analyze_watchlist()` tool — LLM uses `list_watchlist` + N calls to `get_analysis`. Token savings from a batch tool were measured at ~5%, not worth the surface area.
- **Not auto-installed.** User explicitly configures in their MCP client's config file. Settings UI helps with copy-paste.
- **Not deep historical.** 3-year pattern mining is out of scope; `get_ohlcv` returns up to 500 bars (configurable later).

## Architecture

Three-layer design with strict separation:

```
┌─────────────────────┐           ┌──────────────────────┐
│  Claude Desktop     │           │  Codex CLI           │
│  Claude Code        │           │  (stdio-only client) │
│  (HTTP streamable)  │           │                      │
└──────────┬──────────┘           └──────────┬───────────┘
           │ HTTP POST                       │ stdio (JSON-RPC)
           │ Authorization: Bearer <TOKEN>    │
           │                                 ▼
           │                     ┌────────────────────────┐
           │                     │ chartnagari-mcp binary │
           │                     │ (stdio ↔ HTTP 번역기)    │
           │                     │ env:                   │
           │                     │   CHARTNAGARI_URL       │
           │                     │   CHARTNAGARI_TOKEN     │
           │                     └───────────┬────────────┘
           │                                 │ HTTP POST (forward)
           ▼                                 ▼
┌──────────────────────────────────────────────────────┐
│           ChartNagari 서버 (기존 프로세스)              │
│  ┌─────────────────────────────────────────────┐     │
│  │ internal/api/mcp_handler.go                 │     │
│  │  - POST /api/mcp (streamable HTTP)          │     │
│  │  - requireBearer 인증                        │     │
│  │  - MCP 세션 관리                              │     │
│  └─────────────────────┬───────────────────────┘     │
│  ┌─────────────────────▼───────────────────────┐     │
│  │ internal/mcp/registry.go                    │     │
│  │  - 툴 등록 + 스키마 검증                       │     │
│  └─────────────────────┬───────────────────────┘     │
│  ┌─────────────────────▼───────────────────────┐     │
│  │ internal/mcp/tools.go                       │     │
│  │  list_watchlist, get_analysis,              │     │
│  │  get_signal_history, get_ohlcv,             │     │
│  │  get_economic_calendar                      │     │
│  └─────────────────────────────────────────────┘     │
└──────────────────────────────────────────────────────┘
```

### Key decisions

| Dimension | Choice | Rationale |
|---|---|---|
| Transport | HTTP streamable + stdio bridge | Support both modern clients (HTTP) and stdio-only clients (Codex) with one source of truth |
| Auth | Reuse existing `API_TOKEN` | Matches Ollama/Execution endpoints. No new permission surface. |
| MCP SDK | `github.com/modelcontextprotocol/go-sdk` (공식) | Follows spec changes. Hand-roll too risky for MCP's scope. |
| Session model | Stateful per MCP spec | `Mcp-Session-Id` header. 30 min idle timeout. |
| Tool count (v1) | 5 | `list_watchlist`, `get_analysis`, `get_signal_history`, `get_ohlcv`, `get_economic_calendar` |
| Response format | Markdown for 4 tools, JSON for `get_ohlcv` | Markdown is LLM-native for tabular data; JSON better for 50-row OHLCV arrays |
| Bind | `127.0.0.1` only | Local-only. No exposure risk. |

### File structure

```
ChartNagari/
├── internal/mcp/                     # 새 패키지 (비즈니스 로직)
│   ├── registry.go                   # 툴 등록/조회/디스패치
│   ├── schemas.go                    # JSON 스키마 상수
│   ├── tools.go                      # 5개 툴 핸들러
│   ├── format.go                     # markdown/JSON 포매터 헬퍼
│   └── *_test.go
│
├── internal/api/
│   └── mcp_handler.go                # 새 파일: HTTP streamable 엔드포인트
│                                     # requireBearer + 세션 관리 + dispatch
│
└── cmd/chartnagari-mcp/              # 새 바이너리 (stdio 브릿지)
    ├── main.go                       # stdio JSON-RPC ↔ HTTP forward
    └── main_test.go
```

## Tool contracts

### 1. `list_watchlist`

- **Description (LLM 용):** "List all symbols currently tracked by ChartNagari. Use when user asks about their watchlist or which symbols they are tracking."
- **Input:** none
- **Output format:** Markdown table (~100 tokens)

```markdown
**Watchlist (3 symbols, 3 enabled)**

| Symbol | Exchange | Class | Enabled |
|--------|----------|-------|---------|
| BTCUSDT | BINANCE | crypto | ✓ |
| ETHUSDT | BINANCE | crypto | ✓ |
| AAPL | NASDAQ | stock | ✓ |
```

### 2. `get_analysis(symbol)`

- **Description:** "Get current multi-timeframe analysis for a symbol: fired rules, MTF score, direction, key support/resistance. Returns all 4 timeframes (1W/1D/4H/1H). Prefer this over get_ohlcv for pattern questions — it is pre-computed and much more token-efficient."
- **Input:** `{symbol: string}` (required)
- **Output format:** Markdown (~230 tokens)

```markdown
**BTCUSDT** · $58,432.10 · 2026-04-22 10:00 UTC

| TF | Dir | Score | Rules |
|----|-----|-------|-------|
| 1W | LONG | 12.0 | wyckoff.accumulation_phase_C (spring) |
| 1D | LONG | 14.5 | ict.order_block_bullish (level 57800, 2 bars old) |
| 4H | LONG | 11.0 | ta.macd_bullish_cross |
| 1H | NEUTRAL | 3.0 | — |

**Support:** 57800, 57200 · **Resistance:** 58900, 59500
```

Errors:
- Symbol not in watchlist → `-32602 Invalid params`, `data.hint: "Call list_watchlist to see available symbols"`

### 3. `get_signal_history(symbol, since?, limit?)`

- **Description:** "Get recent alert history for a symbol — rules that fired above alert threshold. Default: last 7 days, 50 items."
- **Input:**
  - `symbol: string` (required)
  - `since: string` (ISO 8601, optional, default 7 days ago)
  - `limit: integer` (default 50, max 200)
- **Output format:** Markdown table (~200 tokens for 5 alerts)

```markdown
**BTCUSDT · 5 alerts in last 7 days**

| Time (UTC) | TF | Dir | Score | Rules |
|------------|----|----|-------|-------|
| 2026-04-20 14:30 | 1H | LONG | 14.0 | ict.order_block_bullish, ta.rsi_oversold |
| 2026-04-18 09:00 | 4H | SHORT | 11.5 | wyckoff.distribution_phase_D |
```

### 4. `get_ohlcv(symbol, timeframe, limit?)`

- **Description:** "Get raw OHLCV candles. Use ONLY when you need to analyze raw price action yourself — prefer get_analysis for pattern detection as it is pre-computed and more token-efficient."
- **Input:**
  - `symbol: string` (required)
  - `timeframe: enum ['1W','1D','4H','1H']` (required)
  - `limit: integer` (default 50, max 500)
- **Output format:** JSON (~500 tokens for 50 candles)

```json
{
  "symbol": "BTCUSDT",
  "tf": "1H",
  "candles": [
    {"t":"2026-04-22T10:00:00Z","o":58500,"h":58600,"l":58400,"c":58432,"v":123.45}
  ]
}
```

### 5. `get_economic_calendar(start, end, impact_min?)`

- **Description:** "Get economic events (FOMC, CPI, employment, earnings) in a time range. Use for news/macro context questions."
- **Input:**
  - `start: string` (ISO 8601, required)
  - `end: string` (ISO 8601, required)
  - `impact_min: enum ['low','medium','high']` (default `medium`)
- **Output format:** Markdown table (~200 tokens for 5 events)

```markdown
**Economic events · 2026-04-22 to 2026-04-29 · impact ≥ medium**

| Time (UTC) | Event | Impact | Actual | Forecast | Previous |
|------------|-------|--------|--------|----------|----------|
| 04-23 12:30 | US CPI YoY | high | — | 3.2 | 3.4 |
| 04-24 18:00 | FOMC Rate Decision | high | — | 5.25 | 5.25 |
```

### Tool design rules

- Abbreviated keys in tables (`TF`, `Dir`) — token savings, context makes meaning unambiguous
- `—` for empty values (readable for both LLM and human)
- UTC timestamps, ISO 8601 format
- `Description` field is load-bearing: includes "Use X when..." and "Prefer X over Y" guidance for LLM to choose correctly
- `null`/empty arrays never dropped silently — LLM must see that data was requested and nothing returned

## Error handling

### JSON-RPC errors (MCP protocol layer)

| Scenario | Code | HTTP | Example message |
|---|---|---|---|
| Invalid JSON | `-32700 Parse error` | 400 | "invalid JSON-RPC request" |
| Unknown method | `-32601 Method not found` | 404 | "unknown tool: get_xyz" |
| Invalid params | `-32602 Invalid params` | 400 | "symbol required" |
| Internal error | `-32603 Internal error` | 500 | "database error" |

### Auth errors

| Scenario | HTTP | Body |
|---|---|---|
| Missing/bad Authorization | 401 | `{"error":"unauthorized"}` |

Matches existing Ollama/Execution pattern exactly.

### Tool-level errors

All tool errors include `data.hint` to help LLM self-recover:

```json
{
  "error": {
    "code": -32602,
    "message": "symbol 'INVALID' not found in watchlist",
    "data": { "hint": "Call list_watchlist to see available symbols" }
  }
}
```

### stdio bridge error translation

| Upstream HTTP | stdio response |
|---|---|
| 401 | `-32603 Internal error, message: "unauthorized — check CHARTNAGARI_TOKEN"` |
| 503 | `-32603`, message: "ChartNagari server not reachable at CHARTNAGARI_URL" |
| Network timeout | `-32603`, message: "timeout contacting ChartNagari server" |
| Connection refused | `-32603`, message: "ChartNagari server not running" |

## Session management

Stateful per MCP spec. Streamable HTTP protocol:

1. Client sends `initialize` request **without** `Mcp-Session-Id` header
2. Server generates UUID, returns it in response header `Mcp-Session-Id`
3. All subsequent requests must include that header
4. Session state (minimal): client capabilities + last-active timestamp
5. Server-side idle timeout: 30 minutes → session evicted, next request needs new `initialize`

Concurrent sessions: yes (multiple MCP clients can connect simultaneously). Session storage: in-memory map with `sync.RWMutex`. No persistence required.

## Security

- Bind: `127.0.0.1` only (reuses existing ChartNagari config — SERVER_HOST default is already `127.0.0.1`)
- Auth: `requireBearer` middleware, identical to Ollama handlers
- No new attack surface beyond existing REST API (MCP is a second consumer of the same data)
- `http.MaxBytesReader` caps request body at 1 MiB per request (DoS protection)
- Secret token never logged (enforced by existing zerolog conventions)
- `panic` recovery middleware ensures a bad tool call cannot take down the ChartNagari process

## Logging

Using existing zerolog structured log conventions:

| Level | When |
|---|---|
| INFO | Session initialize, tool call start/end (with duration_ms), session eviction |
| WARN | 401 auth failure, malformed request, session timeout, unknown tool |
| ERROR | Panic recovery, database errors, unexpected internal failures |

Structured fields: `session_id` (truncated to first 8 chars), `tool_name`, `duration_ms`, `err`.

## Installation and configuration UX

### Binary installation (for stdio mode)

```bash
go install github.com/Ju571nK/Chatter/cmd/chartnagari-mcp@latest
```

v2 will ship pre-built binaries; v1 relies on Go install.

### Claude Code (HTTP streamable, recommended)

```bash
claude mcp add --transport http chartnagari \
  http://localhost:8080/api/mcp \
  --header "Authorization: Bearer $CHARTNAGARI_TOKEN"
```

### Claude Desktop (HTTP streamable)

Edit `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "chartnagari": {
      "type": "http",
      "url": "http://localhost:8080/api/mcp",
      "headers": { "Authorization": "Bearer <API_TOKEN>" }
    }
  }
}
```

### Codex CLI (stdio bridge)

Edit `~/.codex/config.toml`:

```toml
[[mcp_servers]]
name = "chartnagari"
command = "chartnagari-mcp"

[mcp_servers.env]
CHARTNAGARI_URL = "http://localhost:8080"
CHARTNAGARI_TOKEN = "<API_TOKEN>"
```

### Generic stdio client

Any MCP client that speaks stdio works with `chartnagari-mcp` + two env vars:
- `CHARTNAGARI_URL` (default `http://localhost:8080`)
- `CHARTNAGARI_TOKEN` (required if server has `API_TOKEN` set)

### Settings UI

New "MCP server" section in Settings tab (after Ollama):

- Shows HTTP endpoint URL and active status
- Lists registered tools (with descriptions on hover)
- Generates copy-paste config snippets for Claude Desktop / Claude Code / Codex CLI using the currently-configured token
- Token is masked (same style as Ollama section) until click-to-reveal

~150 lines of React + i18n entries.

### User documentation

`docs/MCP_SETUP.md`:

1. Why use MCP (token-saving math with concrete numbers)
2. Three client integrations (copy-paste ready)
3. Troubleshooting (401, connection refused, session timeout)
4. Example conversations showing tool invocations
5. Limitations (server must be running, port 8080 must not conflict)

## Testing strategy

### Layers

| Layer | Location | Style | Goal |
|---|---|---|---|
| Unit | `internal/mcp/tools_test.go` | Interface-mocked storage/rule engine | Tool logic correctness |
| Schema | `internal/mcp/schemas_test.go` | Validate tool output matches registered schema | Contract stability |
| HTTP integration | `internal/api/mcp_handler_test.go` | `httptest.NewServer` + full MCP handshake | Session/auth/error paths |
| stdio bridge | `cmd/chartnagari-mcp/main_test.go` | Fake upstream (httptest) + pipe stdio | Translation correctness |
| E2E | Manual checklist | Real Claude Code / Codex round-trip | Pre-release verification |

### Must-have unit test cases per tool

- `list_watchlist`: mixed enabled/disabled, empty watchlist
- `get_analysis`: full 4-TF data, partial TF data, unknown symbol, no rules fired
- `get_signal_history`: default `since`, `limit` clamping, no alerts
- `get_ohlcv`: invalid `timeframe`, `limit` clamping, data shortage
- `get_economic_calendar`: `start > end`, `impact_min` filtering, empty range

### Must-have HTTP integration tests

- `initialize` → capabilities + `Mcp-Session-Id` issued
- `tools/list` with valid session → 5 tools
- `tools/call` per tool → correct output shape
- Missing session → auto-create or error per spec
- Missing bearer → 401
- Bad bearer → 401
- Oversized body → 413 (`MaxBytesReader`)
- Panic in handler → recovered, 500 response, process alive

### Must-have stdio bridge tests

- Upstream 200 → pass through to stdio
- Upstream 401 → translated `-32603` with clear message
- Upstream connection refused → translated error
- stdin EOF → graceful shutdown
- Missing `CHARTNAGARI_URL` → startup error

### Coverage targets

- `internal/mcp/`: 90%+
- `internal/api/mcp_handler.go`: 80%+
- `cmd/chartnagari-mcp/`: 70%+

### E2E manual checklist (pre-merge)

```
[ ] ChartNagari 서버 실행
[ ] Claude Code `mcp add` 등록 성공
[ ] "내 관심종목 보여줘" → list_watchlist 호출 확인
[ ] "BTCUSDT 분석" → get_analysis markdown 표 렌더
[ ] "지난 3일 BTCUSDT 알림" → get_signal_history
[ ] "BTCUSDT 1H 최근 100캔들" → get_ohlcv JSON
[ ] "이번 주 경제 이벤트" → get_economic_calendar
[ ] Codex CLI stdio 브릿지 등록 → 같은 5개 툴 확인
[ ] ChartNagari 서버 중지 → Claude에서 호출 → 명확한 에러
[ ] API_TOKEN 잘못 → 401 → Claude가 이해 가능한 에러 수신
[ ] 토큰 절감 실측: "10종목 분석" 쿼리 vs 외부 fetch → 70%+ 절감 확인
```

## Phase breakdown (구현 플랜 예고)

- **Phase A — 백엔드 코어:** `internal/mcp/` 패키지, 5개 툴 핸들러, 단위 + 스키마 테스트
- **Phase B — HTTP 전송:** `internal/api/mcp_handler.go`, streamable HTTP 엔드포인트, requireBearer, 세션 관리, 통합 테스트, `cmd/server/main.go` 와이어링
- **Phase C — stdio 브릿지:** `cmd/chartnagari-mcp/` 바이너리, stdio↔HTTP 번역, 에러 전파, 통합 테스트
- **Phase D — UI + 문서 + 릴리즈:** Settings 탭 MCP 섹션, `docs/MCP_SETUP.md`, CHANGELOG, VERSION 2.7.0.0

Each phase is independently testable and mergeable — suited for subagent-driven development.

## Risks

| Risk | Mitigation |
|---|---|
| MCP 공식 Go SDK 스펙 변화 | Phase A 시작 전 SDK 의존성 추가 + 핸드셰이크 테스트 커밋 1개로 검증. 문제 시 `mark3labs/mcp-go`로 fallback. |
| Claude Desktop/Code/Codex MCP 클라이언트 구현 차이 | E2E 체크리스트로 릴리즈 전 모두 검증 |
| 포트 충돌 (8080 공유) | 동일 프로세스에 엔드포인트 추가 — 충돌 없음 |
| `analyze_watchlist` 없이 실제 UX가 불편할 가능성 | v1.1 팔로업으로 쉽게 추가 가능. v1 출시 후 실사용 피드백 수집 |

## Success criteria

1. 5개 툴 모두 Claude Code + Claude Desktop + Codex CLI에서 작동
2. `go test ./... -race` 전부 통과
3. 토큰 실측: "내 관심종목 10개 분석" 시나리오에서 외부 fetch 대비 **≥70% 절감** 검증
4. 신규 사용자가 `docs/MCP_SETUP.md` 따라가며 **15분 내** 작동 환경 완성
