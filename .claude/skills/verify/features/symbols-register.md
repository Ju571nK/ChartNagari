# Register a symbol

Symbols lets a user validate and register an instrument on the watchlist, then see it listed before market data has necessarily arrived.

## Sub-features

- `symbols-open` opens the Symbols workspace.
- `symbols-type-forex` selects Forex as the instrument type.
- `symbols-add-eurusd` registers `EURUSD` after client-side Forex validation.
- `symbols-listed` shows `EURUSD` in the list with a post-add data status.

## How to get to it (user POV)

- Choose the `Symbols` item in Workspace navigation.
- Open `/?view=symbols` on the verification base URL.
- After landing elsewhere, use the mobile primary nav `Symbols` button if the desktop rail is collapsed.

## Driving it with agent-browser

Preconditions:

- Doctor passes for this run's `VERIFY_BASE_URL`.
- `GET /api/symbols` does not already include `EURUSD`.
- English UI.

- **Open Symbols.** Run `agent-browser open "${VERIFY_BASE_URL}/?view=symbols"` then `agent-browser wait --load networkidle` and `agent-browser snapshot -i`. A heading or nav current page for `Symbols` is visible; the add form shows.
- **Dismiss onboarding if present.** If `Set up later` is visible, click it, then re-snapshot. The add-symbol form must be reachable without an overlay intercepting clicks.
- **Choose Forex.** In the add-symbol form, set the type select to `Forex` (option label from i18n `forex.type`). Snapshot confirms the exchange field shows `FX`.
- **Enter EURUSD.** Fill the symbol textbox (placeholder is the stock hint string) with `EURUSD`. Wait until validation shows a found-style label (e.g. `EUR/USD`) and the `Add` button is enabled.
- **Add.** Click `Add`. A registration confirmation mentioning `EURUSD` appears; a “no stored candles” / data-status message may also appear — that is success for registration, not a chart-ready claim.
- **Confirm list + API.** Snapshot the Symbols list showing `EURUSD`. Run `curl -fsS "${VERIFY_BASE_URL}/api/symbols"` and require a JSON entry with `"symbol":"EURUSD"` and `"type":"forex"`.
- **Proof.** Save `agent-browser snapshot -i` to `.claude/skills/verify/artifacts/symbols-register/after.aria.txt` and `agent-browser screenshot .claude/skills/verify/artifacts/symbols-register/after.png`. Save the `/api/symbols` body beside them as `symbols.json`.

## Gotchas

- First visit may show an onboarding overlay (`Try demo` / `Set up later`). Dismiss with `Set up later` before driving the add form; otherwise clicks hit the wrong layer.
- Stock/crypto add calls `/api/symbols/validate` (network). Prefer Forex `EURUSD` for offline-friendly verification.
- The primary add control may render as `ADD` (uppercase). Prefer the enabled button in the add-symbol form, not the onboarding `Add`.
- `Add`/`ADD` stays disabled until validation reaches `found`; do not force-click.
- Registration ≠ candles ready. A “Try again” / empty candle status after add is still a pass for this feature.
- DXY init may already list `DX-Y.NYB`; `forex(1)` / API entry for `EURUSD` is the proof, not list length 1.
- Preset buttons (`Majors`, etc.) mutate many pairs — avoid them in this journey.
- `agent-browser screenshot PATH` may still write under `~/.agent-browser/tmp/screenshots/`; copy the printed path into `.claude/skills/verify/artifacts/<feature>/` afterward.
- Clean up `EURUSD` on the scratch instance after proof if a later feature needs an empty list; keep artifacts.
