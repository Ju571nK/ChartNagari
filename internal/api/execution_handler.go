package api

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/execution"
	"github.com/Ju571nK/Chatter/pkg/models"
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
	var fb models.OrderFeedback
	if err := json.Unmarshal(body, &fb); err != nil {
		http.Error(w, "invalid feedback JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if fb.SignalID == "" || fb.Status == "" {
		http.Error(w, "signal_id and status are required", http.StatusBadRequest)
		return
	}

	fresh, err := s.execFeedback.RecordOnce(
		r.Context(),
		pluginID, fb.SignalID, fb.OrderID, fb.Status,
		strings.ToUpper(fb.Symbol), fb.Message,
		time.Now(),
	)
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

// getExecutionPluginStats returns 24h aggregated counts per plugin (filled,
// rejected, submitted) together with the most recent failure message.
// Only plugins that have at least one feedback row in the window appear.
// Cache-Control: max-age=60 is set so UI pollers stay cheap.
func (s *Server) getExecutionPluginStats(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearer(w, r) {
		return
	}
	if s.execDB == nil {
		http.Error(w, "not enabled", http.StatusNotFound)
		return
	}
	window := r.URL.Query().Get("window")
	if window == "" {
		window = "24h"
	}
	if window != "24h" {
		http.Error(w, "only window=24h is supported", http.StatusBadRequest)
		return
	}

	cutoff := time.Now().Add(-24 * time.Hour).Unix()

	rows, err := s.execDB.QueryContext(r.Context(), `
		SELECT plugin_id,
		       SUM(CASE WHEN status IN ('SUBMITTED','RECEIVED') THEN 1 ELSE 0 END) AS submitted,
		       SUM(CASE WHEN status IN ('FILLED','PARTIAL_FILL')  THEN 1 ELSE 0 END) AS filled,
		       SUM(CASE WHEN status IN ('REJECTED','ERROR')       THEN 1 ELSE 0 END) AS rejected,
		       MAX(CASE WHEN status IN ('REJECTED','ERROR') THEN received_at END)   AS last_fail_at,
		       COALESCE(
		         (SELECT message FROM feedback_idempotency f2
		          WHERE f2.plugin_id = f1.plugin_id
		            AND f2.status IN ('REJECTED','ERROR')
		            AND f2.received_at >= ?
		          ORDER BY f2.received_at DESC LIMIT 1), '') AS last_fail_msg
		FROM feedback_idempotency f1
		WHERE received_at >= ?
		GROUP BY plugin_id`,
		cutoff, cutoff,
	)
	if err != nil {
		http.Error(w, "query", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type stat struct {
		PluginID       string `json:"plugin_id"`
		Submitted      int    `json:"submitted"`
		Filled         int    `json:"filled"`
		Rejected       int    `json:"rejected"`
		LastFailureAt  *int64 `json:"last_failure_at,omitempty"`
		LastFailureMsg string `json:"last_failure_msg"`
	}
	out := []stat{}
	for rows.Next() {
		var st stat
		var lastFailAt sql.NullInt64
		if err := rows.Scan(&st.PluginID, &st.Submitted, &st.Filled, &st.Rejected, &lastFailAt, &st.LastFailureMsg); err != nil {
			http.Error(w, "scan", http.StatusInternalServerError)
			return
		}
		if lastFailAt.Valid {
			v := lastFailAt.Int64
			st.LastFailureAt = &v
		}
		out = append(out, st)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "max-age=60")
	_ = json.NewEncoder(w).Encode(map[string]any{"window": "24h", "plugins": out})
}

// listExecutionFeedback returns recent feedback rows, newest first, with
// optional plugin/status/symbol filters and a limit (default 100, max 500).
// Unlike most GET endpoints, this one enforces bearer-token auth when s.apiToken
// is configured, because feedback rows may contain sensitive trade data.
func (s *Server) listExecutionFeedback(w http.ResponseWriter, r *http.Request) {
	if s.execDB == nil {
		http.Error(w, "not enabled", http.StatusNotFound)
		return
	}

	// Enforce bearer-token auth for this sensitive GET endpoint.
	if !s.requireBearer(w, r) {
		return
	}

	q := r.URL.Query()
	plugin := q.Get("plugin")
	status := q.Get("status")
	symbol := strings.ToUpper(strings.TrimSpace(q.Get("symbol")))

	limit := 100
	if ls := q.Get("limit"); ls != "" {
		n, err := strconv.Atoi(ls)
		if err != nil {
			http.Error(w, "bad limit", http.StatusBadRequest)
			return
		}
		if n < 0 || n > 500 {
			http.Error(w, "limit out of range (0..500)", http.StatusBadRequest)
			return
		}
		if n > 0 {
			limit = n
		}
	}

	rows, err := s.execDB.QueryContext(r.Context(), `
		SELECT plugin_id, signal_id, order_id, status, COALESCE(symbol,''), COALESCE(message,''), received_at
		FROM feedback_idempotency
		WHERE (? = '' OR plugin_id = ?)
		  AND (? = '' OR status = ?)
		  AND (? = '' OR symbol = ?)
		ORDER BY received_at DESC
		LIMIT ?`,
		plugin, plugin, status, status, symbol, symbol, limit,
	)
	if err != nil {
		http.Error(w, "query", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type feedbackItem struct {
		PluginID   string `json:"plugin_id"`
		SignalID   string `json:"signal_id"`
		OrderID    string `json:"order_id"`
		Status     string `json:"status"`
		Symbol     string `json:"symbol"`
		Message    string `json:"message"`
		ReceivedAt int64  `json:"received_at"`
	}
	items := []feedbackItem{}
	for rows.Next() {
		var it feedbackItem
		if err := rows.Scan(&it.PluginID, &it.SignalID, &it.OrderID, &it.Status, &it.Symbol, &it.Message, &it.ReceivedAt); err != nil {
			http.Error(w, "scan", http.StatusInternalServerError)
			return
		}
		items = append(items, it)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "count": len(items)})
}
