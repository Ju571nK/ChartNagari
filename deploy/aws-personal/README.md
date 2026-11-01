# Personal always-on AWS bundle

Phase-1 files for a single-operator ChartNagari host. Full procedure:
[docs/aws-personal-always-on.md](../../docs/aws-personal-always-on.md).

| File | Purpose |
| --- | --- |
| `docker-compose.yml` | Server + Caddy stack (image UI; no host `web/dist` mount) |
| `Caddyfile.example` | Copy to `Caddyfile` and set your hostname |
| `settings.remote.snippet.yaml` | Keys to merge into `config/settings.yaml` |
| `cost-note.template.md` | Fill after month one to record personal AWS cost |

Backup/restore scripts: [`scripts/aws-personal/`](../../scripts/aws-personal/).
