# Rakuten FX (MT4 demo) bridge — read-only foundation

Rakuten Securities does not offer a public FX API. Automated access works through
its MT4 platform (楽天MT4) and an Expert Advisor (EA). This plugin is the base for
that connection, built on the [order plugin v1 kit](../../docs/order-plugins/protocol.md):

```
Rakuten MT4 terminal (demo server)
  └─ ChartNagariBridge.mq4 ──POST /bridge/v1/snapshot (bridge token)──▶ plugin-rakutenfx (127.0.0.1:9300)
                                                                         ▲
ChartNagari server / cmd/plugin-check ──signed GET /v1/* (HMAC)──────────┘
```

**Current scope:** account, free margin and order snapshots from a **demo** account only.
The EA never trades, and the plugin's `submit`/`cancel` capabilities and `orders_enabled`
are all false. Snapshots are refused, and stored data is dropped, when the terminal reports
a live account or a server other than `demo_server`. Data older than 60 seconds is
reported as `502 broker_unavailable`.

## Setup

1. Open a Rakuten MT4 demo account and log in to server `RakutenSecurities-Demo`.
2. Configure and start the plugin from the repository root:

   ```bash
   cp cmd/plugin-rakutenfx/config.example.yaml cmd/plugin-rakutenfx/config.local.yaml
   chmod 600 cmd/plugin-rakutenfx/config.local.yaml
   openssl rand -hex 32   # plugin_secret
   openssl rand -hex 32   # bridge_token (must differ)
   go run ./cmd/plugin-rakutenfx -config cmd/plugin-rakutenfx/config.local.yaml
   ```

3. In MT4: copy `ChartNagariBridge.mq4` into `MQL4/Experts`, compile it in MetaEditor,
   add `http://127.0.0.1:9300` under **Tools → Options → Expert Advisors → Allow WebRequest
   for listed URL**, then attach the EA to any chart with `BridgeToken` set.
   If WebRequest fails on the non-standard port (error 4014/4060 in the Experts log),
   try `listen: "127.0.0.1:80"` and the matching URL.
4. Verify without writing anything:

   ```bash
   # plugin-check.local.yaml: url http://127.0.0.1:9300, plugin_id rakuten-fx-demo, plugin_secret ...
   go run ./cmd/plugin-check -config plugin-check.local.yaml
   ```

To inspect it from ChartNagari, register the plugin in `config/execution.yaml`
**disabled**. It is not a `/webhook` execution plugin.

## Data mapping

| MT4 | v1 protocol |
| --- | --- |
| Account number `12345` | account `mt4-12345`, mode `paper` |
| `AccountFreeMargin()` | `buying_power` |
| Ticket `1001` | `client_order_id` `mt4-1001`, `broker_order_id` `1001` |
| Open/closed market order | `filled` |
| Pending order / deleted pending | `submitted` / `cancelled` |
| Lots × `MODE_LOTSIZE` | `quantity` in currency units |

Order pages always report `complete: false`. They contain open orders plus the most recent
`HistoryLimit` entries of the history loaded in the terminal.

## Not implemented yet

- Order placement. This needs a command queue that the EA polls, ambiguous-outcome
  reconciliation by ticket/comment, and an `fx` asset class in the v1 protocol.
  ChartNagari's common execution gateway (risk limits, sizing, kill switch) must come first.
- Live accounts.
