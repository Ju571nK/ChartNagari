# Getting started / 설치 안내

These instructions target **v2.13.0.0**. Older releases can have
different configuration behavior. Existing users should read [migration notes](../SETTINGS.md)
first and must not replace their settings with the example.

## Try without installing

[Open the demo](https://ju571nk.github.io/ChartNagari/). It uses bundled samples and
does not connect a brokerage account. Some server-backed functions are unavailable.

## Run locally

Requires Go 1.26+ and Node.js 20+. From a new checkout:

```sh
git clone https://github.com/Ju571nK/ChartNagari.git
cd ChartNagari
cp config/settings.example.yaml config/settings.yaml
chmod 600 config/settings.yaml
npm --prefix web ci
npm --prefix web run build
go run ./cmd/server
```

Open [http://localhost:8080](http://localhost:8080). No alert or AI credentials are
required just to start the server. Market-data availability depends on the provider.

처음 설치할 때만 예제 설정 파일을 복사하세요. 기존 사용자는 설정을 덮어쓰지 말고
[이전 안내](../SETTINGS.md)를 따르세요. 실행 후 웹 **Symbols → Chart → Settings** 순서로
관심 종목과 필요한 서비스를 설정할 수 있습니다.

## Run with Docker

Requires Docker Compose and Node.js 20+ for the host-mounted frontend build.
First clone the repository, copy the example settings for a **new install** and
run the two npm commands above. In `config/settings.yaml`, set:

```yaml
server:
  host: "0.0.0.0"
  port: "8080"
database:
  path: ./data/chart_analyzer.db
```

Edit these fields in the existing YAML; do not replace the whole file with this
partial example. Keep its `version: 1` marker and other settings. Then run:

```sh
docker compose up -d --build
```

The Compose file mounts `web/dist` from the host, so build it before starting.
Host access is restricted to `127.0.0.1:8080`; the container itself must bind to
`0.0.0.0`. No `.env` is required. The container sees the mounted configuration
and data directories, not arbitrary host paths.

## First use

1. Add a stock ticker or crypto pair in **Symbols**. Registration does not mean
   historical data is already available; check the chart's data status.
2. Choose a timeframe in **Chart**. Start in beginner mode; use expert mode to
   inspect additional overlays.
3. Connect optional data, AI and notification services in **Settings**.
4. Restart the relevant process after changing general settings. If API-token
   authentication is enabled, use **Authorize changes** in Settings to save edits.

Do not expose an unauthenticated server publicly. Review execution configuration
separately; nothing in these instructions enables an execution adapter.

[Detailed settings](../SETTINGS.md) · [Development setup](../CONTRIBUTING.md)
