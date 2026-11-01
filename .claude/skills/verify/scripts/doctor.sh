#!/usr/bin/env bash
# Read-only health check for the verification instance.
set -euo pipefail

STATE="${VERIFY_STATE:-/tmp/chartnagari-verify-latest/state.env}"
if [[ ! -f "${STATE}" ]]; then
  echo "doctor: no state file at ${STATE}; launch first" >&2
  exit 1
fi
# shellcheck disable=SC1090
source "${STATE}"

echo "run_id=${VERIFY_RUN_ID}"
echo "base_url=${VERIFY_BASE_URL}"
echo "root=${VERIFY_ROOT}"
echo "pid=${VERIFY_PID}"

if [[ ! -d "${VERIFY_ROOT}" ]]; then
  echo "doctor: VERIFY_ROOT missing" >&2
  exit 1
fi

if ! kill -0 "${VERIFY_PID}" 2>/dev/null; then
  echo "doctor: process ${VERIFY_PID} is not running" >&2
  exit 1
fi

BODY="$(curl -fsS "${VERIFY_BASE_URL}/health")"
echo "health=${BODY}"

# Confirm the listening port belongs to our PID (best-effort; macOS/Linux).
if command -v lsof >/dev/null 2>&1; then
  if ! lsof -nP -iTCP:"${VERIFY_PORT}" -sTCP:LISTEN 2>/dev/null | awk 'NR>1 {print $2}' | grep -qx "${VERIFY_PID}"; then
    echo "doctor: port ${VERIFY_PORT} is not owned by pid ${VERIFY_PID}" >&2
    exit 1
  fi
  echo "port_owner=ok"
fi

# Disposable data dir marker.
if [[ "${VERIFY_ROOT}" != /tmp/chartnagari-verify-* ]]; then
  echo "doctor: refusing non-scratch VERIFY_ROOT=${VERIFY_ROOT}" >&2
  exit 1
fi
echo "scratch=ok"
echo "doctor=pass"
