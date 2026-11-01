# Workspace navigation

The workspace shell keeps ChartNagari identity while the user moves between primary pages.

## Sub-features

- `nav-symbols` opens Symbols from Chart.
- `nav-chart` returns to Chart.
- `nav-settings` opens Settings.
- `nav-shell` keeps the main workspace chrome mounted across hops.

## How to get to it (user POV)

- Use buttons inside `nav` named `Workspace navigation`.
- On narrow viewports, use the mobile primary nav row.

## Driving it with agent-browser

Preconditions:

- Doctor passes.
- Start at `/?view=chart`.

- **Land on Chart.** Open `${VERIFY_BASE_URL}/?view=chart`, wait, snapshot. `Chart` is current.
- **Go to Symbols.** Click the `Symbols` nav button (or mobile primary). Snapshot shows Symbols content.
- **Go to Settings.** Click `Settings`. Snapshot shows Settings content.
- **Return to Chart.** Click `Chart`. Snapshot shows Chart again.
- **Proof.** Save three screenshots under `.claude/skills/verify/artifacts/workspace-nav/` (`chart.png`, `symbols.png`, `settings.png`) plus a final ARIA snapshot `final.aria.txt`.

## Gotchas

- Beginner vs expert mode changes which links are in the rail vs under More; primary `Chart` / `Symbols` / `Settings` remain reachable.
- Do not treat lazy-loaded tab code-splitting delays as failures until network idle + snapshot miss the destination after a few seconds.
