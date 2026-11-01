#!/usr/bin/env bash
# Tear down the verification instance started by launch.sh.
# Does NOT delete proof artifacts under .claude/skills/verify/artifacts/.
set -euo pipefail

STATE="${VERIFY_STATE:-/tmp/chartnagari-verify-latest/state.env}"
KEEP_ROOT="${VERIFY_KEEP_ROOT:-0}"

if [[ ! -f "${STATE}" ]]; then
  echo "cleanup: nothing to do (no ${STATE})"
  exit 0
fi
# shellcheck disable=SC1090
source "${STATE}"

if [[ -n "${VERIFY_PID:-}" ]] && kill -0 "${VERIFY_PID}" 2>/dev/null; then
  kill "${VERIFY_PID}" || true
  for _ in $(seq 1 40); do
    if ! kill -0 "${VERIFY_PID}" 2>/dev/null; then
      break
    fi
    sleep 0.1
  done
  if kill -0 "${VERIFY_PID}" 2>/dev/null; then
    kill -9 "${VERIFY_PID}" || true
  fi
  echo "stopped pid=${VERIFY_PID}"
fi

if [[ "${KEEP_ROOT}" != "1" && -n "${VERIFY_ROOT:-}" && "${VERIFY_ROOT}" == /tmp/chartnagari-verify-* ]]; then
  rm -rf "${VERIFY_ROOT}"
  echo "removed ${VERIFY_ROOT}"
fi

if [[ -L /tmp/chartnagari-verify-latest ]]; then
  target="$(readlink /tmp/chartnagari-verify-latest || true)"
  if [[ -z "${target}" || ! -e /tmp/chartnagari-verify-latest ]]; then
    rm -f /tmp/chartnagari-verify-latest
  fi
fi

echo "cleanup=done"
