// Package execution delivers TradeSignal envelopes to external plugins and
// accepts asynchronous OrderFeedback callbacks. HMAC-SHA256 is the shared
// authentication primitive for both directions.
package execution

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

// SignatureHeader is the fixed header name used for both outbound dispatch
// signatures and inbound feedback signatures. Codex #1 requires a single
// canonical string format — pluginID\ntimestamp\nmethod\npath\nhex(sha256(body)).
const (
	SignatureHeader = "X-ChartNagari-Signature-256"
	TimestampHeader = "X-ChartNagari-Timestamp"
	PluginIDHeader  = "X-ChartNagari-Plugin-Id"
)

// BuildCanonical produces the HMAC canonical string from request components.
// Codex #1: every field separated by a single \n; body is prehashed with SHA-256
// and hex-encoded so the signature does not require re-reading the raw body.
//
//	plugin_id\n
//	timestamp\n
//	method\n
//	path\n
//	hex(sha256(body))
func BuildCanonical(pluginID string, timestamp int64, method, path string, body []byte) string {
	sum := sha256.Sum256(body)
	bodyHex := hex.EncodeToString(sum[:])
	return pluginID + "\n" +
		strconv.FormatInt(timestamp, 10) + "\n" +
		method + "\n" +
		path + "\n" +
		bodyHex
}

// Sign computes the hex-encoded HMAC-SHA256 for the given canonical inputs.
func Sign(secret string, pluginID string, timestamp int64, method, path string, body []byte) string {
	canonical := BuildCanonical(pluginID, timestamp, method, path, body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify performs a constant-time comparison of `provided` against the
// expected HMAC for the canonical inputs. Codex #1: hmac.Equal is mandatory to
// avoid timing attacks. Callers MUST pass the raw body bytes (no JSON
// re-marshal) so the SHA-256 sum matches what was signed.
func Verify(secret string, pluginID string, timestamp int64, method, path string, body []byte, provided string) bool {
	if secret == "" || provided == "" {
		return false
	}
	expected := Sign(secret, pluginID, timestamp, method, path, body)
	return hmac.Equal([]byte(expected), []byte(provided))
}

// WithinSkew reports whether the signed timestamp is inside the accepted
// clock-skew window. Codex #7: window is configurable, not hardcoded.
// skewSec <= 0 falls back to 300 seconds.
func WithinSkew(timestamp int64, skewSec int, now time.Time) bool {
	if skewSec <= 0 {
		skewSec = 300
	}
	diff := now.Unix() - timestamp
	if diff < 0 {
		diff = -diff
	}
	return diff <= int64(skewSec)
}

// ParseTimestamp parses a decimal unix-seconds header value.
func ParseTimestamp(raw string) (int64, error) {
	if raw == "" {
		return 0, fmt.Errorf("empty timestamp")
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid timestamp %q: %w", raw, err)
	}
	return v, nil
}
