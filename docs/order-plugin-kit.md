# Order Plugin Developer Kit v1 (experimental)

Build a broker/exchange adapter as a **separate process**, without importing
ChartNagari's `internal` packages or loading third-party code into the server.
The HTTP contract is language-neutral; the reference SDK and simulator use Go.
This is an initial developer foundation, not a live-trading release or a plugin
marketplace. Passing checks is not a security or financial-safety certification.

## What works now

| Area | v1 implementation |
| --- | --- |
| Public SDK | `pkg/brokerplugin`: authenticated HTTP server/client, types, validation, durable request journal |
| Discovery | Versioned manifest, capabilities, declarative settings schema |
| Queries | Accounts/buying power, bounded order snapshots, lookup by client order ID |
| Writes | Paper market orders and optional cancellation; host must explicitly enable them |
| Simulator | Fill, reject, partial fill, pending order, response lost **after** acceptance; persistent state |
| Compatibility checks | Read-only CLI against external plugins; isolated simulator integration/race tests |
| Alpaca example | v1 discovery/accounts/order queries only; existing `/webhook` submission remains unchanged |
| Main server | Authenticated metadata inspection of an already registered plugin |

**Not implemented yet:** live mode, limit/stop orders, replace, event streaming /
fill subscriptions, positions, automatic settings forms/writes, installation,
signature verification of distributed binaries, automatic updates, and routing
ChartNagari signals through the new v1 order API. `replace` and `events` must be
false. A plugin cannot advertise these as implemented by this SDK.

The simulator template is a v1 development endpoint, **not a drop-in `/webhook`
execution plugin**. Registering its origin for metadata inspection does not make
ChartNagari dispatch v1 orders. Keep it disabled in execution settings. Existing
Alpaca still uses the legacy signal/feedback pipeline and its existing safeguards.

## Run the template safely

From the repository root (Go 1.26+):

```bash
cp examples/order-plugin/config.example.yaml examples/order-plugin/config.local.yaml
chmod 600 examples/order-plugin/config.local.yaml
openssl rand -hex 32
```

Put the generated value in `plugin_secret` in the **local** YAML, not in a command
argument, issue, manifest or committed file. Leave `orders_enabled: false` to
inspect without writes. Then:

```bash
go run ./examples/order-plugin -config examples/order-plugin/config.local.yaml
```

The template listens on `127.0.0.1:9200`. It contains no broker network client or
real trading credentials. Its only account is `sim-account`, mode `paper`, USD.
For manual simulator order exercises, explicitly set `orders_enabled: true` and
restart **this template**, not the ChartNagari server. Symbols select scenarios:

| Symbol | POST result | Subsequent GET |
| --- | --- | --- |
| `SIM-FILL` | `filled` | `filled` |
| `SIM-REJECT` | `rejected` | `rejected` |
| `SIM-PARTIAL` | `partial_fill` (half the quantity) | Same; cancellation retains filled quantity |
| `SIM-PENDING` | `submitted` | Same until cancelled |
| `SIM-TIMEOUT` | `unknown` | `filled` — response loss, not order loss |

Other symbols are rejected. Simulated buying power is fixed fixture data, not an
accounting engine. The SDK request journal and the simulated broker database are
separate files; preserve both across restarts. Do not use them for other adapters.

## Read-only external check

Create a private `plugin-check.local.yaml` (gitignored) and restrict it to mode
0600:

```yaml
url: "http://127.0.0.1:9200"
plugin_id: example-paper
plugin_secret: "your-private-shared-secret"
```

```bash
go run ./cmd/plugin-check -config plugin-check.local.yaml
```

The command makes only GET requests: manifest, accounts and order snapshots. It
checks version, identity and response structure. It does **not** submit or cancel
even if the plugin advertises `simulation: true`; an untrusted declaration is
not permission to trade. The report is printed to stdout without secrets.

The mutating conformance scenarios only run against locally created simulator
instances, using temporary databases and test credentials:

```bash
go test -race ./pkg/brokerplugin/... ./internal/plugins/alpaca
```

These cover concurrent duplicates, changed-payload conflicts, interrupted
reservations, restart persistence, ambiguous outcomes, explicit account/mode
validation, partial cancellation, authentication and redirect refusal. Third-
party adapters must additionally run their own broker-specific sandbox tests.

## Create your own adapter

Use `examples/order-plugin/main.go` as the executable skeleton. In a separate Go
module, depend on this repository's published revision using its **module path**:

```text
github.com/Ju571nK/Chatter/pkg/brokerplugin
```

The module path intentionally differs from the GitHub repository name
`ChartNagari`; it is not a typo. Before a revision is published, use a local Go
`replace github.com/Ju571nK/Chatter => /absolute/path/to/Chartter` directive.

Implement `Broker` (`Manifest`, `Accounts`, `Orders`, `Order`, `Submit`) and, if
advertised, `Canceller`. Replace `simulator.Open` in the template with your
implementation. Provide real account IDs from the connected broker, not a
user-entered unchecked account string. Validate instruments, quantity increments,
available order types, permissions and account status at the adapter boundary.
Never reinterpret unsupported inputs silently (e.g. crypto as stocks).

Use decimal **strings** for money and quantities. v1 quantities allow up to 18
integer and 12 fractional digits, no exponent notation, positive quantities only.
ID fields use `[A-Za-z0-9][A-Za-z0-9_.-]{0,47}`. Map broker IDs to stable protocol
IDs if necessary. `BrokerOrderID` retains the broker's original order identifier.
`buy`/`sell` are explicit: `sell` is not a guarantee of “close existing long only.”

Return `ErrRejected` from submission only when the broker definitively rejected
the order. Transport errors, timeouts and undecodable responses are ambiguous.
The SDK returns a journaled `unknown` result and never invokes Submit again for
that ID. A failed cancellation must not turn the original order into `rejected`.
Query the broker by the original client ID to reconcile it instead.

The HTTP definition is in [OpenAPI](api/order-plugin-v1.openapi.yaml). Python,
TypeScript, Java, etc. can implement this contract directly. Generated clients
still need the HMAC signer below. No Codex plugin format or MCP tool is required.

## HTTP and authentication

Every v1 request uses these headers:

```text
X-ChartNagari-Plugin-Id: <manifest ID>
X-ChartNagari-Timestamp: <Unix seconds>
X-ChartNagari-Signature-256: <lowercase HMAC-SHA256 hex>
```

The HMAC key is the shared secret. Sign the UTF-8 bytes of:

```text
plugin_id + "\n" + timestamp + "\n" + METHOD + "\n"
+ escaped_path_and_query + "\n" + hex(sha256(raw_body))
```

`escaped_path_and_query` is the exact request target (`/v1/orders?account_id=...`),
including query order/escaping. Empty GET bodies use SHA-256 of zero bytes. Use
constant-time signature comparison. Requests expire outside ±300 seconds. Body
and client response limits are 1 MiB. There is no automatic HTTP retry or redirect.
Legacy `/webhook` and feedback signing still uses **path only**; do not change it.

| Method/path | Behavior |
| --- | --- |
| `GET /v1/manifest` | Identity, capabilities, settings schema, current `orders_enabled` gate |
| `GET /v1/accounts` | Paper accounts, currency and buying power |
| `GET /v1/orders?account_id=...` | `{orders: [...], complete: bool}` snapshot |
| `GET /v1/orders/{client_order_id}?account_id=...` | Broker reconciliation query |
| `POST /v1/orders` | Explicit account/mode/side/quantity order request |
| `POST /v1/orders/{client_order_id}/cancel` | `{request_id, account_id}` cancellation |

An order request example (simulator only):

```json
{
  "client_order_id": "demo-order-001",
  "account_id": "sim-account",
  "mode": "paper",
  "symbol": "SIM-FILL",
  "asset_class": "stock",
  "side": "buy",
  "type": "market",
  "time_in_force": "day",
  "quantity": "2"
}
```

Writes return HTTP 202 plus an Order, even when that order is `rejected` or
`unknown`. **202 is not a fill confirmation.** IDs are scoped by operation and
account. A repeated identical submission/cancellation returns the originally
journaled result (possibly stale); use GET for current status. Reusing an ID with
a different payload returns 409. Cancellation IDs cannot be reused for another
target order. Comparison is based on normalized request JSON, but decimal string
spelling remains significant (`"2"` differs from `"2.0"`).

Other statuses: 400 malformed/unknown fields; 401 authentication; 404 unknown
account/order/route; 413 oversized request; 422 unsupported/invalid order; 423
writes disabled; 501 capability unavailable; 502 invalid/unavailable broker read;
503 journal unavailable. Error responses use safe codes, never raw broker bodies.

The journal reserves an ID **before** broker invocation. If the process dies
between reservation and invocation, that ID remains unknown even if nothing was
sent. This deliberately favors duplicate prevention over automatic recovery.
There is no exactly-once guarantee across a network. Do not delete/rotate the
journal, choose a new ID, or resubmit after a 404 until acceptance ambiguity is
resolved with the broker (including eventual-consistency delays).

## Settings, local/remote and trust boundaries

Manifest settings declare `key`, `label`, `type` (`text`, `secret`, `select`),
`required` and bounded `options` for select fields. They contain no values or
secret defaults. v1 provides the schema but does not yet generate a web form or
provide a settings-write endpoint. The template reads private YAML; the existing
Alpaca adapter continues reading the application's web-managed YAML on startup.

SDK clients allow HTTPS remotely and HTTP only for loopback. The template itself
binds loopback only. For a remote plugin, terminate TLS at a properly configured
reverse proxy on its host; retain HMAC authentication and restrict network access.
Mobile clients should use their chosen ChartNagari server, not keep broker keys or
run plugins inside the mobile app. Separate processes are an isolation boundary,
**not a sandbox**: run reviewed binaries under restricted OS/container identities.
Use unique secrets per plugin, encrypted secret storage where available, no
withdrawal permission, and separate paper/live credentials.

The main server adds `GET /api/execution/plugins/{id}/manifest`. It requires a
configured admin token and matching Bearer header, selects only an already
registered ID and signs a GET to `/v1/manifest` at that plugin's configured
webhook origin. It refuses redirects and non-loopback cleartext URLs. Metadata
inspection does not install anything, change execution settings, or enable a
plugin. Legacy plugins can keep working even if v1 metadata is unavailable.

## Alpaca compatibility and next integration steps

`cmd/plugin-alpaca` exposes read-only v1 metadata/accounts/orders alongside its
existing `/webhook` and `/healthz`. Its v1 `submit`, `cancel`, `replace`, `events`
and `orders_enabled` are all false. The legacy webhook stays paper-only. No new
v1 route bypasses the main dispatcher's execution controls. HTTP clients now
refuse redirects so credentials are not forwarded to another endpoint.

Order snapshots contain at most the broker's latest 100 results and explicitly
report `complete: false`; this is not full history. Unknown broker statuses remain
`unknown`, not `filled`. Endpoints follow Alpaca's official
[account documentation](https://docs.alpaca.markets/us/docs/working-with-account),
[order documentation](https://docs.alpaca.markets/us/docs/working-with-orders) and
[client-ID lookup](https://docs.alpaca.markets/us/reference/getorderbyclientorderid).
Tests use an HTTP fixture, not a real Alpaca account.

Before enabling v1 in ChartNagari's actual signal dispatch, implement the common
execution gateway: account selection, symbol mapping, sizing/position rules,
global kill-switch enforcement, risk limits, durable fill reconciliation and
audit logging. Then add schema-generated settings and explicitly reviewed broker
adapters. Do not simply switch `OrdersEnabled` on for the Alpaca v1 host.
