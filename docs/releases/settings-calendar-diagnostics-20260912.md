# Settings and calendar diagnostics

## Scope

- No issue created. No runtime configuration, credentials, orders or notification
  delivery settings were changed. No production process was restarted.
- `GET /api/settings/status` compares saved YAML with a copied snapshot captured
  inside `config.Load`, not with a file read later during API initialization.
  States are `startup_match`, `pending_restart`, `external` and `unknown`.
- Startup match means the startup input agrees, **not** proof of activation,
  effective normalized values, or a successful connection. MCP and Alpaca settings
  belong to separately managed processes and are not attested by this server.
- Forms also distinguish unsaved edits. Successful saves refresh the comparison;
  a save confirmation alone no longer asserts that every save requires a restart.
- Only state names are returned: no secret values, secret hashes or raw provider
  responses. Rotating or clearing a secret is detected even though both old and
  new values are masked by the settings form.

## Calendar

- `GET /api/calendar/status` reads a synchronized, process-local collector snapshot.
- Shows active provider, disabled/waiting/fetching/success/error, last attempt,
  last success and the last successful US-event batch count after date filtering.
- Failures distinguish authentication (401), access/subscription (402/403), rate
  limits (429), other provider failures, network, malformed response and database
  storage failure. These are categories, not proof of a particular billing cause.
- Database write failure now propagates to the existing retry loop. Successful
  empty batches are distinct from a missing key; failed requests retain previous
  success metadata. A malformed/null response is not treated as an empty success.
- History resets on process restart. Collection still runs at startup and every
  six hours, with retries after one and five minutes. Status checks poll every
  30 seconds and may be refreshed manually; neither starts provider collection.
  Existing event refresh still reads cached events only.
- Diagnostic requests are aborted on replacement/unmount. A missing/older endpoint
  shows an unavailable message without blocking forms or event loading.
- English/Korean/Japanese status copy; details collapsed to keep mobile pages short.

## Verification

- Frontend: 178 tests across 27 files; production TypeScript/Vite build passed.
- Backend: `go test ./...` passed; `go test -race ./internal/calendar ./internal/api
  ./internal/config` passed.
- Regression cases: startup snapshot copying, secret replacement/clear, simulated
  restart, external-process uncertainty, disabled collector, success with zero
  events, HTTP/auth/permission/rate-limit failures, malformed payload, storage
  failure/recovery, network failure preserving last success, refresh and aborts.
- Browser: isolated temporary Go API server with temporary YAML, no live collectors
  or notifications. Saving advance-alert minutes from 30 to 45 through the real
  form changed only that field to `pending_restart` in UI and API. Calendar access
  failure was an explicitly simulated fixture, not a live provider test.
- Desktop 1440×900, English mobile 390×844, Korean mobile 360×800 inspected;
  no horizontal overflow at measured mobile sizes; no browser JavaScript errors.
  Temporary verification server, files and browser session were cleaned up.
- The existing local endpoint at 127.0.0.1:8080 refused connections during this
  verification. A rebuilt/restarted backend is required to expose the new APIs.
