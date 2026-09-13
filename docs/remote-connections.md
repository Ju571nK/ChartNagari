# Local and remote connections

Chart Nagari can use the server that serves the page, or connect from that page
to another Chart Nagari server. This is web-client support, not a native mobile
app or an internet tunnel. Local LLM means local to the **connected backend**, not
necessarily the phone or browser computer. Existing `OLLAMA_HOST` configuration
continues to select the backend's local or remote Ollama endpoint.

## Local use

Existing local startup remains unchanged. The connection disclosure above the
workspace shows the current server. Expand it to choose the web server again,
enter a token if configured, and verify the connection. A configured token is
checked even when local read APIs otherwise retain their legacy anonymous access.

## Enable a remote server

On the target server, use Settings → Advanced to set:

- `API_TOKEN`: a strong, unique random token of at least 32 characters. This is
  the existing **administrator** token, not a read-only user credential.
- `REMOTE_ACCESS`: `true`. This requires authentication for all `/api/` reads as
  well as writes. Existing scripts must send bearer authentication for reads too.
- `REMOTE_ALLOWED_ORIGINS`: exact origins of permitted client apps, comma
  separated, e.g. `https://charts.example.com`. No wildcard, paths, query strings,
  fragments, or embedded credentials. HTTP origins are allowed only for loopback
  development such as `http://localhost:5173`.

Save and restart the backend. The server validates this combination before
starting collectors; saving an enabled remote configuration without a sufficiently
long token is rejected. Do not publish the plain HTTP backend port directly.
Terminate HTTPS at a reverse proxy, keep the backend behind it, forward WebSocket
upgrades and `Sec-WebSocket-Protocol`, and restrict direct backend access. No TLS
certificate, proxy, firewall rule or public listener is created automatically.
The client requires HTTPS for non-loopback servers and rejects URL credentials
and subpath deployments. A valid browser-trusted TLS certificate is required.

## Connect from a browser or phone

1. Expand **Server connections**, enter a name, server origin, and administrator
   token for that server. For example `https://home-charts.example.com`.
2. **Verify and connect** authenticates `/api/connection` and checks protocol
   compatibility and remote mode. It displays the available capability names.
3. Confirm the switch. Unsaved forms and workspace selection reset; pending
   requests and old WebSocket connections are closed. Operations already accepted
   by the previous server are not rolled back or cancelled on that server.
4. API requests, model-download streams, and authenticated file downloads now use
   the selected server. Non-API external links are not rewritten.

Only profile names/addresses are stored in browser local storage. The selected
address is stored in session storage; tokens stay only in memory, are cleared on
server changes/disconnect, and never enter URLs. Reloading returns to a connection
gate, requiring selection/authentication again, rather than falling back silently
to the web server. Failed verification leaves the current connection unchanged.

The app must be served as a top-level web application on the web server. Native
WebView/file origins, offline native execution, user accounts/roles, automatic
failover, subpath API bases and proxy provisioning are outside this first stage.
Feature capability discovery does not introduce read-only permissions: possession
of the administrator token still grants administrative operations.

## WebSocket authentication

In remote mode, browser sockets require an origin-bound, single-use ticket from
authenticated `POST /api/connection/ws-ticket`. Tickets expire after 30 seconds;
at most 256 unexpired tickets are held. The ticket travels in the WebSocket
subprotocol header, not a URL; the persistent API token is not sent in socket
URLs. The stable negotiated protocol is `chartnagari.browser.v1`. Existing
execution clients retain the hub's independent HMAC protocol validation.

## Verification (2026-09-13)

- Frontend: 182 tests across 28 files and production build passed. Backend:
  `go test ./...` and `go test -race ./internal/api ./internal/config` passed.

- Tested server authentication, local-read compatibility, CORS allowlisting,
  malformed execution-protocol rejection, ticket expiration/origin/replay checks,
  and a real Gorilla WebSocket handshake regression.
- Frontend tests cover secure-origin validation, destination/token isolation,
  aborting old requests, local verification while a remote target is selected,
  secret-free profile persistence and failed verification without switching.
- An isolated two-server browser harness (loopback ports 18081/18082) used only
  temporary YAML, dummy credentials and distinct test calendars. Verified remote
  authentication, real WebSocket connection, remote-only calendar data, switching
  back to local without old data, and reauthentication gate after reload.
- Inspected desktop 1440×900 and mobile 390×844 / Korean 360×800. Mobile document
  width matched viewport width. No browser JavaScript errors were reported.
- No real remote host, public TLS reverse proxy, physical phone or external LLM
  service was used. No existing user settings, server process, notifications or
  order configuration were changed. The harness and browser were stopped afterward.
