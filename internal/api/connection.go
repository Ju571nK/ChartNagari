package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type connectionTicket struct {
	origin  string
	expires time.Time
}

// WithRemoteAccess opts into authentication for all API reads as well as writes.
// TLS must be terminated by the deployment's HTTPS reverse proxy.
func (s *Server) WithRemoteAccess(enabled bool) { s.remoteAccess = enabled }

func (s *Server) connectionInfo(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	jsonOK(w, map[string]any{"application": "ChartNagari", "protocol": 1, "remote_access": s.remoteAccess, "authentication": "admin_token", "capabilities": map[string]bool{"calendar": s.calendarStore != nil, "analysis": s.analystDirector != nil, "websocket": s.wsHub != nil, "ollama_management": s.ollamaStarter != nil}})
}

func (s *Server) connectionWSTicket(w http.ResponseWriter, r *http.Request) {
	if s.wsHub == nil {
		http.Error(w, "websocket unavailable", 503)
		return
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		http.Error(w, "ticket unavailable", 503)
		return
	}
	ticket := hex.EncodeToString(random[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.wsTickets == nil {
		s.wsTickets = make(map[string]connectionTicket)
	}
	for key, value := range s.wsTickets {
		if time.Now().After(value.expires) {
			delete(s.wsTickets, key)
		}
	}
	if len(s.wsTickets) >= 256 {
		http.Error(w, "too many tickets", 429)
		return
	}
	s.wsTickets[ticket] = connectionTicket{r.Header.Get("Origin"), time.Now().Add(30 * time.Second)}
	w.Header().Set("Cache-Control", "no-store")
	jsonOK(w, map[string]string{"ticket": ticket})
}

func (s *Server) connectionWS(w http.ResponseWriter, r *http.Request) {
	if s.remoteAccess {
		origin := r.Header.Get("Origin")
		u, err := url.Parse(origin)
		if origin != "" && !s.allowedOrigins[origin] && (err != nil || u.Host != r.Host) {
			http.Error(w, "untrusted origin", 403)
			return
		}
		// Execution clients retain the hub's independent HMAC authentication.
		execution := false
		valid := false
		for _, protocol := range strings.Split(r.Header.Get("Sec-WebSocket-Protocol"), ",") {
			protocol = strings.TrimSpace(protocol)
			if protocol == "chartnagari.v1" {
				execution = true
			}
			if strings.HasPrefix(protocol, "ticket.") {
				s.mu.Lock()
				key := strings.TrimPrefix(protocol, "ticket.")
				entry, ok := s.wsTickets[key]
				delete(s.wsTickets, key)
				s.mu.Unlock()
				valid = ok && entry.origin == origin && time.Now().Before(entry.expires)
			}
		}
		if !valid && !execution {
			http.Error(w, "websocket authentication required", 401)
			return
		}
	}
	s.wsHub.ServeWS(w, r)
}
