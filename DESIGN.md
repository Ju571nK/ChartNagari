# Design System — ChartNagari

## Product Context
- **What this is:** Local-run trading signal detection platform with ICT/Wyckoff/SMC analysis across multiple timeframes
- **Who it's for:** Technical traders who use ICT/Wyckoff methodology, self-hosted tool users, algo-trading developers
- **Space/industry:** Algorithmic trading, technical analysis tooling
- **Project type:** Data-dense dashboard with chart visualization, signal log, backtesting

## Aesthetic Direction
- **Direction:** Industrial/Utilitarian — function-first, data-dense, muted palette with selective color emphasis
- **Decoration level:** Minimal — typography and spacing do the work. No gradients, shadows, or decorative elements except subtle borders
- **Mood:** A quiet, professional trading terminal. Dark, focused, no distractions. Color appears only when it means something (signal direction, risk level, quality score)

## Color

### Core Palette (5 tokens)

| Token       | Hex       | Role                                              |
|-------------|-----------|---------------------------------------------------|
| `--bg`      | `#12130F` | Dark ground. All backgrounds start here            |
| `--green`   | `#5B9279` | Sage green. Primary accent, borders, active states |
| `--mint`    | `#8FCB9B` | Light mint. Brand highlight, LONG signals, bullish |
| `--text`    | `#EAE6E5` | Warm off-white. All primary text                   |
| `--muted`   | `#8F8073` | Warm brown-gray. Secondary text, labels, disabled  |

### Semantic Tokens (4 tokens)

| Token       | Hex       | Role                                        |
|-------------|-----------|---------------------------------------------|
| `--danger`  | `#ef4444` | High-impact red. Errors, SHORT signals       |
| `--warning` | `#f59e0b` | Medium-impact amber. Alerts, unset states    |
| `--safe`    | `#22c55e` | Low-impact green. Success, completed states  |
| `--slate`   | `#94a3b8` | Secondary data. Supplemental information     |

### Usage Rules
- **LONG/bullish:** `--mint` (`#8FCB9B`)
- **SHORT/bearish:** `--muted` with 0.9 opacity (`rgba(143,128,115,0.9)`) for chart markers, `--danger` for text labels
- **Active button/tab:** `--green` border + `--bg` background
- **Disabled/inactive:** `--muted` at 0.4 opacity
- **Overlay backdrop:** `rgba(0,0,0,0.72)`
- **Borders:** `--green` at 0.2 opacity for section dividers, full opacity for active elements
- **Hover states:** `--green` at 0.08 background + 0.25 border opacity

### Dark Mode
Single-mode only. The entire app is dark by design. No light mode.

## Typography
- **Font stack:** `-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif` (system fonts)
- **Why system fonts:** Zero load time, familiar to users on every platform, no FOUT. A data-dense trading dashboard benefits from native rendering speed over custom typography
- **Heading (h1):** 1.2rem / weight 600 / `--text` / letter-spacing -0.01em
- **Subtitle:** 0.75rem / weight 400 / `--muted`
- **Body/buttons:** 0.8rem / weight 500
- **Small labels:** 0.68-0.72rem / weight 400 / `--muted`
- **Tiny data:** 0.75rem / weight 400

### Tabular Data
Use `font-variant-numeric: tabular-nums` for all numeric columns (scores, prices, percentages). System fonts support this natively.

## Spacing
- **Base unit:** 4px
- **Density:** Compact — this is a data-dense dashboard, not a marketing page
- **Common values:** 4px (tight gaps), 6px (button groups), 8px (section padding), 16px (medium gaps), 24px (section margins), 32px (large section separators), 36px (container padding)
- **Container:** workspace max-width 1800px, 208px navigation rail, 30px content padding (16px on compact screens)

## Layout
- **Approach:** Persistent grouped navigation: Explore, Validate, Trade, Configure. Every existing page remains reachable.
- **Max content width:** 1800px workspace; settings content constrained to 1020px including padding.
- **Navigation:** Labeled sidebar buttons with `aria-current`; navigation, selected symbol and timeframe are represented in the URL.
- **Grid:** Main chart and 300px signal inspector side by side above 1200px; stacked below. Use minmax(0, 1fr) to avoid canvas/table overflow.
- **Responsive:** Below 760px, a labeled select with grouped options replaces the sidebar. Tables may scroll within their own region.
- **Reading hierarchy:** Page heading 1.5rem; normal form controls 0.875rem; compact metadata may remain smaller. Numeric data uses tabular figures.
- **Status:** Never label failed requests as empty results. Execution controls require a successfully loaded configuration; failed writes show actionable errors.

## Components

### Buttons (`.tab-btn`)
- Default: transparent background, `--green` border at 0.15 opacity, `--muted` text
- Active: `--green` border at full opacity, `--text` color
- Hover: `--green` background at 0.08 opacity
- Border-radius: 4-6px
- Padding: 5px 16px
- Font-size: 0.8rem

### Signal Markers (chart)
- LONG: green arrow below bar, opacity varies by quality score (0.4/0.7/1.0)
- SHORT: muted arrow above bar, opacity varies by quality score
- Wyckoff events: circle markers (Spring = `--mint`, Upthrust = `#B47B4A`)
- Deduplication: one marker per candle per direction (highest score wins)

### Score Quality Indicators
- HIGH (>= 0.7): `--safe` color, "HIGH" label
- MED (0.4-0.7): `--warning` color, "MED" label
- LOW (< 0.4): `--danger` color, "LOW" label
- Display: 8px color dot + numeric score (e.g., "4.5")

### Category Filter Buttons
- ICT / Wyckoff / SMC / TA toggle group
- Same `.tf-btn` pattern as timeframe buttons
- Separated from TF group by a 1px vertical divider (`--muted` at 0.2 opacity)
- State persisted in localStorage (`chartnagari_chart_signal_filter`)

### Onboarding Modal (canonical token usage)
- Container: `max-width: 640px`, `display: flex` split layout (240px left rail + flex-grow right)
- Border: `1px solid var(--green)`
- Background: `var(--bg)`
- Step indicators: 24px circles. Active = `--green` border, Complete = `--safe` fill + checkmark, Inactive = `--muted` at 0.4
- Completed state: `--safe`
- Error inline: 12px `--danger` text
- Warning banner: `--warning` with 0.3 opacity border

## Motion
- **Approach:** Minimal-functional. Only transitions that aid comprehension
- **Default transition:** `all 0.15s` for interactive elements
- **Easing:** CSS default (ease)
- **No entrance animations, no scroll effects, no loading skeletons.** Data appears when ready

## Anti-patterns (do not use)
- Emojis in step labels or headings
- Center-aligned step content (always left-align)
- Decorative border-left color stripes on cards
- Gradient backgrounds or buttons
- Box shadows on cards (except dropdown menus: `0 8px 24px rgba(0,0,0,0.5)`)
- New fonts imported from CDN
- New color tokens without updating this file
- Hardcoded hex values instead of CSS variables
- `!important` overrides
- `outline: none` without replacement focus indicator

## Decisions Log

| Date       | Decision                          | Rationale                                                    |
|------------|-----------------------------------|--------------------------------------------------------------|
| 2026-03-22 | 5-color core palette              | Minimal palette forces consistency across all UI              |
| 2026-03-23 | 4 semantic tokens added           | Financial risk levels need distinct colors (danger/warning/safe/slate) |
| 2026-03-28 | Onboarding modal token patterns   | Established canonical examples for all semantic token usage   |
| 2026-04-01 | Score quality visual system       | 3-tier color coding (HIGH/MED/LOW) for signal quality scores |
| 2026-04-01 | Chart marker opacity by score     | Score-based alpha (1.0/0.7/0.4) to visually differentiate signal quality |
| 2026-04-01 | Category filter UI                | ICT/Wyckoff/SMC/TA toggle buttons with localStorage persistence |
| 2026-04-02 | DESIGN.md created                 | Document existing system before external contributors submit UI PRs |
