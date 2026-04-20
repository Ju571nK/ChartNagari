package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/Ju571nK/Chatter/internal/ollama"
)

// OllamaStatusProvider is the minimal interface the api package needs from the
// ollama detector. *ollama.Detector satisfies it. Tests can substitute a fake.
type OllamaStatusProvider interface {
	Detect(ctx context.Context) ollama.Status
}

// getOllamaStatus returns the detector's current view of Ollama state. Protected
// by requireBearer — the response includes host/model/version which are not
// sensitive, but the endpoint also drives UI that controls subprocess lifecycle,
// so we keep it behind auth.
func (s *Server) getOllamaStatus(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearer(w, r) {
		return
	}
	if s.ollamaDetector == nil {
		http.Error(w, "ollama detector not configured", http.StatusServiceUnavailable)
		return
	}
	status := s.ollamaDetector.Detect(r.Context())
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		log.Error().Err(err).Msg("api: ollama status encode")
	}
}
