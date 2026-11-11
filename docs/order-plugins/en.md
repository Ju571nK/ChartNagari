# Getting started with the Order Plugin Developer Kit

[English](en.md) · [日本語](ja.md) · [한국어](ko.md) · [Documentation index](README.md)

An order plugin is a **separate executable** that connects a broker or exchange's
authentication and order API to ChartNagari's common contract. The developer kit
lets external developers build adapters without modifying the core application.
It is not a browser extension or a Codex plugin.

v1 is currently an **experimental developer foundation**, not a plugin store or
a product that can place live orders immediately after installation. This guide
describes the current main branch; it does not mean all features are included in
an earlier stable release.

## Everyday users: do I need this?

You do not need the kit for charts, price alerts or backtests. If you want to
connect a particular broker, first check whether an adapter actually exists and
what it supports. Entering an API key alone does not connect an arbitrary broker.

| Area | Current status |
| --- | --- |
| Existing Alpaca paper webhook | Existing execution path retained; separate adapter process and configuration required |
| New Alpaca v1 API | Plugin metadata, accounts and order queries only |
| Kit simulator | Develop/test orders, cancellation and failures without connecting to a broker |
| Installation, updates, generated web settings | Not implemented yet |
| Chart signals → new v1 order API | Not connected yet |
| v1 live trading, order replacement, fill-event subscriptions | Not supported yet |

**“Add plugin” on the Execution screen registers a legacy webhook adapter.**
The kit's simulator template does not replace a `/webhook` adapter. Registering
or enabling it does not send orders through the new v1 path. Do not enable
execution just to check connectivity. Opening the guide or reading documentation
does not change settings or enable orders.

## External developers: what is included?

- **Public Go SDK:** request authentication, validation, account/order APIs and durable duplicate-request records
- **OpenAPI contract:** a language-neutral HTTP definition for non-Go implementations
- **Standalone template:** private YAML, orders disabled by default, local execution
- **Simulated exchange:** fill, rejection, partial fill, pending and lost-response scenarios
- **Checker:** `plugin-check`, which only reads from external plugins
- **Automated tests:** duplicates, restarts, unknown outcomes and cancellation after partial fills

The settings schema declares field names and types. The kit does not yet
generate a web configuration form or save values from that schema.

## 1. Run the template without orders

Go 1.26 or later is required. Run these commands on macOS/Linux from the
**repository root**. If you already have a local configuration, do not overwrite
it with the copy command.

```bash
cp examples/order-plugin/config.example.yaml examples/order-plugin/config.local.yaml
chmod 600 examples/order-plugin/config.local.yaml
openssl rand -hex 32
```

Put the generated secret into `plugin_secret` in `config.local.yaml`.
Never commit it or paste it into issues, chats or command-line arguments.
Keep `orders_enabled: false` unchanged.

```bash
go run ./examples/order-plugin -config examples/order-plugin/config.local.yaml
```

Successful startup listens on `127.0.0.1:9200` and prints
`order API enabled: false`. The program contains no real broker client or live
account keys. Press `Ctrl+C` in its terminal to stop it.

## 2. Check the connection with read-only requests

Open another terminal and create `plugin-check.local.yaml` in the repository root.

```yaml
url: "http://127.0.0.1:9200"
plugin_id: example-paper
plugin_secret: "the-same-shared-secret-as-the-template"
```

```bash
chmod 600 plugin-check.local.yaml
go run ./cmd/plugin-check -config plugin-check.local.yaml
```

A successful result includes `protocol: "1.0"`, `plugin_id: "example-paper"`,
`accounts: 1` and `read_only: true`. The command reads the manifest, accounts and
order list; it does not submit or cancel orders. **Passing is not a safety
certification or proof of live-trading readiness.**

If the connection fails:

- Check that `url` matches the running address and port. Do not include a path such as `/webhook`.
- Check that `plugin_id` and the shared secret match on both sides.
- For authentication errors, also check both server clocks. The allowed difference is ±300 seconds.
- A legacy webhook adapter may not expose v1. A failed check does not necessarily mean its existing webhook is broken.

## 3. Build an adapter for your broker or exchange

1. Use the [standalone template](../../examples/order-plugin/main.go) as the starting point for a separate project.
2. Implement authentication, accounts, order queries and submission through `Broker`. Implement `Canceller` if cancellation is supported.
3. Validate symbols, quantity increments, account permissions and supported order types against the provider's rules.
4. Advertise only implemented capabilities in the manifest. `replace` and `events` must be `false` in v1.
5. Verify normal operation, errors and restarts in an isolated simulator, not a live account.

The Go module path differs from the repository name; this is not a typo:

```text
github.com/Ju571nK/Chatter/pkg/brokerplugin
```

For other languages, use the [OpenAPI specification](../api/order-plugin-v1.openapi.yaml)
and [technical reference](protocol.md). Generated HTTP clients still need HMAC
signing. The SDK does not implement broker-specific authentication, trading
permissions or quantity rules for you.

## 4. Simulator verification and safety rules

The repository tests use temporary databases and a fake exchange:

```bash
go test -race ./pkg/brokerplugin/... ./internal/plugins/alpaca
```

Only for explicit manual simulator-order exercises, enable `orders_enabled` in
the **simulator's** configuration and restart that program. Choose scenarios with
`SIM-FILL`, `SIM-REJECT`, `SIM-PARTIAL`, `SIM-PENDING` and `SIM-TIMEOUT`.
Request and signing examples are in the
[HTTP reference](protocol.md#http-and-authentication).

In particular:

- HTTP `202` is not a fill confirmation. Check the order status in the response.
- For `unknown`, query the original `client_order_id`. Do not submit again using a new ID.
- Repeating the same request with the same ID returns the original record. Use GET for the current state.
- Preserve the request journal across restarts. Deleting it removes the basis for duplicate prevention.
- Never use a live account to check connectivity or experiment with the kit.

## Local, remote and mobile use

Run the plugin as a **separate process** on a local PC or remote server. The
template binds only to loopback by default. Remote operation requires an HTTPS
reverse proxy, HMAC authentication and access restrictions that you configure.
Do not simply expose the port to the internet.

Mobile users should use their selected ChartNagari server, not install plugins
or store broker keys in the mobile browser. The current kit does not provide a
mobile live-trading interface or the new order-dispatch path.

Process separation is not a security sandbox. Run only reviewed plugins with
restricted permissions and use a different shared secret for each plugin.

## Next steps

Product integration still needs a common execution path for account selection,
sizing, symbol mapping, global order blocking, risk limits, fill reconciliation
and audit records. Plugin-specific web settings and reviewed adapters can then
be connected. Simply enabling writes in Alpaca's read-only v1 API is not a
substitute for those controls.

[Technical reference](protocol.md) · [OpenAPI](../api/order-plugin-v1.openapi.yaml) ·
[Template](../../examples/order-plugin/) · [Contributing](../../CONTRIBUTING.md)
