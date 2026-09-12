package api

import (
	"github.com/Ju571nK/Chatter/internal/backtest"
	"net/http"
)

type backtestPreparer interface {
	Prepare(string, string) (*backtest.Readiness, error)
}

func (s *Server) getBacktestReadiness(w http.ResponseWriter, r *http.Request) {
	preparer, ok := s.backtestRunner.(backtestPreparer)
	if !ok {
		http.Error(w, "backtest preparation unavailable", http.StatusServiceUnavailable)
		return
	}
	symbol, tf := r.URL.Query().Get("symbol"), r.URL.Query().Get("timeframe")
	if symbol == "" || tf == "" {
		http.Error(w, "symbol and timeframe required", http.StatusBadRequest)
		return
	}
	result, err := preparer.Prepare(symbol, tf)
	if err != nil {
		http.Error(w, "failed to read backtest history", http.StatusInternalServerError)
		return
	}
	jsonOK(w, result)
}
