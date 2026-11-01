#!/usr/bin/env bash
# Restore ChartNagari data/ and config/ from an S3 object or local tarball.
# Stop the app before restore. Overwrites existing data/ and config/.
#
# Usage:
#   export CHARTNAGARI_ROOT=/opt/chartnagari
#   ./scripts/aws-personal/restore.sh s3://bucket/prefix/chartnagari-YYYYMMDDTHHMMSSZ.tar.gz
#   # or:
#   ./scripts/aws-personal/restore.sh /path/to/chartnagari-YYYYMMDDTHHMMSSZ.tar.gz
#   docker compose -f deploy/aws-personal/docker-compose.yml up -d

set -euo pipefail

ROOT="${CHARTNAGARI_ROOT:-}"
if [[ -z "${ROOT}" ]]; then
  ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
fi

SRC="${1:-}"
if [[ -z "${SRC}" ]]; then
  echo "usage: $0 <s3://bucket/key.tar.gz|/path/to/archive.tar.gz>" >&2
  exit 1
fi

COMPOSE_FILE="${COMPOSE_FILE:-${ROOT}/deploy/aws-personal/docker-compose.yml}"
TMP_DIR="$(mktemp -d)"
ARCHIVE="${TMP_DIR}/restore.tar.gz"

cleanup() {
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

if [[ "${SRC}" == s3://* ]]; then
  aws s3 cp "${SRC}" "${ARCHIVE}"
else
  cp "${SRC}" "${ARCHIVE}"
fi

if [[ -f "${COMPOSE_FILE}" ]]; then
  docker compose -f "${COMPOSE_FILE}" stop server >/dev/null 2>&1 || true
fi

mkdir -p "${ROOT}"
# Replace data and config trees from the archive roots.
tar -C "${ROOT}" -xzf "${ARCHIVE}" data config
chmod 700 "${ROOT}/config" 2>/dev/null || true
chmod 600 "${ROOT}/config/settings.yaml" 2>/dev/null || true

echo "restored data/ and config/ under ${ROOT}"
echo "start the stack when ready:"
echo "  docker compose -f ${COMPOSE_FILE} up -d"
