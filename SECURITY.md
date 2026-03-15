# Security Policy

## Reporting a Vulnerability

Please **do not** open a public GitHub issue for security vulnerabilities.

Use [GitHub Private Vulnerability Reporting](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing/privately-reporting-a-security-vulnerability) instead. We aim to acknowledge reports within 72 hours.

## Scope

- SQL injection or data corruption via API endpoints
- Authentication or authorization bypasses
- Remote code execution

## Out of Scope

- Denial-of-service via extremely large inputs (no network-facing auth surface)
- Issues requiring physical access to the host machine
- Third-party dependencies — please report those upstream

## Data Handling Notes

- **No API keys are stored server-side.** All credentials are read from `.env` at startup and held in process memory only.
- **Local SQLite only.** The database stores OHLCV bars and signal metadata. No personal data is collected.
- The server is designed for local or private-network deployment. Exposing it to the public internet without additional authentication is not recommended.
