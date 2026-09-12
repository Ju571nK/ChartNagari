# Desktop and mobile UX improvements

Seven audit groups addressed together:

1. Onboarding: single-column mobile dialog, bounded dynamic viewport height,
   scrollable content, non-overflowing input and separated secondary actions.
2. Chart space: compact normal data state, expandable timestamps/disclaimers and
   guide after the chart. Error/empty guidance stays visible. Refresh differs
   from retry; a successful price read still does not certify a live feed.
3. Touch controls: 44px minimum height on mobile controls; full-width alert
   inputs/selects and wrapping settings fields.
4. Signal clarity: full English/Korean/Japanese rule labels, original-description
   caption, score disclaimer, loaded-candle-range badges and matching marker filter.
   The range is not the zoomed viewport; historical records remain inspectable.
5. Navigation: Chart / Symbols / Alerts / Settings are directly available;
   beginner desktop tools fold under More. All existing routes remain available.
6. Backtest: selectors/actions before guidance, expandable ATR inputs, explicit
   strategy label, expert-only HTF inventory. No simulation semantics changed.
7. Settings/alerts: signal, price and delivery sections share an alert hub;
   global/profile override precedence is explained. Operational settings move
   to Advanced; General holds display/language. Authorization is expandable.
   Unsaved changes persist across settings sections and retain a save action.
   Saved YAML is explicitly restart-required and application remains unverified;
   the UI does not claim runtime settings match the file.

## Verification

- Frontend: 167 tests across 25 files passed; TypeScript and production build passed.
- Browser checks on the running local server: 390×844 English chart, 360×800
   Korean alert/backtest, 1440×900 desktop beginner/expert navigation and chart.
- At 390×844, chart canvas top moved from about 829px to 566px; timeframe
   button heights moved from 23px to 44px. No page-wide horizontal overflow.
- At 360×800, Korean Backtest Run was at 404px from the top; alert select was
   294px wide and 44px tall with no page-wide horizontal overflow.
- Onboarding input, general settings, delivery-channel navigation and expert
   OB overlays were visually inspected. No live setting saves/orders/notifications
   were triggered during browser verification.

These checks use desktop Chromium viewports, not physical iOS/Android devices.
Virtual keyboard, safe-area behavior and real-device chart gestures still need
device testing. Browser profile used for checks was isolated from user Chrome.
