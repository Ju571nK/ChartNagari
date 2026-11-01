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

- `config/settings.yaml` stores `server.api_token` and provider credentials on disk. `.gitignore` excludes that file. A person who can read the file can use the token.
- Set `server.remote_access` to `"true"` only with a token of at least 32 characters. The server refuses to start otherwise. In that mode every `/api/` request except `OPTIONS` must send `Authorization: Bearer` and the token. The token is an administrator credential. Terminate HTTPS in front of the server.
- When `remote_access` is off and `api_token` is empty, local read APIs stay open. Do not expose that server on the public internet.
- **Local SQLite only.** The database stores OHLCV bars and signal metadata. No personal data is collected.
