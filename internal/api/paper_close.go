package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/Ju571nK/Chatter/internal/paper"
)

type paperPipsCloser interface {
	ClosePaperPositionWithPips(id int64, exitPrice float64, status string, pnlPct, pnlPips float64) error
}

// closePaperPosition handles POST /api/paper/positions/{id}/close.
// exit_price in the body wins when it is positive. Otherwise the latest bar close is used.
func (s *Server) closePaperPosition(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearer(w, r) {
		return
	}
	if s.paperStore == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "paper store not configured")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeAPIError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var body struct {
		ExitPrice float64 `json:"exit_price"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	opens, err := s.paperStore.GetAllOpenPositions()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var pos *paper.PaperPosition
	for i := range opens {
		if opens[i].ID == id {
			found := opens[i]
			pos = &found
			break
		}
	}
	if pos == nil {
		writeAPIError(w, http.StatusNotFound, "open position not found")
		return
	}
	if pos.EntryPrice <= 0 {
		writeAPIError(w, http.StatusBadRequest, "position has no entry price")
		return
	}

	exitPrice := body.ExitPrice
	if exitPrice <= 0 {
		price, ok := s.latestClose(w, pos.Symbol, pos.Timeframe)
		if !ok {
			return
		}
		exitPrice = price
	}

	const status = "CLOSED_MANUAL"
	pnlPct, pnlPips := paper.PositionPnL(*pos, exitPrice)
	if pos.PipSize > 0 {
		if closer, ok := s.paperStore.(paperPipsCloser); ok {
			err = closer.ClosePaperPositionWithPips(id, exitPrice, status, pnlPct, pnlPips)
		} else {
			err = s.paperStore.ClosePaperPosition(id, exitPrice, status, pnlPct)
		}
	} else {
		err = s.paperStore.ClosePaperPosition(id, exitPrice, status, pnlPct)
	}
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]any{
		"id":         id,
		"status":     status,
		"exit_price": exitPrice,
		"pnl_pct":    pnlPct,
		"pnl_pips":   pnlPips,
	})
}

// latestClose writes the error response and returns false when no usable close exists.
func (s *Server) latestClose(w http.ResponseWriter, symbol, timeframe string) (float64, bool) {
	if s.chartStore == nil {
		writeAPIError(w, http.StatusBadRequest, "exit_price required")
		return 0, false
	}
	bars, err := s.chartStore.GetOHLCV(symbol, timeframe, 1)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return 0, false
	}
	if len(bars) == 0 || bars[0].Close <= 0 {
		writeAPIError(w, http.StatusBadRequest, "exit_price required")
		return 0, false
	}
	return bars[0].Close, true
}
