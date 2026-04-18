package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
)

// requireBearer enforces bearer-token authentication for sensitive GET endpoints
// that are exempt from the global authMiddleware (which skips MethodGet).
//
// It returns true when the request is authorized — the caller may proceed.
// It returns false after writing a 401 Unauthorized JSON response — the caller
// must return immediately.
//
// When s.apiToken is empty (dev mode / no token configured) the function is a
// no-op and always returns true, preserving backward compatibility.
//
// Token comparison uses crypto/subtle.ConstantTimeCompare to prevent timing
// oracle attacks. If the lengths differ the comparison short-circuits early
// (length mismatch cannot be exploited as a timing side-channel because it
// reveals no information about the token value).
func (s *Server) requireBearer(w http.ResponseWriter, r *http.Request) bool {
	if s.apiToken == "" {
		return true
	}

	authHeader := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		writeUnauthorized(w)
		return false
	}

	provided := authHeader[len(prefix):]
	expected := s.apiToken

	// Reject immediately on length mismatch — no timing information is leaked
	// because the expected length is not secret (the prefix is fixed).
	if len(provided) != len(expected) {
		writeUnauthorized(w)
		return false
	}

	if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
		writeUnauthorized(w)
		return false
	}

	return true
}

// writeUnauthorized writes a 401 Unauthorized response with a JSON error body.
func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
}
