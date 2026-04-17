package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/execution"
)

// getExecutionConfig returns the current execution config with plugin secrets
// redacted (A5). Secrets never leave the server in clear form.
func (s *Server) getExecutionConfig(w http.ResponseWriter, r *http.Request) {
	if s.execHolder == nil {
		http.Error(w, "execution config not enabled", http.StatusNotFound)
		return
	}
	cfg := s.execHolder.Get().RedactSecrets()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(cfg); err != nil {
		log.Warn().Err(err).Msg("api: encode execution config failed")
	}
}

// updateExecutionConfig accepts a full ExecutionConfig, merges in existing
// secrets for any plugin field sent as "" or "***" (A5), validates, persists
// atomically to disk, then flips the in-memory holder.
func (s *Server) updateExecutionConfig(w http.ResponseWriter, r *http.Request) {
	if s.execHolder == nil || s.execPath == "" {
		http.Error(w, "execution config not enabled", http.StatusNotFound)
		return
	}
	var incoming appconfig.ExecutionConfig
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	current := s.execHolder.Get()
	merged := appconfig.MergeIncomingSecrets(current, incoming)
	if err := merged.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := appconfig.SaveExecutionConfig(s.execPath, merged); err != nil {
		log.Error().Err(err).Msg("api: save execution config failed")
		http.Error(w, "persist failed", http.StatusInternalServerError)
		return
	}
	s.execHolder.Set(merged)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(merged.RedactSecrets())
}

// toggleExecutionKill flips the kill switch. Disk-first (Codex #10): the
// ExecutionHolder.SetKillSwitch method persists before the in-memory flag
// updates, guaranteeing a crash can never leave a "killed" dispatcher still
// firing on the next boot.
func (s *Server) toggleExecutionKill(w http.ResponseWriter, r *http.Request) {
	if s.execHolder == nil {
		http.Error(w, "execution config not enabled", http.StatusNotFound)
		return
	}
	var body struct {
		On bool `json:"on"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.execHolder.SetKillSwitch(body.On); err != nil {
		log.Error().Err(err).Msg("api: kill switch persist failed")
		http.Error(w, "persist failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"kill_switch": body.On})
}

// postExecutionFeedback receives asynchronous OrderFeedback from a plugin.
// Codex #1: verify HMAC over the raw request body (no re-marshal).
// Codex #4: idempotent insert — replay returns 409.
// Codex #7: clock skew window is configurable via ExecutionConfig.TimestampSkew.
// On terminal status (FILLED/REJECTED/CANCELLED/ERROR) the dispatcher's
// ActiveCount slot is released so silent plugins do not leak capacity.
func (s *Server) postExecutionFeedback(w http.ResponseWriter, r *http.Request) {
	if s.execHolder == nil || s.execFeedback == nil {
		http.Error(w, "execution feedback not enabled", http.StatusNotFound)
		return
	}

	// Read the raw body once — we MUST verify HMAC against the exact bytes
	// that were signed, not a re-encoded representation.
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MiB cap
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}

	pluginID := r.Header.Get(execution.PluginIDHeader)
	sigHex := r.Header.Get(execution.SignatureHeader)
	tsRaw := r.Header.Get(execution.TimestampHeader)
	if pluginID == "" || sigHex == "" || tsRaw == "" {
		http.Error(w, "missing auth headers", http.StatusUnauthorized)
		return
	}
	ts, err := execution.ParseTimestamp(tsRaw)
	if err != nil {
		http.Error(w, "bad timestamp", http.StatusUnauthorized)
		return
	}

	cfg := s.execHolder.Get()
	if !execution.WithinSkew(ts, cfg.TimestampSkew(), time.Now()) {
		http.Error(w, "timestamp outside skew window", http.StatusUnauthorized)
		return
	}

	plugin, ok := s.execHolder.PluginByID(pluginID)
	if !ok || !plugin.Enabled {
		http.Error(w, "unknown or disabled plugin", http.StatusUnauthorized)
		return
	}
	if !execution.Verify(plugin.Secret, pluginID, ts, r.Method, r.URL.Path, body, sigHex) {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}

	// Parse the feedback payload AFTER authentication succeeds.
	var fb struct {
		SignalID string `json:"signal_id"`
		OrderID  string `json:"order_id"`
		Status   string `json:"status"`
	}
	if err := json.Unmarshal(body, &fb); err != nil {
		http.Error(w, "invalid feedback JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if fb.SignalID == "" || fb.Status == "" {
		http.Error(w, "signal_id and status are required", http.StatusBadRequest)
		return
	}

	fresh, err := s.execFeedback.RecordOnce(r.Context(), pluginID, fb.SignalID, fb.OrderID, fb.Status, "", "", time.Now())
	if err != nil {
		log.Error().Err(err).Msg("api: feedback idempotency insert failed")
		http.Error(w, "persist failed", http.StatusInternalServerError)
		return
	}
	if !fresh {
		http.Error(w, "duplicate feedback", http.StatusConflict)
		return
	}

	// Terminal statuses release the dispatcher's ActiveCount slot. Non-terminal
	// (ACK, PARTIAL_FILL, etc.) are persisted but do not free capacity.
	if s.execDispatcher != nil && isTerminalFeedbackStatus(fb.Status) {
		s.execDispatcher.Release()
	}

	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}

// isTerminalFeedbackStatus returns true for statuses that indicate a dispatch
// has finished (success or failure) and the ActiveCount slot can be freed.
func isTerminalFeedbackStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "FILLED", "REJECTED", "CANCELLED", "CANCELED", "ERROR":
		return true
	default:
		return false
	}
}
