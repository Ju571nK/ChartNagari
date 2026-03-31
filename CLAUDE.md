# CLAUDE.md — ChartNagari AI Contributor Guide

> When Claude Code opens this project, read this file first.

## Project Summary

ChartNagari is a local-run platform that automatically detects ICT/Wyckoff and general TA signals
across multiple timeframes (1W/1D/4H/1H) for US stocks and crypto, and sends alerts via Telegram/Discord.
Go backend + TypeScript/React frontend.

## Tech Stack

- Backend  : Go 1.26+
- Frontend : TypeScript + React 18 + Vite
- DB       : SQLite (local)
- Alerts   : Telegram Bot / Discord Webhook
- Data     : Yahoo Finance (stocks) / Binance WebSocket (crypto)

## Contribution Principles

- Run `go test ./...` before submitting any code change — all tests must pass.
- Follow existing patterns: zerolog structured logging, context propagation, table-driven tests.
- New trading rules go in `internal/methodology/<category>/` and must be registered in `config/rules.yaml`.
- Do not commit `.env` or any file containing real API keys.
- Update `CHANGELOG.md` with a brief entry under the appropriate version/date heading.

## Agent Team

This project uses a multi-agent team architecture. The `orchestrator` agent coordinates 4 specialists:

- **go-backend** — Go code (cmd/, internal/)
- **react-frontend** — React/TS code (web/src/)
- **trading-analyst** — Trading rule design & tuning (internal/methodology/)
- **release-engineer** — VERSION, CHANGELOG, PRs, CI

For cross-domain tasks, spawn the orchestrator which will create a team and dispatch work.
For single-domain tasks, spawn the specialist directly.

Agent definitions: `.claude/agents/`

## gstack
Use /browse from gstack for all web browsing.
Never use mcp__claude-in-chrome__* tools.
Available skills: /office-hours, /plan-ceo-review, /plan-eng-review,
 /plan-design-review, /design-consultation, /review, /ship, /browse, /qa,
 /qa-only, /design-review, /setup-browser-cookies, /retro, /investigate,
 /document-release, /codex, /careful, /freeze, /guard, /unfreeze,
 /gstack-upgrade