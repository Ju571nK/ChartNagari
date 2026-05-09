package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Ju571nK/Chatter/internal/marks"
)

var validMarkActions = map[string]struct{}{
	"took": {}, "skip": {}, "win": {}, "loss": {}, "be": {}, "undo": {},
}

type markRequest struct {
	Action string `json:"action"`
}

type markResponse struct {
	SignalID  int64  `json:"signal_id"`
	Status    string `json:"status"` // "" when row deleted (after undo)
	UpdatedAt int64  `json:"updated_at"`
}

// postMark handles POST /api/marks/{signal_id}.
func (s *Server) postMark(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearer(w, r) {
		return
	}
	if s.markStore == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "mark store not configured")
		return
	}
	idStr := r.PathValue("signal_id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid signal_id")
		return
	}

	var req markRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if _, ok := validMarkActions[req.Action]; !ok {
		writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("invalid action: %q (must be took|skip|win|loss|be|undo)", req.Action))
		return
	}

	exists, err := s.markStore.SignalExists(id)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		writeAPIError(w, http.StatusNotFound, fmt.Sprintf("signal not found: %d", id))
		return
	}

	newStatus, err := s.markStore.Mark(id, req.Action)
	if err != nil {
		if strings.Contains(err.Error(), "invalid") {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeAPIError(w, http.StatusInternalServerError, fmt.Sprintf("mark: %v", err))
		return
	}
	jsonOK(w, markResponse{SignalID: id, Status: newStatus, UpdatedAt: time.Now().Unix()})
}

// getPending handles GET /api/marks/pending.
func (s *Server) getPending(w http.ResponseWriter, r *http.Request) {
	if s.markStore == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "mark store not configured")
		return
	}
	since, limit := parseSinceLimit(r, 24*time.Hour, 50)
	rows, err := s.markStore.ListPending(since, limit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, rows)
}

// getRecent handles GET /api/marks/recent.
func (s *Server) getRecent(w http.ResponseWriter, r *http.Request) {
	if s.markStore == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "mark store not configured")
		return
	}
	since, limit := parseSinceLimit(r, 30*24*time.Hour, 50)
	rows, err := s.markStore.ListMarked(since, limit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, rows)
}

type rollupResponse struct {
	By    string            `json:"by"`
	Since string            `json:"since"`
	Rows  []marks.RollupRow `json:"rows"`
}

// getRollup handles GET /api/marks/rollup.
func (s *Server) getRollup(w http.ResponseWriter, r *http.Request) {
	if s.aggregator == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "aggregator not configured")
		return
	}
	by := r.URL.Query().Get("by")
	if by == "" {
		by = "rule"
	}
	since, _ := parseSinceLimit(r, 30*24*time.Hour, 50)
	rows, err := s.aggregator.Rollup(marks.GroupBy(by), since)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "max-age=60")
	jsonOK(w, rollupResponse{By: by, Since: since.UTC().Format(time.RFC3339), Rows: rows})
}

// parseSinceLimit reads `since` and `limit` query params with defaults.
// `since` accepts RFC3339 strings or Unix seconds as an integer.
// Passing since=0 means "no lower bound" (returns time.Time{}).
func parseSinceLimit(r *http.Request, defaultSinceAgo time.Duration, defaultLimit int) (time.Time, int) {
	sinceStr := r.URL.Query().Get("since")
	since := time.Now().Add(-defaultSinceAgo)
	if sinceStr != "" {
		// Try RFC3339 first.
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = t
		} else if n, err := strconv.ParseInt(sinceStr, 10, 64); err == nil {
			// Unix seconds; 0 means "all time" (zero Time disables the filter in ListPending).
			if n == 0 {
				since = time.Time{}
			} else {
				since = time.Unix(n, 0)
			}
		}
	}
	limit := defaultLimit
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	return since, limit
}
