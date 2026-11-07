# Open the chart

Chart shows candlesticks and signal context for the selected workspace symbol and timeframe.

## Sub-features

- `chart-open` opens the Chart workspace.
- `chart-select-symbol` chooses a registered symbol.
- `chart-surface` shows the chart surface region (busy or with data).

## How to get to it (user POV)

- Choose `Chart` in Workspace navigation.
- Open `/?view=chart&symbol=EURUSD` when `EURUSD` is already registered.
- From Symbols after add, follow any "open chart" affordance if present. Otherwise use nav.

## Driving it with agent-browser

Preconditions:

- Doctor passes.
- `EURUSD` is registered (run [symbols-register](./symbols-register.md) first if needed).

- **Open Chart.** `agent-browser open "${VERIFY_BASE_URL}/?view=chart&symbol=EURUSD"` then wait for network idle and snapshot. Nav shows `Chart` as current.
- **Confirm symbol control.** The symbol selector (labelled with the symbol column name) includes or shows `EURUSD`.
- **Confirm surface.** A `section` named `Chart` exists (`aria-label` chart). It may be `aria-busy=true` while loading. Empty or error copy when Yahoo has no bars yet is also acceptable proof that the chart shell rendered for the selection.
- **Proof.** Write `.claude/skills/verify/artifacts/chart-view/chart.aria.txt` and `chart.png`. Optionally `curl -fsS "${VERIFY_BASE_URL}/api/ohlcv/EURUSD/1H"` and record status code (200 with bars, or empty/error body) without treating missing history as a UI failure.

## Gotchas

- Empty candle history is an environment/data condition, not a navigation bug.
- Demo static mode is a different build (`build:demo`); this skill drives the live server + `web/dist` production build.
- Do not require Telegram, Tiingo, or OANDA for this feature.
