package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/Ju571nK/Chatter/internal/alertdrop"
)

type alertDropJSON struct {
	Symbol    string           `json:"symbol"`
	Rule      string           `json:"rule"`
	Timeframe string           `json:"timeframe"`
	Direction string           `json:"direction"`
	Reason    alertdrop.Reason `json:"reason"`
	Stage     string           `json:"stage"`
	Knob      string           `json:"knob"`
	Score     float64          `json:"score"`
	Threshold float64          `json:"threshold"`
	SignalID  int64            `json:"signal_id"`
	Sender    string           `json:"sender"`
	Detail    string           `json:"detail"`
	Count     int64            `json:"count"`
	FirstSeen time.Time        `json:"first_seen"`
	LastSeen  time.Time        `json:"last_seen"`
}

type alertDropsResponse struct {
	Enabled bool            `json:"enabled"`
	Drops   []alertDropJSON `json:"drops"`
}

func (s *Server) getAlertDrops(w http.ResponseWriter, r *http.Request) {
	q := alertdrop.Query{Symbol: r.URL.Query().Get("symbol")}
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeAPIError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		q.Limit = n
	}
	st := s.drops
	if st == nil {
		st = alertdrop.Nop{}
	}
	rows, err := st.Recent(r.Context(), q)
	if err != nil {
		log.Error().Err(err).Msg("alert drops query failed")
		writeAPIError(w, http.StatusInternalServerError, "alert drops query failed")
		return
	}
	out := make([]alertDropJSON, 0, len(rows))
	for _, row := range rows {
		out = append(out, alertDropJSON{
			Symbol:    row.Symbol,
			Rule:      row.Rule,
			Timeframe: row.Timeframe,
			Direction: row.Direction,
			Reason:    row.Reason,
			Stage:     row.Reason.Stage().String(),
			Knob:      row.Reason.Knob(),
			Score:     row.Score,
			Threshold: row.Threshold,
			SignalID:  row.SignalID,
			Sender:    row.Sender,
			Detail:    row.Detail,
			Count:     row.Count,
			FirstSeen: row.FirstSeen,
			LastSeen:  row.LastSeen,
		})
	}
	jsonOK(w, alertDropsResponse{Enabled: alertdrop.Enabled(st), Drops: out})
}
