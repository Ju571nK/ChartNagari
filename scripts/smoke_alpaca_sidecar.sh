#!/usr/bin/env bash
# scripts/smoke_alpaca_sidecar.sh
#
# End-to-end smoke test for the v2.4.0.0 Alpaca paper-trading sidecar.
#
# This harness:
#   1. verifies required env is set
#   2. builds and starts the sidecar in the background
#   3. waits for /healthz to respond 200
#   4. POSTs a correctly-signed TradeSignal   → expects 202 + order_id
#   5. tampers the HMAC signature              → expects 401
#   6. posts a TradeSignal with bad direction  → expects 422
#   7. replays the golden-path signal_id       → expects 409
#   8. kills the sidecar and cleans up
#   9. prints a PASS/FAIL report
#
# Requires: bash, curl, python3 (stdlib only), go.
# No real secrets are read from this script — credentials come from the user's
# environment (or .env, which godotenv loads inside the binary).
#
# Paths are relative to the repository root. Run from the repo root.

set -u

# ── Colours ──────────────────────────────────────────────────────────────────
if [[ -t 1 ]]; then
  RED=$'\033[0;31m'; GREEN=$'\033[0;32m'; YELLOW=$'\033[0;33m'; BOLD=$'\033[1m'; NC=$'\033[0m'
else
  RED=""; GREEN=""; YELLOW=""; BOLD=""; NC=""
fi

pass_count=0
fail_count=0
report=()

record_pass() { pass_count=$((pass_count + 1)); report+=("${GREEN}PASS${NC}  $1"); }
record_fail() { fail_count=$((fail_count + 1)); report+=("${RED}FAIL${NC}  $1"); }
info()        { printf "%b\n" "${BOLD}[smoke]${NC} $*"; }
warn()        { printf "%b\n" "${YELLOW}[smoke]${NC} $*"; }
err()         { printf "%b\n" "${RED}[smoke]${NC} $*" >&2; }

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

# Best-effort .env load so the user doesn't have to export manually.
if [[ -f .env ]]; then
  info "loading .env"
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

# ── Step 1: required env ─────────────────────────────────────────────────────
REQUIRED=(ALPACA_API_KEY ALPACA_API_SECRET CHARTNAGARI_PLUGIN_SECRET)
missing=()
for v in "${REQUIRED[@]}"; do
  if [[ -z "${!v-}" ]]; then missing+=("$v"); fi
done
if (( ${#missing[@]} > 0 )); then
  err "missing required env vars: ${missing[*]}"
  err "set them in .env or export them, then re-run. Aborting before any live call."
  exit 2
fi

# Defaults (match config.go)
export ALPACA_API_URL="${ALPACA_API_URL:-https://paper-api.alpaca.markets}"
export CHARTNAGARI_PLUGIN_ID="${CHARTNAGARI_PLUGIN_ID:-alpaca-paper}"
export LISTEN_ADDR="${LISTEN_ADDR:-:9100}"
export ALPACA_NOTIONAL_PER_TRADE="${ALPACA_NOTIONAL_PER_TRADE:-1000}"
# Smoke test feedback sink — plain sink so the sidecar has a URL to POST to.
# ChartNagari's main server is NOT required for this harness. A tiny netcat-
# style collector would be nicer; for now we just point it at an unused local
# port. Feedback POST failures are logged by the sidecar but do not affect
# the webhook response codes we are asserting here.
export CHARTNAGARI_FEEDBACK_URL="${CHARTNAGARI_FEEDBACK_URL:-http://127.0.0.1:9199/api/execution/feedback}"
# Isolated DB so a prior run's idempotency rows don't cause a spurious 409.
SMOKE_DB="$(mktemp -t plugin-alpaca-smoke.XXXXXX.db)"
export ALPACA_DB_PATH="$SMOKE_DB"

PORT="${LISTEN_ADDR#:}"
BASE="http://127.0.0.1:${PORT}"

info "ALPACA_API_URL=$ALPACA_API_URL"
info "plugin_id=$CHARTNAGARI_PLUGIN_ID  listen=$LISTEN_ADDR  db=$SMOKE_DB"

# ── Step 2: build & start sidecar ────────────────────────────────────────────
info "building sidecar"
BIN="$(mktemp -t plugin-alpaca.XXXXXX)"
if ! go build -o "$BIN" ./cmd/plugin-alpaca; then
  err "go build failed"; exit 3
fi

info "starting sidecar"
LOG="$(mktemp -t plugin-alpaca-smoke.XXXXXX.log)"
"$BIN" >"$LOG" 2>&1 &
PID=$!

cleanup() {
  info "cleanup: killing sidecar PID=$PID"
  kill "$PID" 2>/dev/null || true
  wait "$PID" 2>/dev/null || true
  rm -f "$BIN" "$SMOKE_DB"
  if [[ -n "${LOG:-}" && -f "$LOG" ]]; then
    info "sidecar log retained at $LOG"
  fi
}
trap cleanup EXIT

# ── Step 3: wait for /healthz ────────────────────────────────────────────────
info "waiting for /healthz"
ready=0
for i in $(seq 1 40); do
  if curl -fsS "$BASE/healthz" >/dev/null 2>&1; then ready=1; break; fi
  if ! kill -0 "$PID" 2>/dev/null; then
    err "sidecar died during startup. last log lines:"
    tail -40 "$LOG" >&2
    exit 4
  fi
  sleep 0.25
done
if (( ready == 0 )); then
  err "sidecar did not become healthy. last log lines:"
  tail -40 "$LOG" >&2
  exit 4
fi
info "sidecar healthy"

# ── HMAC signer (python3, stdlib only) ──────────────────────────────────────
# Canonical format (internal/execution/hmac.go BuildCanonical):
#   plugin_id \n unix_ts \n METHOD \n path \n hex(sha256(body))
sign_header() {
  # args: <body_file> <plugin_id> <timestamp> <method> <path> <secret>
  python3 - "$@" <<'PY'
import hashlib, hmac, sys
body = open(sys.argv[1], "rb").read()
plugin_id, ts, method, path, secret = sys.argv[2], sys.argv[3], sys.argv[4], sys.argv[5], sys.argv[6]
body_hex = hashlib.sha256(body).hexdigest()
canonical = f"{plugin_id}\n{ts}\n{method}\n{path}\n{body_hex}"
sig = hmac.new(secret.encode(), canonical.encode(), hashlib.sha256).hexdigest()
print(sig)
PY
}

uuid() { python3 -c 'import uuid; print(uuid.uuid4())'; }

post_signed() {
  # args: <body_file> <sig_override_or_empty> → prints HTTP status then body
  local body_file="$1"
  local sig_override="${2:-}"
  local ts method path sig
  ts="$(date +%s)"
  method="POST"
  path="/webhook"
  sig="$(sign_header "$body_file" "$CHARTNAGARI_PLUGIN_ID" "$ts" "$method" "$path" "$CHARTNAGARI_PLUGIN_SECRET")"
  if [[ -n "$sig_override" ]]; then sig="$sig_override"; fi
  curl -sS -o /tmp/smoke_resp.$$ -w "%{http_code}" -X POST "$BASE$path" \
    -H "Content-Type: application/json" \
    -H "X-ChartNagari-Plugin-Id: $CHARTNAGARI_PLUGIN_ID" \
    -H "X-ChartNagari-Timestamp: $ts" \
    -H "X-ChartNagari-Signature-256: $sig" \
    --data-binary "@$body_file"
  local rc=$?
  echo
  cat /tmp/smoke_resp.$$
  rm -f /tmp/smoke_resp.$$
  return $rc
}

make_signal() {
  # args: <id> <direction> <symbol>
  local id="$1" dir="$2" sym="$3"
  cat <<JSON
{"id":"$id","version":"1","timestamp":"$(date -u +%Y-%m-%dT%H:%M:%SZ)","symbol":"$sym","direction":"$dir","timeframe":"1h","rule":"smoke-test","entry_price":150.25,"take_profit":155.00,"stop_loss":148.00,"score":85.0,"asset_class":"stock","exchange":"NASDAQ"}
JSON
}

assert_status() {
  # args: <expected_code> <actual_code> <label>
  if [[ "$1" == "$2" ]]; then
    record_pass "$3 → HTTP $2"
  else
    record_fail "$3 → expected HTTP $1, got $2"
  fi
}

# ── Step 4: golden path ──────────────────────────────────────────────────────
info "TEST 1: golden-path signed POST → expect 202"
GOLDEN_ID="$(uuid)"
BODY_GOLDEN="$(mktemp)"
make_signal "$GOLDEN_ID" "LONG" "SPY" > "$BODY_GOLDEN"
RESP_GOLDEN="$(post_signed "$BODY_GOLDEN" "" 2>&1)"
CODE_GOLDEN="$(printf "%s" "$RESP_GOLDEN" | head -n1)"
BODY_RESP_GOLDEN="$(printf "%s" "$RESP_GOLDEN" | tail -n +2)"
info "response: $CODE_GOLDEN  body=$BODY_RESP_GOLDEN"
assert_status "202" "$CODE_GOLDEN" "golden path"
if [[ "$CODE_GOLDEN" == "202" ]]; then
  if echo "$BODY_RESP_GOLDEN" | grep -q '"order_id"'; then
    record_pass "golden path response contains order_id"
  else
    record_fail "golden path response missing order_id: $BODY_RESP_GOLDEN"
  fi
else
  warn "golden path non-202 — usually means Alpaca rejected the order (check credentials or market hours)."
  warn "sidecar log tail:"
  tail -20 "$LOG" >&2 || true
fi

# ── Step 5: tampered HMAC → 401 ──────────────────────────────────────────────
info "TEST 2: tampered HMAC → expect 401"
BODY_TAMPER="$(mktemp)"
make_signal "$(uuid)" "LONG" "SPY" > "$BODY_TAMPER"
RESP_TAMPER="$(post_signed "$BODY_TAMPER" "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef" 2>&1)"
CODE_TAMPER="$(printf "%s" "$RESP_TAMPER" | head -n1)"
info "response: $CODE_TAMPER"
assert_status "401" "$CODE_TAMPER" "tampered HMAC"

# ── Step 6: bad direction → 422 ──────────────────────────────────────────────
info "TEST 3: bad direction 'SIDEWAYS' → expect 422"
BODY_BADDIR="$(mktemp)"
make_signal "$(uuid)" "SIDEWAYS" "SPY" > "$BODY_BADDIR"
RESP_BADDIR="$(post_signed "$BODY_BADDIR" "" 2>&1)"
CODE_BADDIR="$(printf "%s" "$RESP_BADDIR" | head -n1)"
info "response: $CODE_BADDIR"
assert_status "422" "$CODE_BADDIR" "bad direction"

# ── Step 7: replay same signal_id → 409 ──────────────────────────────────────
info "TEST 4: replay golden signal_id → expect 409"
RESP_REPLAY="$(post_signed "$BODY_GOLDEN" "" 2>&1)"
CODE_REPLAY="$(printf "%s" "$RESP_REPLAY" | head -n1)"
info "response: $CODE_REPLAY"
assert_status "409" "$CODE_REPLAY" "replay duplicate"

rm -f "$BODY_GOLDEN" "$BODY_TAMPER" "$BODY_BADDIR"

# ── Step 8: report ───────────────────────────────────────────────────────────
echo
printf "%b\n" "${BOLD}── Smoke test report ──${NC}"
for line in "${report[@]}"; do printf "%b\n" "$line"; done
echo
total=$((pass_count + fail_count))
if (( fail_count == 0 )); then
  printf "%b\n" "${GREEN}${BOLD}ALL ${total} CHECKS PASSED${NC}"
  exit 0
else
  printf "%b\n" "${RED}${BOLD}${fail_count}/${total} CHECKS FAILED${NC}"
  exit 1
fi
