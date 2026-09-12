package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/Ju571nK/Chatter/internal/storage"
)

type htfReadinessStore interface {
	HTFCalibrationReadiness(context.Context, string, string, time.Time) (*storage.HTFReadiness, error)
}

func (s *Server) getHTFReadiness(w http.ResponseWriter, r *http.Request) {
	symbol, tf := strings.TrimSpace(r.URL.Query().Get("symbol")), r.URL.Query().Get("timeframe")
	if symbol == "" || len(symbol) > 64 || symbol == "ALL" || (tf != "1H" && tf != "4H") {
		http.Error(w, "symbol and lower timeframe (1H or 4H) required", http.StatusBadRequest)
		return
	}
	store, ok := s.chartStore.(htfReadinessStore)
	if !ok {
		http.Error(w, "HTF history unavailable", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	result, err := store.HTFCalibrationReadiness(ctx, symbol, tf, time.Now())
	if err != nil {
		http.Error(w, "failed to read HTF history", http.StatusInternalServerError)
		return
	}
	jsonOK(w, result)
}
