package api

import (
	"context"
	"net/http"
	"time"

	"github.com/Ju571nK/Chatter/internal/collector"
	appconfig "github.com/Ju571nK/Chatter/internal/config"
)

func (s *Server) WithForexRuntime(onSettingsChanged func(), status func() any) {
	s.forexSettingsChanged = onSettingsChanged
	s.forexStatus = status
}

func (s *Server) getForexStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.forexStatus != nil {
		jsonOK(w, s.forexStatus())
		return
	}
	jsonOK(w, map[string]string{"state": "unknown"})
}

func (s *Server) testForexConnection(w http.ResponseWriter, r *http.Request) {
	if s.settingsFile == "" {
		http.Error(w, "settings unavailable", http.StatusServiceUnavailable)
		return
	}
	settings, err := appconfig.LoadSettings(s.settingsFile)
	if err != nil {
		http.Error(w, "settings unavailable", http.StatusInternalServerError)
		return
	}
	fx := appconfig.ForexConfig{Provider: settings.Forex.Provider, OANDA: appconfig.OANDAConfig{Token: settings.Forex.OANDA.Token, Environment: settings.Forex.OANDA.Environment}}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if fx.EffectiveProvider() == "yahoo" {
		c := collector.NewYahooFXCollector(nil, nil, nil, time.Minute)
		bars, err := c.Fetch(ctx, appconfig.SymbolEntry{Symbol: "EURUSD"}, "1D")
		if err != nil || len(bars) == 0 {
			http.Error(w, "Yahoo FX candle request failed", http.StatusBadGateway)
			return
		}
		jsonOK(w, map[string]any{"ok": true, "provider": "yahoo", "message": "EURUSD candles received"})
		return
	}
	if fx.OANDA.Token == "" {
		http.Error(w, "OANDA token is required", http.StatusBadRequest)
		return
	}
	c := collector.NewOANDACollector(nil, nil, nil, time.Minute, fx.OANDA.Token, fx.OANDA.Environment)
	bars, err := c.Fetch(ctx, appconfig.SymbolEntry{Symbol: "EURUSD"}, "1D")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if len(bars) == 0 {
		http.Error(w, "OANDA returned no complete candles", http.StatusBadGateway)
		return
	}
	jsonOK(w, map[string]any{"ok": true, "provider": "oanda", "environment": fx.OANDA.Environment, "message": "EURUSD candles received"})
}
