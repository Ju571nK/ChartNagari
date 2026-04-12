package execution

import (
	"strings"
	"testing"
	"time"
)

// TestBuildCanonical_Format locks in the exact canonical format Codex #1
// mandates: five fields separated by a single \n, body pre-hashed to hex.
func TestBuildCanonical_Format(t *testing.T) {
	body := []byte(`{"signal_id":"abc"}`)
	got := BuildCanonical("plugin-1", 1700000000, "POST", "/api/execution/feedback", body)

	lines := strings.Split(got, "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 fields separated by \\n, got %d: %q", len(lines), got)
	}
	if lines[0] != "plugin-1" {
		t.Errorf("plugin_id = %q, want plugin-1", lines[0])
	}
	if lines[1] != "1700000000" {
		t.Errorf("timestamp = %q, want 1700000000", lines[1])
	}
	if lines[2] != "POST" {
		t.Errorf("method = %q, want POST", lines[2])
	}
	if lines[3] != "/api/execution/feedback" {
		t.Errorf("path = %q, want /api/execution/feedback", lines[3])
	}
	// hex sha256 is 64 chars.
	if len(lines[4]) != 64 {
		t.Errorf("body hash length = %d, want 64", len(lines[4]))
	}
}

// TestSignVerify_RoundTrip confirms Sign + Verify agree on a realistic payload.
func TestSignVerify_RoundTrip(t *testing.T) {
	secret := "s3cr3t"
	body := []byte(`{"id":"x","score":10}`)
	sig := Sign(secret, "p1", 1700000000, "POST", "/api/dispatch", body)

	if !Verify(secret, "p1", 1700000000, "POST", "/api/dispatch", body, sig) {
		t.Fatal("Verify returned false for valid signature")
	}
}

// TestVerify_TamperedBody fails closed when the body has been altered even by
// a single byte (Codex #1 canonical inclusion of hex(sha256(body))).
func TestVerify_TamperedBody(t *testing.T) {
	secret := "s3cr3t"
	body := []byte(`{"id":"x"}`)
	sig := Sign(secret, "p1", 1700000000, "POST", "/api/dispatch", body)

	tampered := []byte(`{"id":"y"}`)
	if Verify(secret, "p1", 1700000000, "POST", "/api/dispatch", tampered, sig) {
		t.Fatal("Verify accepted tampered body")
	}
}

// TestVerify_WrongSecret ensures a different secret does not validate.
func TestVerify_WrongSecret(t *testing.T) {
	body := []byte(`{}`)
	sig := Sign("correct", "p1", 1700000000, "POST", "/", body)
	if Verify("wrong", "p1", 1700000000, "POST", "/", body, sig) {
		t.Fatal("Verify accepted wrong secret")
	}
}

// TestVerify_EmptyInputs rejects empty secret or provided signature.
func TestVerify_EmptyInputs(t *testing.T) {
	body := []byte(`{}`)
	if Verify("", "p1", 0, "POST", "/", body, "deadbeef") {
		t.Error("empty secret must not verify")
	}
	if Verify("s", "p1", 0, "POST", "/", body, "") {
		t.Error("empty signature must not verify")
	}
}

// TestWithinSkew_Boundary checks the skew window inclusive boundaries.
func TestWithinSkew_Boundary(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	// exactly at boundary: within
	if !WithinSkew(now.Unix()-300, 300, now) {
		t.Error("-300s should be within 300s window")
	}
	if !WithinSkew(now.Unix()+300, 300, now) {
		t.Error("+300s should be within 300s window")
	}
	// just outside
	if WithinSkew(now.Unix()-301, 300, now) {
		t.Error("-301s should be outside")
	}
	if WithinSkew(now.Unix()+301, 300, now) {
		t.Error("+301s should be outside")
	}
	// default fallback when <= 0
	if !WithinSkew(now.Unix()-299, 0, now) {
		t.Error("skewSec <= 0 should use 300s default")
	}
}

// TestParseTimestamp_Valid accepts positive decimal seconds.
func TestParseTimestamp_Valid(t *testing.T) {
	v, err := ParseTimestamp("1700000000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 1700000000 {
		t.Fatalf("got %d, want 1700000000", v)
	}
}

// TestParseTimestamp_Invalid rejects empty and non-numeric.
func TestParseTimestamp_Invalid(t *testing.T) {
	if _, err := ParseTimestamp(""); err == nil {
		t.Error("empty timestamp should error")
	}
	if _, err := ParseTimestamp("not-a-number"); err == nil {
		t.Error("non-numeric should error")
	}
}
