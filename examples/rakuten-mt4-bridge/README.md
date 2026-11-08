# Rakuten MT4 bridge

Rakuten FX has no official REST API for orders. This sidecar is a placeholder.
A later build can connect an MT4 Expert Advisor to this process. Until that
transport exists, the process serves the order-plugin simulator on loopback.
It does not open a broker connection.

Leave orders off. The process exits if `orders_enabled` is true. `bridge_mode`
must be `not-connected`. The manifest also lists `MT4 EA`. Startup rejects
that value because this build has no MT4 transport.

Forex candles stay on Yahoo or OANDA. Do not set `forex.provider` to Rakuten.

## Run the sidecar

From the repository root, copy the example and restrict the local file:

```bash
cp examples/rakuten-mt4-bridge/config.example.yaml examples/rakuten-mt4-bridge/config.local.yaml
chmod 600 examples/rakuten-mt4-bridge/config.local.yaml
openssl rand -hex 32
```

Put the generated value in `plugin_secret` in the local YAML. Do not commit
that file. Then start the process:

```bash
go run ./examples/rakuten-mt4-bridge -config examples/rakuten-mt4-bridge/config.local.yaml
```

The process listens on `127.0.0.1:9300`. The only account it reports is the
simulator fixture `sim-account`. That account is not a Rakuten account.

## Register for metadata inspection

Uncomment the Rakuten block in `config/execution.yaml`. Keep that plugin's
`enabled` value false, and keep the dispatcher `enabled` value false. Use the
same `plugin_id` and `plugin_secret` as the local YAML.

Start the sidecar, then open the Execution tab. Metadata inspection sends
`GET /v1/manifest` to the registered origin. The manifest shows bridge mode
options `not-connected` and `MT4 EA`, with `orders_enabled` false. Inspection
does not place an order.

The same read-only check as the template works against this origin. Point
`cmd/plugin-check` at `http://127.0.0.1:9300` and `plugin_id` `rakuten-mt4-bridge`.
