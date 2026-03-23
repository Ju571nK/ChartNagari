# best-of-algorithmic-trading PR Draft

**Target repo:** https://github.com/wcarhart/best-of-algorithmic-trading
(or https://github.com/futekov/best-of-algotrading — check which is most active/maintained)

**Prerequisite:** 10+ GitHub stars

---

## PR Title
> Add: ChartNagari — open-source ICT/Wyckoff multi-timeframe signal detector (Go)

---

## PR Body

### What is ChartNagari?

[ChartNagari](https://github.com/Ju571nK/ChartNagari) is a self-hosted, real-time signal detection platform focused on ICT (Inner Circle Trader) and Wyckoff methodology, built in Go.

**Category suggestion:** `Signal Generation` or `Technical Analysis Indicators`

### Why it belongs here

The ICT and Wyckoff methodology space is significantly underrepresented in open-source algorithmic trading tools. Most existing libraries are:
- Single-language (Python) with no alerting layer
- Backtesting-only (no real-time scanning)
- Single-timeframe

ChartNagari is the only open-source project that combines:
1. **Real-time multi-timeframe scanning** (1W / 1D / 4H / 1H) across both stocks and crypto
2. **ICT + Wyckoff rule library** (14+ rules) with a clean, extensible interface
3. **Alert delivery** (Telegram / Discord) with cooldown and multi-timeframe consensus scoring
4. **Self-hosted** — no cloud accounts, single `docker compose up`

### Project metadata

| Field | Value |
|---|---|
| **Name** | ChartNagari |
| **URL** | https://github.com/Ju571nK/ChartNagari |
| **License** | MIT |
| **Language** | Go (backend), TypeScript/React (frontend) |
| **Category** | Signal Generation / ICT & Wyckoff |
| **Last active** | Active (2026) |
| **Stars** | (current count) |

### projects.yaml entry (if applicable)

```yaml
- name: ChartNagari
  github_id: Ju571nK/ChartNagari
  category: signal-generation
  labels: [ict, wyckoff, multi-timeframe, go, self-hosted]
  description: >
    Real-time ICT and Wyckoff signal detector. Scans stocks and crypto across
    1W/1D/4H/1H simultaneously, fires Telegram/Discord alerts, optional AI
    interpretation. Self-hosted, MIT, Go + React.
```

---

## Notes for submitter (Justin)

1. Before opening: confirm the repo is maintained and accepts PRs (check last merged PR date)
2. Search the list first: make sure ChartNagari isn't already listed
3. Check if they use `projects.yaml` format or a README table format — adjust accordingly
4. Possible alternative lists to also submit to:
   - https://github.com/edarchimbaud/awesome-systematic-trading
   - https://github.com/wilsonfreitas/awesome-quant
   - https://github.com/josephmisiti/awesome-machine-learning (under Finance section)
5. Star threshold: 10★ is generally the minimum for these lists to accept. Check list maintainer's CONTRIBUTING for their actual requirement.
