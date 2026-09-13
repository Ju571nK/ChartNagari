# ChartNagari

A personal workspace for exploring US stocks and crypto — from chart patterns to alerts and strategy review.

[Try the demo](https://ju571nk.github.io/ChartNagari/) · [Latest release](https://github.com/Ju571nK/ChartNagari/releases/latest) · [Get started](#get-started)

[English](README.md) · [한국어](README.ko.md) · [日本語](README.ja.md)

![ChartNagari: expert chart overlays, timeframe switching and web settings](docs/demo.gif)

*Captured from the current main build on September 12, 2026. This short tour uses sample candles, not a live feed. [View still screenshots](docs/screenshots/README.md).*

## One place to follow your market

- **See the setup.** Explore charts in a simpler beginner view or switch to expert mode for FVG, Order Block and signal overlays.
- **Keep your context.** Carry the selected instrument and timeframe into analysis and backtesting.
- **Make alerts your own.** Manage watchlists, price alerts and Telegram/Discord notification preferences.
- **Review before acting.** Check available history, run backtests and follow paper trades without treating simulated results as predictions.
- **Set things up in the browser.** Configure data providers, optional AI and connection settings through the web UI.

English, Korean and Japanese interfaces are available. AI interpretation is optional, including a local Ollama option.

## What's new

**Latest release: [v2.13.0.0 — Web Settings & Clearer Charts](https://github.com/Ju571nK/ChartNagari/releases/tag/v2.13.0.0)**

Grouped navigation, a chart-side signal inspector and shared chart → analysis → backtest context.

**Included in this release**

- Visible FVG/OB candidate ranges with distinct colors and counts.
- Clearer loading, missing-data and retry states; history checks before backtesting.
- YAML-backed web settings, masked keys and explicit key removal.
- Fixes for symbol settings, alert persistence, timeframe filtering and VIX collection.

Upgrading? Read the [YAML migration notes](SETTINGS.md) and [release notes](docs/releases/v2.13.0.0.md) before restarting your server.

## Get started

**Just looking?** [Open the browser demo](https://ju571nk.github.io/ChartNagari/). It uses bundled sample data; live collection, persistent settings and connected services require a self-hosted server.

**Ready to use your own watchlist?** Follow the [installation guide](docs/getting-started.md) for local or Docker setup. Then:

1. Add an instrument in **Symbols**.
2. Open **Chart**, choose a timeframe and start in beginner mode.
3. Visit **Settings** to connect only the data, AI or notification services you need.

Already using an older version? Read the [YAML migration guide](SETTINGS.md) before upgrading. General configuration changes need a restart; the interface identifies the relevant settings.

## A few things to know

ChartNagari is a research tool, not investment advice. FVG/OB overlays are visual candidates, not instructions to trade. Historical and paper results do not guarantee future performance.

Your database and settings are stored locally. Enabled market-data, hosted AI and notification integrations communicate with their providers; availability, fees and limits depend on those services. Saving settings does not start an execution adapter.

## Learn more

[Settings & upgrades](SETTINGS.md) · [Architecture](docs/architecture.md) · [Contributing & development](CONTRIBUTING.md) · [Order plugin developer kit](docs/order-plugin-kit.md) · [Report an issue](https://github.com/Ju571nK/ChartNagari/issues)

Built by Justin. Open source under the [MIT License](LICENSE).
