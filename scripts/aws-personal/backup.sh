#!/usr/bin/env bash
# Backup ChartNagari data/ and config/ for a personal always-on host.
# Requires: tar, aws CLI (for S3 upload), optional docker compose.
#
# Usage:
#   export CHARTNAGARI_ROOT=/opt/chartnagari
#   export BACKUP_S3_URI=s3://your-bucket/chartnagari/personal
#   # optional: BACKUP_STOP_CONTAINER=1 for a colder SQLite copy
#   ./scripts/aws-personal/backup.sh

set -euo pipefail

ROOT="${CHARTNAGARI_ROOT:-}"
if [[ -z "${ROOT}" ]]; then
  ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
fi

S3_URI="${BACKUP_S3_URI:-}"
if [[ -z "${S3_URI}" ]]; then
  echo "BACKUP_S3_URI is required (e.g. s3://bucket/prefix)" >&2
  exit 1
fi

COMPOSE_FILE="${COMPOSE_FILE:-${ROOT}/deploy/aws-personal/docker-compose.yml}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
TMP_DIR="$(mktemp -d)"
ARCHIVE="${TMP_DIR}/chartnagari-${STAMP}.tar.gz"
STOPPED=0

cleanup() {
  if [[ "${STOPPED}" -eq 1 ]]; then
    docker compose -f "${COMPOSE_FILE}" start server >/dev/null 2>&1 || true
  fi
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

if [[ ! -d "${ROOT}/data" || ! -d "${ROOT}/config" ]]; then
  echo "expected data/ and config/ under ${ROOT}" >&2
  exit 1
fi

if [[ "${BACKUP_STOP_CONTAINER:-0}" == "1" ]]; then
  docker compose -f "${COMPOSE_FILE}" stop server
  STOPPED=1
fi

# config may contain API tokens and provider secrets — treat archives as secret.
tar -C "${ROOT}" -czf "${ARCHIVE}" data config

aws s3 cp "${ARCHIVE}" "${S3_URI}/chartnagari-${STAMP}.tar.gz"
echo "uploaded ${S3_URI}/chartnagari-${STAMP}.tar.gz"
