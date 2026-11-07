#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LAUNCH="${SCRIPT_DIR}/launch.sh"

DECISION="$(
  awk '
    $0 ~ /^BIN="\$\{BIN_DIR\}\/chart-analyzer"$/ { capture = 1 }
    capture { print }
    capture && $0 ~ /^fi$/ { exit }
  ' "${LAUNCH}"
)"

case "${DECISION}" in
  *'go build -o "${BIN}" ./cmd/server'*) ;;
  *)
    printf 'assert-rebuild: rebuild decision in launch.sh does not call go build\n' >&2
    exit 1
    ;;
esac

WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT

fresh_repo() {
  local name="$1"
  local repo="${WORK}/${name}"
  mkdir -p "${repo}/cmd/server" "${repo}/internal/pkg"
  printf 'package main\n' > "${repo}/cmd/server/main.go"
  printf 'package pkg\n' > "${repo}/internal/pkg/a.go"
  printf 'note\n' > "${repo}/internal/pkg/note.txt"
  touch -t 202001010000 \
    "${repo}/cmd/server/main.go" \
    "${repo}/internal/pkg/a.go" \
    "${repo}/internal/pkg/note.txt" \
    "${repo}/cmd/server" \
    "${repo}/internal" \
    "${repo}/internal/pkg"
  printf '%s\n' "${repo}"
}

fresh_bin_dir() {
  local name="$1"
  local dir="${WORK}/${name}-bin"
  mkdir -p "${dir}"
  printf '#!/bin/sh\nexit 0\n' > "${dir}/chart-analyzer"
  chmod +x "${dir}/chart-analyzer"
  touch -t 202001010100 "${dir}/chart-analyzer"
  printf '%s\n' "${dir}"
}

require_dir_not_newer() {
  local repo="$1"
  local bin="$2"
  if [[ "${repo}/cmd/server" -nt "${bin}" ]]; then
    printf 'assert-rebuild: fixture error: cmd/server directory is newer than the bin\n' >&2
    exit 2
  fi
}

require_go_newer() {
  local repo="$1"
  local bin="$2"
  if ! find "${repo}/cmd/server" "${repo}/internal" -name '*.go' -newer "${bin}" -print -quit | grep -q .; then
    printf 'assert-rebuild: fixture error: no .go file is newer than the bin\n' >&2
    exit 2
  fi
}

run_decision() {
  local repo="$1"
  local bin_dir="$2"
  REPO_ROOT="${repo}"
  BIN_DIR="${bin_dir}"
  MARKER="${WORK}/go-build-marker"
  : > "${MARKER}"
  go() {
    local out="" prev="" arg
    if [[ "${1:-}" != "build" ]]; then
      printf 'assert-rebuild: unexpected go invocation: %s\n' "$*" >&2
      return 1
    fi
    for arg in "$@"; do
      if [[ "${prev}" == "-o" ]]; then
        out="${arg}"
      fi
      prev="${arg}"
    done
    if [[ -z "${out}" ]]; then
      printf 'assert-rebuild: go build missing -o\n' >&2
      return 1
    fi
    printf 'build\n' >> "${MARKER}"
    printf '#!/bin/sh\nexit 0\n' > "${out}"
    chmod +x "${out}"
  }
  eval "${DECISION}"
  if [[ -s "${MARKER}" ]]; then
    BUILT=1
  else
    BUILT=0
  fi
}

failures=0
expect() {
  local want="$1"
  local label="$2"
  local got="skip"
  if [[ "${BUILT}" -eq 1 ]]; then
    got="build"
  fi
  if [[ "${got}" != "${want}" ]]; then
    printf 'assert-rebuild: %s: expected %s, observed %s\n' "${label}" "${want}" "${got}" >&2
    failures=$((failures + 1))
  else
    printf 'assert-rebuild: ok: %s\n' "${label}"
  fi
}

repo="$(fresh_repo internal-newer)"
bin_dir="$(fresh_bin_dir internal-newer)"
touch -t 202001010200 "${repo}/internal/pkg/a.go"
require_dir_not_newer "${repo}" "${bin_dir}/chart-analyzer"
require_go_newer "${repo}" "${bin_dir}/chart-analyzer"
run_decision "${repo}" "${bin_dir}"
expect build "internal .go newer than bin"

repo="$(fresh_repo server-go-newer)"
bin_dir="$(fresh_bin_dir server-go-newer)"
touch -t 202001010200 "${repo}/cmd/server/main.go"
touch -t 202001010000 "${repo}/cmd/server"
require_dir_not_newer "${repo}" "${bin_dir}/chart-analyzer"
require_go_newer "${repo}" "${bin_dir}/chart-analyzer"
run_decision "${repo}" "${bin_dir}"
expect build "cmd/server .go newer than bin while the directory is older"

repo="$(fresh_repo dir-only)"
bin_dir="$(fresh_bin_dir dir-only)"
touch -t 202001010300 "${repo}/cmd/server"
run_decision "${repo}" "${bin_dir}"
expect skip "cmd/server directory newer than bin while .go files are older"

repo="$(fresh_repo note-only)"
bin_dir="$(fresh_bin_dir note-only)"
touch -t 202001010200 "${repo}/internal/pkg/note.txt"
run_decision "${repo}" "${bin_dir}"
expect skip "non-go file newer than bin"

repo="$(fresh_repo sources-older)"
bin_dir="$(fresh_bin_dir sources-older)"
run_decision "${repo}" "${bin_dir}"
expect skip "bin newer than every .go file"

repo="$(fresh_repo missing-bin)"
bin_dir="${WORK}/missing-bin-dir"
mkdir -p "${bin_dir}"
run_decision "${repo}" "${bin_dir}"
expect build "bin missing"

repo="$(fresh_repo non-executable)"
bin_dir="$(fresh_bin_dir non-executable)"
chmod a-x "${bin_dir}/chart-analyzer"
run_decision "${repo}" "${bin_dir}"
expect build "bin not executable"

if [[ "${failures}" -ne 0 ]]; then
  exit 1
fi
