package api

import (
	"net/http"
	"strings"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
)

// WithStartupSettings copies the startup input, never the file at request time.
func (s *Server) WithStartupSettings(values map[string]string) {
	s.startupSettings = make(map[string]string, len(values))
	for k, v := range values {
		s.startupSettings[k] = v
	}
}

// External processes have their own lifecycles; this server cannot attest to them.
func externalSetting(k string) bool {
	return strings.HasPrefix(k, "CHARTNAGARI_") || strings.HasPrefix(k, "ALPACA_") || k == "LISTEN_ADDR"
}

func (s *Server) getSettingsStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	settings, err := appconfig.LoadSettings(s.settingsFile)
	if err != nil {
		http.Error(w, "cannot read settings.yaml", http.StatusInternalServerError)
		return
	}
	saved := settings.ToMap()
	fields := make(map[string]string, len(envExposedKeys))
	for _, k := range envExposedKeys {
		switch {
		case externalSetting(k):
			fields[k] = "external"
		case s.startupSettings == nil:
			fields[k] = "unknown"
		case saved[k] != s.startupSettings[k]:
			fields[k] = "pending_restart"
		default:
			fields[k] = "startup_match"
		}
	}
	// Return states only, never secret values, hashes, or provider error bodies.
	jsonOK(w, map[string]any{"fields": fields})
}

func (s *Server) WithCalendarDiagnostics(status func() any) { s.calendarDiagnostics = status }

func (s *Server) getCalendarStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.calendarDiagnostics == nil {
		jsonOK(w, map[string]string{"state": "unknown"})
		return
	}
	jsonOK(w, s.calendarDiagnostics())
}
