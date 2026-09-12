# Contextual settings audit

## Fixed

- Calendar: previously returned an instruction-only empty state and assumed a
  missing paid key from an empty event array. Now the same page includes a
  lazy-open inline FMP/Finnhub key and advance-alert form, independent of calendar
  loading, errors or events. It reuses YAML persistence, secret masking/clear,
  bearer authorization and restart-required feedback. Refresh only reads cached
  calendar events; it does not restart services or force provider collection.
- Analysis: an inline AI/LLM settings disclosure now exposes provider/credential
  fields without requiring users to locate a separate Settings subsection.
- Calendar request cancellation prevents unmounted/obsolete responses overwriting
  current state. Invalid payloads are errors, not empty calendars.
- Calendar field labels no longer imply contradictory free/paid provider access.
  Actual availability depends on provider account permissions and collection.

## Related menus inspected

| Menu | Finding / disposition |
|---|---|
| Alert hub | Already exposes signal thresholds, price conditions and delivery credentials. |
| Settings / MCP | Already includes bridge URL/token fields and client setup snippets. |
| Settings / AI / Ollama | Detector-unconfigured message exists, but provider/host/model fields are available above it. No duplicate form added. |
| Execution | Existing global/plugin controls; no automatic activation added. |
| Paper / My Trades / History | Empty transaction/history displays represent missing records, not proof of missing credentials. Kept distinct from setup failures. |
| Chart | Existing missing-price actions lead to symbol management; unchanged. |

## Verification

Mocked frontend tests cover focused fields, masked secrets, edited-field-only
save payloads, error/empty distinction, AI form access, abort on unmount, and
retention of edits after a failed save and calendar refresh. No live secrets or
settings were changed. The calendar form was visually inspected at 390×844 in
an isolated Chromium session against the local server.

Saving is not activation: runtime application still requires a server restart.
No claim of a working provider connection is made merely because a save succeeds.
