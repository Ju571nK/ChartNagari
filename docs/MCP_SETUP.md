# ChartNagari MCP Server Setup

ChartNagari exposes a local MCP (Model Context Protocol) endpoint so Claude Desktop, Claude Code, and Codex CLI can query your pre-computed chart analysis directly.

## Why use MCP?

A typical "analyze my 10 watchlist symbols" workflow through external fetch burns ~40,000 tokens just retrieving OHLCV data. Pulling the same data via MCP from your local ChartNagari returns pre-computed multi-timeframe analysis — markdown tables ready for the LLM to reason about — using ~6,000 tokens total (an **~85% saving**).

## Requirements

- ChartNagari running on `localhost:8080` (default)
- `API_TOKEN` set in Settings (shown in `.env` as `API_TOKEN=...`)

## Client setup

### Claude Code (HTTP transport — recommended)

```bash
claude mcp add --transport http chartnagari \
  http://localhost:8080/api/mcp \
  --header "Authorization: Bearer $API_TOKEN"
```

Verify:

```bash
claude mcp list
# Should show: chartnagari (http, active)
```

### Claude Desktop (HTTP transport)

Edit `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS) or `%APPDATA%\Claude\claude_desktop_config.json` (Windows):

```json
{
  "mcpServers": {
    "chartnagari": {
      "type": "http",
      "url": "http://localhost:8080/api/mcp",
      "headers": {
        "Authorization": "Bearer <your API_TOKEN>"
      }
    }
  }
}
```

Restart Claude Desktop.

### Codex CLI (stdio bridge)

Install the bridge binary:

```bash
go install github.com/Ju571nK/Chatter/cmd/chartnagari-mcp@latest
# or from source:
cd ~/ChartNagari && go build -o /usr/local/bin/chartnagari-mcp ./cmd/chartnagari-mcp
```

Edit `~/.codex/config.toml`:

```toml
[[mcp_servers]]
name = "chartnagari"
command = "chartnagari-mcp"

[mcp_servers.env]
CHARTNAGARI_URL = "http://localhost:8080"
CHARTNAGARI_TOKEN = "<your API_TOKEN>"
```

### Any stdio MCP client

Point it at the `chartnagari-mcp` binary with the same two env vars:

- `CHARTNAGARI_URL` (default `http://localhost:8080`)
- `CHARTNAGARI_TOKEN` (required if `API_TOKEN` is set on the server)

## Available tools (v2)

| Tool | Purpose |
|------|---------|
| `list_watchlist` | All symbols tracked, enabled/disabled |
| `get_analysis` | Multi-timeframe analysis for a symbol (fired rules, MTF score, key levels) |
| `get_signal_history` | Recent alerts for a symbol |
| `get_ohlcv` | Raw candles (fallback — prefer `get_analysis`) |
| `get_economic_calendar` | Economic events in a date range |
| `get_my_performance` | Personal trading performance from user-marked alerts (Took/Skipped/Win/Loss/BE counts and hit rate by rule/symbol/methodology/timeframe) |

## Example Claude Code conversation

```
You: 이번 주 CPI 발표가 내 관심종목에 어떻게 영향 미칠까?

Claude: [calls list_watchlist]
       [calls get_economic_calendar with start/end covering this week]
       [for each symbol, calls get_analysis]

       Based on your 10 tracked symbols:
       - BTCUSDT: 1D shows LONG bias (score 14.5) with ICT bullish
         order block at 57800. If CPI prints below forecast (3.2),
         the order block will likely hold...
```

## Troubleshooting

### 401 Unauthorized
- Token mismatch. Check `CHARTNAGARI_TOKEN` env matches ChartNagari's `API_TOKEN`.
- In Claude Desktop config, verify `"Authorization": "Bearer <token>"` (with space, no extra quotes).

### Connection refused
- ChartNagari server not running. Start with `./restart.sh` (or your usual command).
- Port 8080 in use by another process. `lsof -i :8080` to check.

### Session expired — reinitialize
- MCP session idle for > 30 min. Clients auto-reinitialize; if not, restart the client.

### Codex bridge prints "ChartNagari not reachable"
- Verify `CHARTNAGARI_URL` in `~/.codex/config.toml`.
- Confirm `curl http://localhost:8080/api/status` from the same shell.

### "unknown tool" error
- Server version mismatch (v2 ships 6 tools; older server has fewer). Upgrade with `git pull && ./restart.sh`.

## Security notes

- MCP endpoint is `127.0.0.1`-bound by default — not reachable from external networks.
- Uses the same `API_TOKEN` as other ChartNagari protected endpoints.
- Rotating `API_TOKEN` (via Settings) invalidates all MCP clients — they'll return 401 until reconfigured.

## Uninstall

### Claude Code
```bash
claude mcp remove chartnagari
```

### Claude Desktop
Delete the `chartnagari` entry from `claude_desktop_config.json` and restart.

### Codex CLI
Delete the `[[mcp_servers]]` block in `~/.codex/config.toml`. Optionally remove the bridge binary: `rm /usr/local/bin/chartnagari-mcp`.
