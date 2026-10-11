package api

import (
	"encoding/json"
	"io"
	"net/http"
)

func (s *Server) getWatchlistTimeframes(w http.ResponseWriter, r *http.Request) {
	wl, err := s.readWatchlist()
	if err != nil {
		http.Error(w, "watchlist unavailable", 500)
		return
	}
	jsonOK(w, map[string]any{"timeframes": wl.Timeframes})
}

// Only the collection timeframes are replaced. Symbol edits made before this
// request are retained, and runtime generations restart through the normal hook.
func (s *Server) putWatchlistTimeframes(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearer(w, r) {
		return
	}
	var body struct {
		Timeframes []string `json:"timeframes"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		http.Error(w, "invalid timeframes", 400)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		http.Error(w, "one JSON object required", 400)
		return
	}
	if len(body.Timeframes) == 0 || len(body.Timeframes) > len(validTimeframes) {
		http.Error(w, "nonempty timeframes required", 400)
		return
	}
	seen := map[string]bool{}
	legacy := false
	for _, tf := range body.Timeframes {
		if _, ok := validTimeframes[tf]; !ok || seen[tf] {
			http.Error(w, "invalid or duplicate timeframe", 400)
			return
		}
		seen[tf] = true
		legacy = legacy || tf != "15M"
	}
	if !legacy {
		http.Error(w, "retain at least one standard timeframe", 400)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	wl, err := s.readWatchlistLocked()
	if err != nil {
		http.Error(w, "watchlist unavailable", 500)
		return
	}
	wl.Timeframes = body.Timeframes
	if err = s.writeWatchlistLocked(wl); err != nil {
		http.Error(w, configWriteErrorMessage(err), 500)
		return
	}
	jsonOK(w, map[string]any{"timeframes": wl.Timeframes})
}

// 15M is an FX feature, determined by registration rather than ticker spelling.
func (s *Server) allowForex15M(w http.ResponseWriter, symbol, tf string) bool {
	if tf != "15M" {
		return true
	}
	wl, err := s.readWatchlist()
	if err != nil {
		http.Error(w, "watchlist unavailable", 500)
		return false
	}
	if wl.AssetClass(symbol) != "forex" {
		http.Error(w, "15M is available only for registered Forex symbols", 400)
		return false
	}
	return true
}
