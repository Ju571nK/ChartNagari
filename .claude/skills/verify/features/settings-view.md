# Open settings

Settings is where the user reviews general configuration, connections, and advanced remote-access controls.

## Sub-features

- `settings-open` opens the Settings workspace from nav or URL.
- `settings-shell` shows the settings page chrome without crashing.

## How to get to it (user POV)

- Choose `Settings` in Workspace navigation (primary nav in beginner mode).
- Open `/?view=settings`.

## Driving it with agent-browser

Preconditions:

- Doctor passes.
- English UI.

- **Open Settings.** `agent-browser open "${VERIFY_BASE_URL}/?view=settings"` then wait and snapshot. Page heading or nav current page is `Settings`.
- **Confirm shell.** Snapshot shows settings sections or forms (connections / advanced / runtime status). No full-page crash blanking the shell.
- **Proof.** Save `.claude/skills/verify/artifacts/settings-view/settings.aria.txt` and `settings.png`.

## Gotchas

- Saving general settings may require restart; this feature only proves **open and render**, not persist-and-restart.
- With empty `api_token`, local reads stay anonymous; do not enable `remote_access` on the scratch instance during this journey.
