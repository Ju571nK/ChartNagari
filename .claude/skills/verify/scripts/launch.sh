#!/usr/bin/env bash
# Launch an isolated ChartNagari instance for verification.
# Prints RUN_ID and exports state under /tmp/chartnagari-verify-<RUN_ID>/.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
RUN_ID="${VERIFY_RUN_ID:-$(date -u +%Y%m%dT%H%M%SZ)-$$}"
ROOT="${VERIFY_ROOT:-/tmp/chartnagari-verify-${RUN_ID}}"
PORT="${VERIFY_PORT:-}"
BIN_DIR="${VERIFY_BIN_DIR:-/tmp/chartnagari-verify-bin}"
LOG="${ROOT}/server.log"
PID_FILE="${ROOT}/server.pid"
STATE_FILE="${ROOT}/state.env"

if [[ -n "${PORT}" ]]; then
  :
else
  PORT=18080
  while curl -fsS "http://127.0.0.1:${PORT}/health" >/dev/null 2>&1; do
    PORT=$((PORT + 1))
    if [[ "${PORT}" -gt 18180 ]]; then
      echo "no free port in 18080-18180" >&2
      exit 1
    fi
  done
fi

mkdir -p "${ROOT}/config" "${ROOT}/data" "${BIN_DIR}"
cp "${REPO_ROOT}/config/rules.yaml" "${ROOT}/config/"
cp "${REPO_ROOT}/config/alert.yaml" "${ROOT}/config/"
cp "${REPO_ROOT}/config/report.yaml" "${ROOT}/config/"
cp "${REPO_ROOT}/config/execution.yaml" "${ROOT}/config/"
cp "${REPO_ROOT}/config/signal_tuning.yaml" "${ROOT}/config/"
cp "${REPO_ROOT}/config/symbol_profiles.yaml" "${ROOT}/config/"
cp "${REPO_ROOT}/config/settings.example.yaml" "${ROOT}/config/settings.yaml"
chmod 600 "${ROOT}/config/settings.yaml"

# Disposable empty watchlist (DXY init may still add an index symbol).
cat > "${ROOT}/config/watchlist.yaml" <<'YAML'
symbols:
    crypto: []
    stocks: []
    indices: []
    forex: []
timeframes:
    - 1W
    - 1D
    - 4H
    - 1H
YAML

# Point settings at this scratch data dir and free port.
python3 - <<PY
from pathlib import Path
p = Path("${ROOT}/config/settings.yaml")
text = p.read_text()
text = text.replace('host: "127.0.0.1"', 'host: "127.0.0.1"')
text = text.replace('port: "8080"', 'port: "${PORT}"')
text = text.replace('path: ./data/chart_analyzer.db', 'path: ./data/chart_analyzer.db')
text = text.replace('env: development', 'env: development')
text = text.replace('log_level: debug', 'log_level: info')
p.write_text(text)
PY

if [[ ! -d "${REPO_ROOT}/web/dist" ]]; then
  echo "web/dist missing; run: npm --prefix web ci && npm --prefix web run build" >&2
  exit 1
fi
# Server serves relative path web/dist from cwd.
ln -sfn "${REPO_ROOT}/web" "${ROOT}/web"

BIN="${BIN_DIR}/chart-analyzer"
if [[ ! -x "${BIN}" ]] || [[ "${REPO_ROOT}/cmd/server" -nt "${BIN}" ]]; then
  (cd "${REPO_ROOT}" && go build -o "${BIN}" ./cmd/server)
fi

cd "${ROOT}"
nohup "${BIN}" >"${LOG}" 2>&1 &
echo $! >"${PID_FILE}"

ready=0
for _ in $(seq 1 60); do
  if curl -fsS "http://127.0.0.1:${PORT}/health" >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 0.25
done
if [[ "${ready}" -ne 1 ]]; then
  echo "server failed to become ready; log: ${LOG}" >&2
  tail -n 80 "${LOG}" >&2 || true
  exit 1
fi

{
  echo "VERIFY_RUN_ID=${RUN_ID}"
  echo "VERIFY_ROOT=${ROOT}"
  echo "VERIFY_PORT=${PORT}"
  echo "VERIFY_BASE_URL=http://127.0.0.1:${PORT}"
  echo "VERIFY_PID=$(cat "${PID_FILE}")"
  echo "VERIFY_LOG=${LOG}"
  echo "VERIFY_REPO=${REPO_ROOT}"
} >"${STATE_FILE}"

# Symlink latest for doctor/cleanup convenience.
ln -sfn "${ROOT}" /tmp/chartnagari-verify-latest

echo "VERIFY_RUN_ID=${RUN_ID}"
echo "VERIFY_BASE_URL=http://127.0.0.1:${PORT}"
echo "VERIFY_ROOT=${ROOT}"
echo "VERIFY_STATE=${STATE_FILE}"
